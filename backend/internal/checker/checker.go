package checker

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"opensource-release-watcher/backend/internal/github"
	"opensource-release-watcher/backend/internal/storage"
	"opensource-release-watcher/backend/internal/version"
)

type GitHubClient interface {
	LatestRelease(ctx context.Context, owner, repo string) (*github.ReleaseInfo, error)
	LatestTag(ctx context.Context, owner, repo string) (*github.ReleaseInfo, error)
}

type Checker struct {
	github GitHubClient
}

func New(github GitHubClient) *Checker {
	return &Checker{github: github}
}

func (c *Checker) Check(ctx context.Context, component storage.Component) storage.CheckRecord {
	record := storage.CheckRecord{
		ComponentID:     component.ID,
		PreviousVersion: component.LastSeenVersion,
		Status:          "success",
		CheckedAt:       time.Now().UTC(),
	}
	if record.PreviousVersion == "" {
		record.PreviousVersion = component.CurrentVersion
	}
	if !component.Enabled {
		record.Status = "skipped"
		return record
	}

	info, err := c.fetchLatest(ctx, component)
	if err != nil {
		record.Status = "failed"
		record.ErrorMessage = err.Error()
		return record
	}
	record.Source = info.Source
	record.LatestVersion = info.Version
	record.ReleaseTitle = info.Title
	record.ReleaseURL = info.URL
	record.ReleasePublishedAt = info.PublishedAt
	record.ReleaseNote = info.Note
	record.ReleaseNoteSummary = summarize(info.Note)
	record.HasUpdate = version.IsNewer(info.Version, record.PreviousVersion)
	return record
}

func (c *Checker) fetchLatest(ctx context.Context, component storage.Component) (*github.ReleaseInfo, error) {
	owner, repo, ok := parseGitHubURL(component.RepoURL)
	if !ok {
		return nil, fmt.Errorf("invalid GitHub repository URL: %s", component.RepoURL)
	}
	if component.CheckStrategy == "tag_only" {
		return c.github.LatestTag(ctx, owner, repo)
	}
	release, err := c.github.LatestRelease(ctx, owner, repo)
	if err == nil {
		return release, nil
	}
	tag, tagErr := c.github.LatestTag(ctx, owner, repo)
	if tagErr == nil {
		return tag, nil
	}
	return nil, errors.Join(err, tagErr)
}

func parseGitHubURL(value string) (string, string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme == "" || !strings.EqualFold(parsed.Hostname(), "github.com") {
		return "", "", false
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], strings.TrimSuffix(parts[1], ".git"), true
}

func summarize(note string) string {
	note = strings.TrimSpace(note)
	if note == "" {
		return ""
	}
	lines := strings.Split(note, "\n")
	kept := make([]string, 0, 5)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		kept = append(kept, line)
		if len(kept) == 5 {
			break
		}
	}
	return strings.Join(kept, "\n")
}
