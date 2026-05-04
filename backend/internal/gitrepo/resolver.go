package gitrepo

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"opensource-release-watcher/backend/internal/version"
)

type TagCommitResolver interface {
	FindTagCommit(ctx context.Context, owner, repo string, tagCandidates []string) (string, string, error)
	FindCommitTag(ctx context.Context, owner, repo, commitSHA string) (string, error)
	FindSuggestedReleaseVersion(ctx context.Context, owner, repo, commitSHA string) (string, error)
}

type Resolver struct {
	github TagCommitResolver
}

func New(github TagCommitResolver) *Resolver {
	return &Resolver{github: github}
}

func (r *Resolver) ResolveCommit(ctx context.Context, repoURL, currentVersion, tagPattern string) (string, string, error) {
	owner, repo, ok := parseGitHubURL(repoURL)
	if !ok {
		return "", "", fmt.Errorf("invalid GitHub repository URL: %s", repoURL)
	}
	candidates := tagCandidates(currentVersion, tagPattern)
	if len(candidates) == 0 {
		return "", "", fmt.Errorf("tag candidates are required")
	}
	return r.github.FindTagCommit(ctx, owner, repo, candidates)
}

func (r *Resolver) ResolveTag(ctx context.Context, repoURL, commitSHA string) (string, error) {
	owner, repo, ok := parseGitHubURL(repoURL)
	if !ok {
		return "", fmt.Errorf("invalid GitHub repository URL: %s", repoURL)
	}
	return r.github.FindCommitTag(ctx, owner, repo, commitSHA)
}

func (r *Resolver) ResolveSuggestedVersion(ctx context.Context, repoURL, commitSHA string) (string, error) {
	owner, repo, ok := parseGitHubURL(repoURL)
	if !ok {
		return "", fmt.Errorf("invalid GitHub repository URL: %s", repoURL)
	}
	return r.github.FindSuggestedReleaseVersion(ctx, owner, repo, commitSHA)
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

func tagCandidates(currentVersion, pattern string) []string {
	base := version.Normalize(currentVersion)
	if base == "" {
		return nil
	}
	values := make([]string, 0, 4)
	appendUnique := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		for _, existing := range values {
			if existing == value {
				return
			}
		}
		values = append(values, value)
	}
	if pattern != "" {
		for _, item := range splitPatterns(pattern) {
			appendUnique(strings.ReplaceAll(item, "{version}", base))
		}
	}
	appendUnique(base)
	appendUnique("v" + base)
	return values
}

func splitPatterns(pattern string) []string {
	fields := strings.FieldsFunc(pattern, func(r rune) bool {
		return r == ',' || r == '\n' || r == ';'
	})
	items := make([]string, 0, len(fields))
	for _, field := range fields {
		if trimmed := strings.TrimSpace(field); trimmed != "" {
			items = append(items, trimmed)
		}
	}
	return items
}
