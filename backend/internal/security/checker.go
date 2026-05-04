package security

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"opensource-release-watcher/backend/internal/osv"
	"opensource-release-watcher/backend/internal/storage"
	"opensource-release-watcher/backend/internal/version"
)

type OSVClient interface {
	QueryByCommit(ctx context.Context, commitSHA string) (*osv.QueryResult, error)
}

type CommitResolver interface {
	ResolveCommit(ctx context.Context, repoURL, currentVersion, tagPattern string) (string, string, error)
	ResolveTag(ctx context.Context, repoURL, commitSHA string) (string, error)
	ResolveSuggestedVersion(ctx context.Context, repoURL, currentVersion, commitSHA string) (string, error)
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
			profile.SecuritySuggestedVersion = ""
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
		profile.SecuritySuggestedVersion = ""
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
		profile.SecuritySuggestedVersion = ""
		record := securityRecord(component.ID, component.CurrentVersion, commitSHA, "unknown", reason, "", "", "", "", 0, reason, rawPayload, "")
		log.Printf("security osv query finished component_id=%d commit=%s vulns=0 status=unknown", component.ID, commitSHA)
		return &Report{Profile: profile, Records: []storage.ComponentSecurityRecord{record}}, nil
	}

	profile.LastSecurityStatus = "affected"
	profile.LastSecurityReason = "当前 commit 命中 OSV 漏洞记录"
	profile.LastSecuritySummary = firstSummary(result.Vulns)
	records := make([]storage.ComponentSecurityRecord, 0, len(result.Vulns))
	suggestedVersions := make([]string, 0, len(result.Vulns))
	for _, vuln := range dedupeVulns(result.Vulns) {
		vulnFixedVersions := fixedVersionsForVuln(vuln)
		vulnSuggestedVersion := c.resolveSuggestedVersion(ctx, component.RepoURL, component.CurrentVersion, vulnFixedVersions)
		if vulnSuggestedVersion != "" {
			suggestedVersions = append(suggestedVersions, vulnSuggestedVersion)
		}
		record := securityRecord(
			component.ID,
			component.CurrentVersion,
			commitSHA,
			"affected",
			"当前 commit 命中 OSV 漏洞记录",
			vuln.ID,
			affectedRange(vuln),
			vulnSuggestedVersion,
			firstSeverity(vuln),
			1.0,
			summaryText(vuln),
			rawPayload,
			firstReferenceURL(vuln),
		)
		records = append(records, record)
	}
	profile.SecuritySuggestedVersion = highestVersion(suggestedVersions)
	log.Printf("security osv query finished component_id=%d commit=%s vulns=%d status=affected", component.ID, commitSHA, len(records))
	return &Report{Profile: profile, Records: records}, nil
}

func (c *Checker) resolveSuggestedVersion(ctx context.Context, repoURL, currentVersion string, fixedVersions []string) string {
	cache := map[string]string{}
	best := ""
	for _, fixedVersion := range dedupeStrings(fixedVersions) {
		log.Printf("security resolving fixed version candidate repo=%s fixed_version=%s", repoURL, fixedVersion)
		resolved := c.resolveFixedVersionCandidate(ctx, repoURL, currentVersion, fixedVersion, cache)
		log.Printf("security resolved fixed version candidate repo=%s fixed_version=%s resolved=%s", repoURL, fixedVersion, resolved)
		if resolved == "" || looksLikeCommitSHA(resolved) {
			continue
		}
		if currentVersion != "" && !version.IsNewer(resolved, currentVersion) {
			log.Printf("security suggested version candidate filtered repo=%s fixed_version=%s resolved=%s current_version=%s", repoURL, fixedVersion, resolved, currentVersion)
			continue
		}
		if best == "" || version.IsNewer(best, resolved) {
			best = resolved
			log.Printf("security suggested version updated repo=%s fixed_version=%s selected=%s", repoURL, fixedVersion, best)
		}
	}
	log.Printf("security suggested version final repo=%s selected=%s", repoURL, best)
	return best
}

func (c *Checker) resolveFixedVersionCandidate(ctx context.Context, repoURL, currentVersion, fixedVersion string, cache map[string]string) string {
	fixedVersion = strings.TrimSpace(fixedVersion)
	if fixedVersion == "" {
		return ""
	}
	if normalized, ok := cache[fixedVersion]; ok {
		return normalized
	}
	if !looksLikeCommitSHA(fixedVersion) {
		log.Printf("security fixed version is already a version repo=%s fixed_version=%s", repoURL, fixedVersion)
		if currentVersion != "" && !version.IsNewer(fixedVersion, currentVersion) {
			cache[fixedVersion] = ""
			return ""
		}
		cache[fixedVersion] = fixedVersion
		return fixedVersion
	}
	tag, err := c.gitrepo.ResolveSuggestedVersion(ctx, repoURL, currentVersion, fixedVersion)
	if err != nil {
		log.Printf("security resolve fixed version suggestion failed repo=%s fixed_version=%s err=%v", repoURL, fixedVersion, err)
		cache[fixedVersion] = ""
		return ""
	}
	if currentVersion != "" && tag != "" && !version.IsNewer(tag, currentVersion) {
		log.Printf("security resolved fixed version not newer than current repo=%s fixed_version=%s tag=%s current_version=%s", repoURL, fixedVersion, tag, currentVersion)
		cache[fixedVersion] = ""
		return ""
	}
	cache[fixedVersion] = tag
	log.Printf("security resolved fixed version from commit repo=%s fixed_version=%s tag=%s", repoURL, fixedVersion, tag)
	return tag
}

func looksLikeCommitSHA(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) != 40 {
		return false
	}
	for _, ch := range value {
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') && (ch < 'A' || ch > 'F') {
			return false
		}
	}
	return true
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

func dedupeStrings(values []string) []string {
	seen := map[string]struct{}{}
	items := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		items = append(items, value)
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

func fixedVersionsForVuln(vuln osv.Vulnerability) []string {
	values := make([]string, 0)
	for _, affected := range vuln.Affected {
		for _, rng := range affected.Ranges {
			for _, event := range rng.Events {
				if event.Fixed != "" {
					values = append(values, strings.TrimSpace(event.Fixed))
				}
			}
		}
	}
	return dedupeStrings(values)
}

func highestVersion(values []string) string {
	best := ""
	for _, value := range dedupeStrings(values) {
		if value == "" {
			continue
		}
		if best == "" || version.IsNewer(value, best) {
			best = value
		}
	}
	return best
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
