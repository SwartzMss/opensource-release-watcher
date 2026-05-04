package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"opensource-release-watcher/backend/internal/version"
)

type Client struct {
	token         string
	httpClient    *http.Client
	mu            sync.Mutex
	tagsCache     map[string]repoTagsCache
	releasesCache map[string]repoReleasesCache
}

type ReleaseInfo struct {
	Source      string     `json:"source"`
	Version     string     `json:"version"`
	Title       string     `json:"title"`
	URL         string     `json:"url"`
	PublishedAt *time.Time `json:"published_at,omitempty"`
	Note        string     `json:"note"`
}

const historyPageSize = 100
const githubRetryAttempts = 3
const githubRetryDelay = 10 * time.Second
const githubHistoryCacheTTL = 30 * time.Minute

func NewClient(token string) *Client {
	return &Client{
		token: token,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
		tagsCache:     map[string]repoTagsCache{},
		releasesCache: map[string]repoReleasesCache{},
	}
}

type repoTagsCache struct {
	loadedAt time.Time
	tags     []tagPayload
}

type repoReleasesCache struct {
	loadedAt time.Time
	releases []releasePayload
}

func (c *Client) LatestRelease(ctx context.Context, owner, repo string) (*ReleaseInfo, error) {
	var payload struct {
		TagName     string    `json:"tag_name"`
		Name        string    `json:"name"`
		HTMLURL     string    `json:"html_url"`
		PublishedAt time.Time `json:"published_at"`
		Body        string    `json:"body"`
	}
	if err := c.get(ctx, fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", owner, repo), &payload); err != nil {
		return nil, err
	}
	return &ReleaseInfo{
		Source:      "release",
		Version:     payload.TagName,
		Title:       payload.Name,
		URL:         payload.HTMLURL,
		PublishedAt: &payload.PublishedAt,
		Note:        payload.Body,
	}, nil
}

type releasePayload struct {
	TagName     string     `json:"tag_name"`
	Name        string     `json:"name"`
	HTMLURL     string     `json:"html_url"`
	PublishedAt *time.Time `json:"published_at"`
	Body        string     `json:"body"`
	Draft       bool       `json:"draft"`
	Prerelease  bool       `json:"prerelease"`
}

type tagPayload struct {
	Name   string `json:"name"`
	Commit struct {
		SHA string `json:"sha"`
	} `json:"commit"`
}

func (c *Client) listReleases(ctx context.Context, owner, repo string) ([]releasePayload, error) {
	if cached, ok := c.getCachedReleases(owner, repo); ok {
		return cached, nil
	}
	releases := make([]releasePayload, 0)
	for page := 1; ; page++ {
		var payload []releasePayload
		url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases?per_page=%d&page=%d", owner, repo, historyPageSize, page)
		if err := c.get(ctx, url, &payload); err != nil {
			return nil, err
		}
		if len(payload) == 0 {
			return releases, nil
		}
		releases = append(releases, payload...)
		if len(payload) < historyPageSize {
			c.setCachedReleases(owner, repo, releases)
			return releases, nil
		}
	}
}

func (c *Client) listTags(ctx context.Context, owner, repo string) ([]tagPayload, error) {
	if cached, ok := c.getCachedTags(owner, repo); ok {
		return cached, nil
	}
	tags := make([]tagPayload, 0)
	for page := 1; ; page++ {
		var payload []tagPayload
		url := fmt.Sprintf("https://api.github.com/repos/%s/%s/tags?per_page=%d&page=%d", owner, repo, historyPageSize, page)
		if err := c.get(ctx, url, &payload); err != nil {
			return nil, err
		}
		if len(payload) == 0 {
			return tags, nil
		}
		tags = append(tags, payload...)
		if len(payload) < historyPageSize {
			c.setCachedTags(owner, repo, tags)
			return tags, nil
		}
	}
}

func (c *Client) LatestTag(ctx context.Context, owner, repo string) (*ReleaseInfo, error) {
	var payload []struct {
		Name   string `json:"name"`
		Commit struct {
			SHA string `json:"sha"`
			URL string `json:"url"`
		} `json:"commit"`
		TarballURL string `json:"tarball_url"`
	}
	if err := c.get(ctx, fmt.Sprintf("https://api.github.com/repos/%s/%s/tags?per_page=1", owner, repo), &payload); err != nil {
		return nil, err
	}
	if len(payload) == 0 {
		return nil, fmt.Errorf("no tags found")
	}
	return &ReleaseInfo{
		Source:  "tag",
		Version: payload[0].Name,
		Title:   payload[0].Name,
		URL:     fmt.Sprintf("https://github.com/%s/%s/releases/tag/%s", owner, repo, payload[0].Name),
	}, nil
}

func (c *Client) FindSuggestedReleaseVersion(ctx context.Context, owner, repo, currentVersion, commitSHA string) (string, error) {
	commitSHA = strings.TrimSpace(commitSHA)
	currentVersion = version.Normalize(currentVersion)
	if commitSHA == "" {
		return "", nil
	}
	tags, err := c.listTags(ctx, owner, repo)
	if err != nil {
		return "", err
	}
	log.Printf("github suggested version tags loaded owner=%s repo=%s count=%d first=%s", owner, repo, len(tags), previewTags(tags, 5))
	if exact := exactTagForCommit(commitSHA, tags, currentVersion); exact != "" {
		log.Printf("github suggested version exact tag hit owner=%s repo=%s commit=%s tag=%s", owner, repo, commitSHA, exact)
		return exact, nil
	}
	log.Printf("github suggested version exact tag miss owner=%s repo=%s commit=%s", owner, repo, commitSHA)

	releases, err := c.listReleases(ctx, owner, repo)
	if err != nil {
		log.Printf("github releases list failed owner=%s repo=%s err=%v", owner, repo, err)
		releases = nil
	}
	tagCommitByName := make(map[string]string, len(tags))
	for _, tag := range tags {
		name := strings.TrimSpace(tag.Name)
		if name == "" {
			continue
		}
		tagCommitByName[version.Normalize(name)] = strings.TrimSpace(tag.Commit.SHA)
	}
	if len(releases) > 0 {
		sort.SliceStable(releases, func(i, j int) bool {
			left := time.Time{}
			right := time.Time{}
			if releases[i].PublishedAt != nil {
				left = *releases[i].PublishedAt
			}
			if releases[j].PublishedAt != nil {
				right = *releases[j].PublishedAt
			}
			return left.Before(right)
		})
		if matched, err := c.findReleaseContainingCommit(ctx, owner, repo, currentVersion, commitSHA, releases, tagCommitByName, false); err != nil {
			return "", err
		} else if matched != "" {
			return matched, nil
		}
		if matched, err := c.findReleaseContainingCommit(ctx, owner, repo, currentVersion, commitSHA, releases, tagCommitByName, true); err != nil {
			return "", err
		} else if matched != "" {
			return matched, nil
		}
	}
	if matched, err := c.findTagContainingCommit(ctx, owner, repo, currentVersion, commitSHA, tags); err != nil {
		return "", err
	} else if matched != "" {
		return matched, nil
	}
	return c.FindCommitTag(ctx, owner, repo, commitSHA)
}

func exactTagForCommit(commitSHA string, tags []tagPayload, currentVersion string) string {
	commitSHA = strings.TrimSpace(commitSHA)
	for _, tag := range tags {
		name := strings.TrimSpace(tag.Name)
		if name == "" || !version.IsNewer(name, currentVersion) {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(tag.Commit.SHA), commitSHA) {
			return name
		}
	}
	return ""
}

func previewTags(tags []tagPayload, limit int) string {
	if limit <= 0 {
		limit = 1
	}
	parts := make([]string, 0, limit)
	for i, tag := range tags {
		if i >= limit {
			break
		}
		parts = append(parts, fmt.Sprintf("%s=%s", strings.TrimSpace(tag.Name), strings.TrimSpace(tag.Commit.SHA)))
	}
	return strings.Join(parts, ", ")
}

func (c *Client) getCachedTags(owner, repo string) ([]tagPayload, bool) {
	key := repoHistoryCacheKey(owner, repo)
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.tagsCache[key]
	if !ok || time.Since(entry.loadedAt) > githubHistoryCacheTTL {
		return nil, false
	}
	log.Printf("github suggested version tags cache hit owner=%s repo=%s count=%d", owner, repo, len(entry.tags))
	return cloneTags(entry.tags), true
}

func (c *Client) setCachedTags(owner, repo string, tags []tagPayload) {
	key := repoHistoryCacheKey(owner, repo)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tagsCache[key] = repoTagsCache{
		loadedAt: time.Now().UTC(),
		tags:     cloneTags(tags),
	}
}

func (c *Client) getCachedReleases(owner, repo string) ([]releasePayload, bool) {
	key := repoHistoryCacheKey(owner, repo)
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.releasesCache[key]
	if !ok || time.Since(entry.loadedAt) > githubHistoryCacheTTL {
		return nil, false
	}
	log.Printf("github suggested version releases cache hit owner=%s repo=%s count=%d", owner, repo, len(entry.releases))
	return cloneReleases(entry.releases), true
}

func (c *Client) setCachedReleases(owner, repo string, releases []releasePayload) {
	key := repoHistoryCacheKey(owner, repo)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.releasesCache[key] = repoReleasesCache{
		loadedAt: time.Now().UTC(),
		releases: cloneReleases(releases),
	}
}

func repoHistoryCacheKey(owner, repo string) string {
	return owner + "/" + repo
}

func cloneTags(tags []tagPayload) []tagPayload {
	if len(tags) == 0 {
		return nil
	}
	clone := make([]tagPayload, len(tags))
	copy(clone, tags)
	return clone
}

func cloneReleases(releases []releasePayload) []releasePayload {
	if len(releases) == 0 {
		return nil
	}
	clone := make([]releasePayload, len(releases))
	copy(clone, releases)
	return clone
}

func (c *Client) HasVersion(ctx context.Context, owner, repo, targetVersion string, releaseFirst bool) (bool, error) {
	targetVersion = version.Normalize(targetVersion)
	if targetVersion == "" {
		return false, nil
	}
	if releaseFirst {
		found, err := c.historyContainsRelease(ctx, owner, repo, targetVersion)
		if err != nil || found {
			return found, err
		}
	}
	return c.historyContainsTag(ctx, owner, repo, targetVersion)
}

func (c *Client) historyContainsRelease(ctx context.Context, owner, repo, targetVersion string) (bool, error) {
	for page := 1; ; page++ {
		var payload []struct {
			TagName string `json:"tag_name"`
			Name    string `json:"name"`
		}
		url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases?per_page=%d&page=%d", owner, repo, historyPageSize, page)
		if err := c.get(ctx, url, &payload); err != nil {
			return false, err
		}
		if len(payload) == 0 {
			return false, nil
		}
		for _, item := range payload {
			if version.Normalize(item.TagName) == targetVersion || version.Normalize(item.Name) == targetVersion {
				return true, nil
			}
		}
		if len(payload) < historyPageSize {
			return false, nil
		}
	}
}

func (c *Client) historyContainsTag(ctx context.Context, owner, repo, targetVersion string) (bool, error) {
	for page := 1; ; page++ {
		var payload []struct {
			Name string `json:"name"`
		}
		url := fmt.Sprintf("https://api.github.com/repos/%s/%s/tags?per_page=%d&page=%d", owner, repo, historyPageSize, page)
		if err := c.get(ctx, url, &payload); err != nil {
			return false, err
		}
		if len(payload) == 0 {
			return false, nil
		}
		for _, item := range payload {
			if version.Normalize(item.Name) == targetVersion {
				return true, nil
			}
		}
		if len(payload) < historyPageSize {
			return false, nil
		}
	}
}

func (c *Client) FindTagCommit(ctx context.Context, owner, repo string, tagCandidates []string) (string, string, error) {
	candidates := map[string]struct{}{}
	for _, candidate := range tagCandidates {
		candidate = version.Normalize(candidate)
		if candidate == "" {
			continue
		}
		candidates[candidate] = struct{}{}
	}
	if len(candidates) == 0 {
		return "", "", fmt.Errorf("tag candidates are required")
	}
	for page := 1; ; page++ {
		var payload []struct {
			Name   string `json:"name"`
			Commit struct {
				SHA string `json:"sha"`
			} `json:"commit"`
		}
		url := fmt.Sprintf("https://api.github.com/repos/%s/%s/tags?per_page=%d&page=%d", owner, repo, historyPageSize, page)
		if err := c.get(ctx, url, &payload); err != nil {
			return "", "", err
		}
		if len(payload) == 0 {
			return "", "", fmt.Errorf("no matching tags found")
		}
		for _, item := range payload {
			tagName := version.Normalize(item.Name)
			if _, ok := candidates[tagName]; ok || containsCandidate(candidates, item.Name) {
				return item.Commit.SHA, item.Name, nil
			}
		}
		if len(payload) < historyPageSize {
			return "", "", fmt.Errorf("no matching tags found")
		}
	}
}

func (c *Client) FindCommitTag(ctx context.Context, owner, repo, commitSHA string) (string, error) {
	commitSHA = version.Normalize(commitSHA)
	if commitSHA == "" {
		return "", nil
	}
	for page := 1; ; page++ {
		var payload []struct {
			Name   string `json:"name"`
			Commit struct {
				SHA string `json:"sha"`
			} `json:"commit"`
		}
		url := fmt.Sprintf("https://api.github.com/repos/%s/%s/tags?per_page=%d&page=%d", owner, repo, historyPageSize, page)
		if err := c.get(ctx, url, &payload); err != nil {
			return "", err
		}
		if len(payload) == 0 {
			return "", nil
		}
		for _, item := range payload {
			if strings.EqualFold(strings.TrimSpace(item.Commit.SHA), commitSHA) {
				return item.Name, nil
			}
		}
		if len(payload) < historyPageSize {
			return "", nil
		}
	}
}

func (c *Client) findReleaseContainingCommit(ctx context.Context, owner, repo, currentVersion, commitSHA string, releases []releasePayload, tagCommitByName map[string]string, includePrereleases bool) (string, error) {
	for _, rel := range releases {
		if rel.Draft {
			continue
		}
		if rel.Prerelease != includePrereleases {
			continue
		}
		tag := strings.TrimSpace(rel.TagName)
		if tag == "" {
			tag = strings.TrimSpace(rel.Name)
		}
		if tag == "" {
			continue
		}
		if !version.IsNewer(tag, currentVersion) {
			continue
		}
		if tagCommit, ok := tagCommitByName[version.Normalize(tag)]; ok && tagCommit != "" {
			contains, err := c.commitContainedByRef(ctx, owner, repo, commitSHA, tagCommit)
			if err != nil {
				return "", err
			}
			if contains {
				return tag, nil
			}
		}
		contains, err := c.commitContainedByRef(ctx, owner, repo, commitSHA, tag)
		if err != nil {
			return "", err
		}
		if contains {
			return tag, nil
		}
	}
	return "", nil
}

func (c *Client) findTagContainingCommit(ctx context.Context, owner, repo, currentVersion, commitSHA string, tags []tagPayload) (string, error) {
	matches := make([]string, 0)
	for _, tag := range tags {
		name := strings.TrimSpace(tag.Name)
		if name == "" {
			continue
		}
		if !version.IsNewer(name, currentVersion) {
			continue
		}
		tagSHA := strings.TrimSpace(tag.Commit.SHA)
		contains := false
		if tagSHA != "" {
			ok, err := c.commitContainedByRef(ctx, owner, repo, commitSHA, tagSHA)
			if err != nil {
				return "", err
			}
			contains = ok
		}
		if !contains {
			ok, err := c.commitContainedByRef(ctx, owner, repo, commitSHA, name)
			if err != nil {
				return "", err
			}
			contains = ok
		}
		if contains {
			matches = append(matches, name)
		}
	}
	if len(matches) == 0 {
		return "", nil
	}
	sort.SliceStable(matches, func(i, j int) bool {
		return version.IsNewer(matches[j], matches[i])
	})
	return matches[0], nil
}

func (c *Client) commitContainedByRef(ctx context.Context, owner, repo, base, head string) (bool, error) {
	base = strings.TrimSpace(base)
	head = strings.TrimSpace(head)
	if base == "" || head == "" {
		return false, nil
	}
	var payload struct {
		Status   string `json:"status"`
		AheadBy  int    `json:"ahead_by"`
		BehindBy int    `json:"behind_by"`
	}
	compareURL := fmt.Sprintf(
		"https://api.github.com/repos/%s/%s/compare/%s...%s",
		owner,
		repo,
		url.PathEscape(base),
		url.PathEscape(head),
	)
	if err := c.get(ctx, compareURL, &payload); err != nil {
		return false, err
	}
	return payload.Status == "ahead" || payload.Status == "identical" || (payload.AheadBy >= 0 && payload.BehindBy == 0), nil
}

func containsCandidate(candidates map[string]struct{}, value string) bool {
	value = version.Normalize(value)
	_, ok := candidates[value]
	return ok
}

func (c *Client) get(ctx context.Context, url string, out any) error {
	var lastErr error
	for attempt := 1; attempt <= githubRetryAttempts; attempt++ {
		log.Printf("github api request started attempt=%d url=%s auth=%t", attempt, url, c.token != "")
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("User-Agent", "opensource-release-watcher")
		if c.token != "" {
			req.Header.Set("Authorization", "Bearer "+c.token)
		}
		resp, err := c.httpClient.Do(req)
		if err != nil {
			log.Printf("github api request failed attempt=%d url=%s err=%v", attempt, url, err)
			lastErr = err
		} else {
			body, readErr := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			log.Printf("github api response attempt=%d url=%s status=%s body_bytes=%d", attempt, url, resp.Status, len(body))
			if readErr != nil {
				log.Printf("github api read body failed attempt=%d url=%s err=%v", attempt, url, readErr)
				lastErr = readErr
			} else if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				if err := json.Unmarshal(body, out); err != nil {
					log.Printf("github api decode failed attempt=%d url=%s err=%v body=%s", attempt, url, err, summarizeGitHubBody(body))
					return err
				}
				return nil
			} else {
				lastErr = fmt.Errorf("github api %s returned %s: %s", url, resp.Status, strings.TrimSpace(string(body)))
				log.Printf("github api non-2xx attempt=%d url=%s err=%v", attempt, url, lastErr)
				if !shouldRetryGitHubResponse(resp.StatusCode, body) || attempt == githubRetryAttempts {
					return lastErr
				}
			}
		}
		if attempt < githubRetryAttempts {
			if err := sleepWithContext(ctx, githubRetryDelay); err != nil {
				return err
			}
		}
	}
	return lastErr
}

func summarizeGitHubBody(body []byte) string {
	text := strings.TrimSpace(string(body))
	if text == "" {
		return "<empty>"
	}
	if len(text) > 300 {
		return text[:300] + "..."
	}
	return text
}

func shouldRetryGitHubResponse(statusCode int, body []byte) bool {
	if statusCode == http.StatusTooManyRequests || statusCode == http.StatusBadGateway || statusCode == http.StatusServiceUnavailable || statusCode == http.StatusGatewayTimeout {
		return true
	}
	if statusCode != http.StatusForbidden {
		return false
	}
	lowerBody := strings.ToLower(strings.TrimSpace(string(body)))
	return strings.Contains(lowerBody, "secondary rate limit") || strings.Contains(lowerBody, "abuse")
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
