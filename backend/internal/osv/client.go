package osv

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
}

type QueryResult struct {
	RawJSON []byte
	Vulns   []Vulnerability
}

type Vulnerability struct {
	ID         string     `json:"id"`
	Summary    string     `json:"summary"`
	Details    string     `json:"details"`
	Affected   []Affected `json:"affected"`
	Severities []Severity `json:"severity"`
}

type Affected struct {
	Package struct {
		Ecosystem string `json:"ecosystem"`
		Name      string `json:"name"`
	} `json:"package"`
	Ranges []Range `json:"ranges"`
}

type Range struct {
	Type   string  `json:"type"`
	Repo   string  `json:"repo,omitempty"`
	Events []Event `json:"events"`
}

type Event struct {
	Introduced string `json:"introduced,omitempty"`
	Fixed      string `json:"fixed,omitempty"`
}

type Severity struct {
	Type  string `json:"type"`
	Score string `json:"score"`
}

func NewClient() *Client {
	return &Client{
		baseURL: "https://api.osv.dev",
		httpClient: &http.Client{
			Timeout: 20 * time.Second,
		},
	}
}

func (c *Client) QueryByCommit(ctx context.Context, commitSHA string) (*QueryResult, error) {
	commitSHA = strings.TrimSpace(commitSHA)
	if commitSHA == "" {
		return nil, fmt.Errorf("commit sha is required")
	}
	payload := map[string]string{"commit": commitSHA}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/query", strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "opensource-release-watcher")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("osv api returned %s", resp.Status)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var payloadResp struct {
		Vulns []Vulnerability `json:"vulns"`
	}
	if err := json.Unmarshal(raw, &payloadResp); err != nil {
		return nil, err
	}
	return &QueryResult{RawJSON: raw, Vulns: payloadResp.Vulns}, nil
}
