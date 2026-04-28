package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Client struct {
	token      string
	httpClient *http.Client
}

type ReleaseInfo struct {
	Source      string
	Version     string
	Title       string
	URL         string
	PublishedAt *time.Time
	Note        string
}

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

func (c *Client) get(ctx context.Context, url string, out any) error {
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
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("github api %s returned %s", url, resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
