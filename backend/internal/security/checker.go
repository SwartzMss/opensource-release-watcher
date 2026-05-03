package security

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"opensource-release-watcher/backend/internal/osv"
	"opensource-release-watcher/backend/internal/storage"
)

type OSVClient interface {
	QueryByCommit(ctx context.Context, commitSHA string) (*osv.QueryResult, error)
}

type CommitResolver interface {
	ResolveCommit(ctx context.Context, repoURL, currentVersion, tagPattern string) (string, string, error)
}

type Checker struct {
	gitrepo CommitResolver
	osv     OSVClient
}

type Report struct {
	Profile storage.ComponentSecurityProfile
	Records []storage.ComponentSecurityRecord
}

func New(gitrepo CommitResolver, osvClient OSVClient) *Checker {
	return &Checker{gitrepo: gitrepo, osv: osvClient}
}

func (c *Checker) Check(ctx context.Context, component storage.Component, profile storage.ComponentSecurityProfile, forceResolve bool) (*Report, error) {
	startedAt := time.Now().UTC()
	log.Printf(
		"security check started component_id=%d name=%s repo=%s version=%s cached_commit=%t force_resolve=%t",
		component.ID,
		component.Name,
		component.RepoURL,
		component.CurrentVersion,
		strings.TrimSpace(profile.SecurityCommitSHA) != "",
		forceResolve,
	)
	defer func() {
		log.Printf(
			"security check finished component_id=%d status=%s reason=%s commit=%s duration=%s",
			component.ID,
			profile.LastSecurityStatus,
			profile.LastSecurityReason,
			strings.TrimSpace(profile.SecurityCommitSHA),
			time.Since(startedAt).Round(time.Millisecond),
		)
	}()
	if profile.SecurityLookupMode == "" {
		profile.SecurityLookupMode = "commit_first"
	}
	if forceResolve {
		profile.SecurityCommitSHA = ""
	}

	commitSHA := strings.TrimSpace(profile.SecurityCommitSHA)
	matchedTag := ""
	var err error
	if commitSHA == "" {
		log.Printf("security resolve commit started component_id=%d version=%s pattern=%s", component.ID, component.CurrentVersion, profile.SecurityTagPattern)
		commitSHA, matchedTag, err = c.gitrepo.ResolveCommit(ctx, component.RepoURL, component.CurrentVersion, profile.SecurityTagPattern)
		if err != nil {
			reason := "无法解析版本对应的 Git 提交"
			profile.LastSecurityStatus = "unknown"
			profile.LastSecurityReason = reason
			profile.LastSecuritySummary = reason
			profile.LastSecurityRawPayload = ""
			now := time.Now().UTC()
			profile.LastSecurityCheckedAt = &now
			profile.UpdatedAt = now
			record := securityRecord(component.ID, component.CurrentVersion, "", "unknown", reason, "", "", "", "", 0, reason, "", "")
			log.Printf("security resolve commit failed component_id=%d version=%s err=%v", component.ID, component.CurrentVersion, err)
			return &Report{Profile: profile, Records: []storage.ComponentSecurityRecord{record}}, nil
		}
		profile.SecurityCommitSHA = commitSHA
		log.Printf("security resolve commit finished component_id=%d version=%s tag=%s commit=%s", component.ID, component.CurrentVersion, matchedTag, commitSHA)
	}

	log.Printf("security osv query started component_id=%d commit=%s", component.ID, commitSHA)
	result, err := c.osv.QueryByCommit(ctx, commitSHA)
	if err != nil {
		reason := fmt.Sprintf("OSV API 查询失败: %v", err)
		now := time.Now().UTC()
		profile.LastSecurityStatus = "check_failed"
		profile.LastSecurityReason = reason
		profile.LastSecuritySummary = reason
		profile.LastSecurityRawPayload = ""
		profile.LastSecurityCheckedAt = &now
		profile.UpdatedAt = now
		record := securityRecord(component.ID, component.CurrentVersion, commitSHA, "check_failed", reason, "", "", "", "", 0, reason, "", "")
		log.Printf("security osv query failed component_id=%d commit=%s err=%v", component.ID, commitSHA, err)
		return &Report{Profile: profile, Records: []storage.ComponentSecurityRecord{record}}, nil
	}

	rawPayload := string(result.RawJSON)
	now := time.Now().UTC()
	profile.LastSecurityCheckedAt = &now
	profile.LastSecurityRawPayload = rawPayload
	profile.UpdatedAt = now
	if len(result.Vulns) == 0 {
		reason := "OSV commit 查询未命中公开已知漏洞"
		profile.LastSecurityStatus = "unknown"
		profile.LastSecurityReason = reason
		profile.LastSecuritySummary = reason
		record := securityRecord(component.ID, component.CurrentVersion, commitSHA, "unknown", reason, "", "", "", "", 0, reason, rawPayload, "")
		log.Printf("security osv query finished component_id=%d commit=%s vulns=0 status=unknown", component.ID, commitSHA)
		return &Report{Profile: profile, Records: []storage.ComponentSecurityRecord{record}}, nil
	}

	profile.LastSecurityStatus = "affected"
	profile.LastSecurityReason = "当前 commit 命中 OSV 漏洞记录"
	profile.LastSecuritySummary = firstSummary(result.Vulns)
	records := make([]storage.ComponentSecurityRecord, 0, len(result.Vulns))
	for _, vuln := range dedupeVulns(result.Vulns) {
		record := securityRecord(
			component.ID,
			component.CurrentVersion,
			commitSHA,
			"affected",
			"当前 commit 命中 OSV 漏洞记录",
			vuln.ID,
			affectedRange(vuln),
			firstFixedVersion(vuln),
			firstSeverity(vuln),
			1.0,
			summaryText(vuln),
			rawPayload,
			firstReferenceURL(vuln),
		)
		records = append(records, record)
	}
	log.Printf("security osv query finished component_id=%d commit=%s vulns=%d status=affected", component.ID, commitSHA, len(records))
	return &Report{Profile: profile, Records: records}, nil
}

func dedupeVulns(vulns []osv.Vulnerability) []osv.Vulnerability {
	seen := map[string]struct{}{}
	items := make([]osv.Vulnerability, 0, len(vulns))
	for _, vuln := range vulns {
		if vuln.ID == "" {
			continue
		}
		if _, ok := seen[vuln.ID]; ok {
			continue
		}
		seen[vuln.ID] = struct{}{}
		items = append(items, vuln)
	}
	return items
}

func firstSummary(vulns []osv.Vulnerability) string {
	for _, vuln := range vulns {
		if summary := summaryText(vuln); summary != "" {
			return summary
		}
	}
	return "当前 commit 命中 OSV 漏洞记录"
}

func summaryText(vuln osv.Vulnerability) string {
	summary := strings.TrimSpace(vuln.Summary)
	if summary != "" {
		return summary
	}
	details := strings.TrimSpace(vuln.Details)
	if details == "" {
		return vuln.ID
	}
	lines := strings.Split(details, "\n")
	for _, line := range lines {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			return trimmed
		}
	}
	return vuln.ID
}

func affectedRange(vuln osv.Vulnerability) string {
	parts := make([]string, 0, len(vuln.Affected))
	for _, affected := range vuln.Affected {
		name := strings.TrimSpace(affected.Package.Name)
		for _, rng := range affected.Ranges {
			events := make([]string, 0, len(rng.Events))
			for _, event := range rng.Events {
				if event.Introduced != "" {
					events = append(events, "introduced="+event.Introduced)
				}
				if event.Fixed != "" {
					events = append(events, "fixed="+event.Fixed)
				}
			}
			if len(events) == 0 {
				continue
			}
			prefix := strings.TrimSpace(rng.Type)
			if prefix == "" {
				prefix = "GIT"
			}
			text := prefix + ": " + strings.Join(events, " -> ")
			if name != "" {
				text = name + " " + text
			}
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "; ")
}

func firstFixedVersion(vuln osv.Vulnerability) string {
	for _, affected := range vuln.Affected {
		for _, rng := range affected.Ranges {
			for _, event := range rng.Events {
				if event.Fixed != "" {
					return event.Fixed
				}
			}
		}
	}
	return ""
}

func firstSeverity(vuln osv.Vulnerability) string {
	for _, severity := range vuln.Severities {
		if value := strings.TrimSpace(severity.Score); value != "" {
			return value
		}
		if value := strings.TrimSpace(severity.Type); value != "" {
			return value
		}
	}
	return ""
}

func firstReferenceURL(vuln osv.Vulnerability) string {
	if vuln.ID == "" {
		return ""
	}
	return "https://osv.dev/vulnerability/" + vuln.ID
}

func securityRecord(componentID int64, version, commitSHA, riskStatus, reason, identifier, affectedRange, fixedVersion, severity string, confidence float64, summary, rawPayload, evidenceURL string) storage.ComponentSecurityRecord {
	return storage.ComponentSecurityRecord{
		ComponentID:   componentID,
		Version:       version,
		CommitSHA:     commitSHA,
		RiskType:      "vulnerability",
		RiskStatus:    riskStatus,
		Source:        "osv",
		Identifier:    identifier,
		AffectedRange: affectedRange,
		FixedVersion:  fixedVersion,
		Severity:      severity,
		Confidence:    confidence,
		Summary:       summary,
		StatusReason:  reason,
		RawPayload:    rawPayload,
		EvidenceURL:   evidenceURL,
		CreatedAt:     time.Now().UTC(),
	}
}
