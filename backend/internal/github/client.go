package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"opensource-release-watcher/backend/internal/version"
)

type Client struct {
	token      string
	httpClient *http.Client
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

func NewClient(token string) *Client {
	return &Client{
		token: token,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
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
			return releases, nil
		}
	}
}

func (c *Client) listTags(ctx context.Context, owner, repo string) ([]tagPayload, error) {
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

func (c *Client) FindSuggestedReleaseVersion(ctx context.Context, owner, repo, commitSHA string) (string, error) {
	commitSHA = strings.TrimSpace(commitSHA)
	if commitSHA == "" {
		return "", nil
	}
	releases, err := c.listReleases(ctx, owner, repo)
	if err != nil {
		return "", err
	}
	tags, err := c.listTags(ctx, owner, repo)
	if err != nil {
		return "", err
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
		if matched, err := c.findReleaseContainingCommit(ctx, owner, repo, commitSHA, releases, tagCommitByName, false); err != nil {
			return "", err
		} else if matched != "" {
			return matched, nil
		}
		if matched, err := c.findReleaseContainingCommit(ctx, owner, repo, commitSHA, releases, tagCommitByName, true); err != nil {
			return "", err
		} else if matched != "" {
			return matched, nil
		}
	}
	if matched, err := c.findTagContainingCommit(ctx, owner, repo, commitSHA, tags); err != nil {
		return "", err
	} else if matched != "" {
		return matched, nil
	}
	return c.FindCommitTag(ctx, owner, repo, commitSHA)
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

func (c *Client) findReleaseContainingCommit(ctx context.Context, owner, repo, commitSHA string, releases []releasePayload, tagCommitByName map[string]string, includePrereleases bool) (string, error) {
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

func (c *Client) findTagContainingCommit(ctx context.Context, owner, repo, commitSHA string, tags []tagPayload) (string, error) {
	matches := make([]string, 0)
	for _, tag := range tags {
		name := strings.TrimSpace(tag.Name)
		if name == "" {
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
			lastErr = err
		} else {
			body, readErr := io.ReadAll(io.LimitReader(resp.Body, 8192))
			_ = resp.Body.Close()
			if readErr != nil {
				lastErr = readErr
			} else if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				if err := json.Unmarshal(body, out); err != nil {
					return err
				}
				return nil
			} else {
				lastErr = fmt.Errorf("github api %s returned %s: %s", url, resp.Status, strings.TrimSpace(string(body)))
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
