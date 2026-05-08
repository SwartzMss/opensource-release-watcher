package storage

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

func (s *Store) ListComponentSecurityRecords(ctx context.Context, componentID int64) ([]ComponentSecurityRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT rs.id, rs.run_id, rs.component_id, c.name AS component_name, rs.version, rs.commit_sha, rs.risk_type, rs.risk_status, rs.source, rs.identifier,
		       rs.affected_range, rs.fixed_version, rs.severity, rs.confidence, rs.summary, rs.status_reason,
		       '', rs.evidence_url, rs.created_at
		FROM component_security_records rs
		JOIN components c ON c.id = rs.component_id
		WHERE rs.component_id = ?
		ORDER BY rs.created_at DESC, rs.id DESC`, componentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]ComponentSecurityRecord, 0)
	for rows.Next() {
		var record ComponentSecurityRecord
		var runID sql.NullInt64
		var commitSHA, identifier, affectedRange, fixedVersion, severity, summary, statusReason, rawPayload, evidenceURL sql.NullString
		if err := rows.Scan(
			&record.ID, &runID, &record.ComponentID, &record.ComponentName, &record.Version, &commitSHA, &record.RiskType, &record.RiskStatus, &record.Source, &identifier,
			&affectedRange, &fixedVersion, &severity, &record.Confidence, &summary, &statusReason,
			&rawPayload, &evidenceURL, &record.CreatedAt,
		); err != nil {
			return nil, err
		}
		record.RunID = runID.Int64
		record.CommitSHA = commitSHA.String
		record.Identifier = identifier.String
		record.AffectedRange = affectedRange.String
		record.FixedVersion = fixedVersion.String
		record.Severity = severity.String
		record.Summary = summary.String
		record.StatusReason = statusReason.String
		record.RawPayload = rawPayload.String
		record.EvidenceURL = evidenceURL.String
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

func (s *Store) ListSecurityRecords(ctx context.Context, opts ListOptions) ([]ComponentSecurityRecord, int, error) {
	clauses := []string{"1 = 1"}
	args := []any{}
	if opts.ComponentID > 0 {
		clauses = append(clauses, "rs.component_id = ?")
		args = append(args, opts.ComponentID)
	}
	if opts.SecurityStatus != "" {
		clauses = append(clauses, "rs.risk_status = ?")
		args = append(args, opts.SecurityStatus)
	}
	where := strings.Join(clauses, " AND ")
	var total int
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM component_security_records rs
		WHERE `+where+``, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	limit, offset := opts.LimitOffset()
	queryArgs := append(args, limit, offset)
	rows, err := s.db.QueryContext(ctx, `
		SELECT rs.id, rs.run_id, rs.component_id, c.name AS component_name, rs.version, rs.commit_sha, rs.risk_type, rs.risk_status, rs.source, rs.identifier,
		       rs.affected_range, rs.fixed_version, rs.severity, rs.confidence, rs.summary, rs.status_reason,
		       '', rs.evidence_url, rs.created_at
		FROM component_security_records rs
		JOIN components c ON c.id = rs.component_id
		WHERE `+where+`
		ORDER BY rs.created_at DESC, rs.id DESC LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	items := []ComponentSecurityRecord{}
	for rows.Next() {
		var record ComponentSecurityRecord
		var runID sql.NullInt64
		var commitSHA, identifier, affectedRange, fixedVersion, severity, summary, statusReason, rawPayload, evidenceURL sql.NullString
		if err := rows.Scan(
			&record.ID, &runID, &record.ComponentID, &record.ComponentName, &record.Version, &commitSHA, &record.RiskType, &record.RiskStatus, &record.Source, &identifier,
			&affectedRange, &fixedVersion, &severity, &record.Confidence, &summary, &statusReason,
			&rawPayload, &evidenceURL, &record.CreatedAt,
		); err != nil {
			return nil, 0, err
		}
		record.RunID = runID.Int64
		record.CommitSHA = commitSHA.String
		record.Identifier = identifier.String
		record.AffectedRange = affectedRange.String
		record.FixedVersion = fixedVersion.String
		record.Severity = severity.String
		record.Summary = summary.String
		record.StatusReason = statusReason.String
		record.RawPayload = rawPayload.String
		record.EvidenceURL = evidenceURL.String
		items = append(items, record)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (s *Store) GetComponentSecurityProfile(ctx context.Context, componentID int64) (*ComponentSecurityProfile, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT component_id, security_mode, security_lookup_mode, security_commit_sha,
		       security_suggested_version, security_package_name, security_ecosystem, security_aliases, security_notes,
		       security_tag_pattern, last_security_status, last_security_reason,
		       last_security_raw_payload, last_security_checked_at, last_security_summary,
		       created_at, updated_at
		FROM component_security_profiles
		WHERE component_id = ?`, componentID)
	var item ComponentSecurityProfile
	var securityMode, securityLookupMode, securityCommitSHA, securitySuggestedVersion, securityPackageName, securityEcosystem, securityAliases, securityNotes, securityTagPattern sql.NullString
	var lastSecurityStatus, lastSecurityReason, lastSecurityRawPayload, lastSecuritySummary sql.NullString
	var lastSecurityCheckedAt sql.NullTime
	if err := row.Scan(
		&item.ComponentID, &securityMode, &securityLookupMode, &securityCommitSHA,
		&securitySuggestedVersion, &securityPackageName, &securityEcosystem, &securityAliases, &securityNotes,
		&securityTagPattern, &lastSecurityStatus, &lastSecurityReason,
		&lastSecurityRawPayload, &lastSecurityCheckedAt, &lastSecuritySummary,
		&item.CreatedAt, &item.UpdatedAt,
	); err != nil {
		return nil, err
	}
	item.SecurityMode = securityMode.String
	item.SecurityLookupMode = securityLookupMode.String
	item.SecurityCommitSHA = securityCommitSHA.String
	item.SecuritySuggestedVersion = securitySuggestedVersion.String
	item.SecurityPackageName = securityPackageName.String
	item.SecurityEcosystem = securityEcosystem.String
	item.SecurityAliases = securityAliases.String
	item.SecurityNotes = securityNotes.String
	item.SecurityTagPattern = securityTagPattern.String
	item.LastSecurityStatus = lastSecurityStatus.String
	item.LastSecurityReason = lastSecurityReason.String
	item.LastSecurityRawPayload = lastSecurityRawPayload.String
	item.LastSecurityCheckedAt = nullTimePtr(lastSecurityCheckedAt)
	item.LastSecuritySummary = lastSecuritySummary.String
	return &item, nil
}

func (s *Store) ClearComponentSecurityCommitCache(ctx context.Context, componentID int64) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO component_security_profiles (
			component_id, security_lookup_mode, security_commit_sha, security_suggested_version, created_at, updated_at
		) VALUES (?, 'commit_first', '', '', ?, ?)
		ON CONFLICT(component_id) DO UPDATE SET
			security_commit_sha = '',
			security_suggested_version = '',
			updated_at = excluded.updated_at`,
		componentID, now, now,
	)
	return err
}

func (s *Store) SaveComponentSecurityState(ctx context.Context, profile ComponentSecurityProfile, records []ComponentSecurityRecord) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	var commitErr error
	defer func() {
		if commitErr != nil {
			_ = tx.Rollback()
		}
	}()

	now := time.Now().UTC()
	if profile.SecurityLookupMode == "" {
		profile.SecurityLookupMode = "commit_first"
	}
	if profile.CreatedAt.IsZero() {
		profile.CreatedAt = now
	}
	profile.UpdatedAt = now
	_, err = tx.ExecContext(ctx, `
		INSERT INTO component_security_profiles (
			component_id, security_mode, security_lookup_mode, security_commit_sha,
			security_suggested_version, security_package_name, security_ecosystem, security_aliases, security_notes,
			security_tag_pattern, last_security_status, last_security_reason,
			last_security_raw_payload, last_security_checked_at, last_security_summary,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(component_id) DO UPDATE SET
			security_mode = excluded.security_mode,
			security_lookup_mode = excluded.security_lookup_mode,
			security_commit_sha = excluded.security_commit_sha,
			security_suggested_version = excluded.security_suggested_version,
			security_package_name = excluded.security_package_name,
			security_ecosystem = excluded.security_ecosystem,
			security_aliases = excluded.security_aliases,
			security_notes = excluded.security_notes,
			security_tag_pattern = excluded.security_tag_pattern,
			last_security_status = excluded.last_security_status,
			last_security_reason = excluded.last_security_reason,
			last_security_raw_payload = excluded.last_security_raw_payload,
			last_security_checked_at = excluded.last_security_checked_at,
			last_security_summary = excluded.last_security_summary,
			updated_at = excluded.updated_at`,
		profile.ComponentID, nullableString(profile.SecurityMode), nullableString(profile.SecurityLookupMode), nullableString(profile.SecurityCommitSHA),
		nullableString(profile.SecuritySuggestedVersion), nullableString(profile.SecurityPackageName), nullableString(profile.SecurityEcosystem), nullableString(profile.SecurityAliases), nullableString(profile.SecurityNotes),
		nullableString(profile.SecurityTagPattern), nullableString(profile.LastSecurityStatus), nullableString(profile.LastSecurityReason),
		nullableString(profile.LastSecurityRawPayload), profile.LastSecurityCheckedAt, nullableString(profile.LastSecuritySummary),
		profile.CreatedAt, profile.UpdatedAt,
	)
	if err != nil {
		commitErr = err
		return commitErr
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM component_security_records WHERE component_id = ?`, profile.ComponentID); err != nil {
		commitErr = err
		return commitErr
	}
	for _, record := range records {
		if record.CreatedAt.IsZero() {
			record.CreatedAt = now
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO component_security_records (
				run_id, component_id, version, commit_sha, risk_type, risk_status, source, identifier,
				package_name, ecosystem, affected_range, fixed_version, severity, confidence,
				summary, status_reason, raw_payload, evidence_url, created_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			nullableInt64(record.RunID), record.ComponentID, record.Version, nullableString(record.CommitSHA), record.RiskType, record.RiskStatus, record.Source, nullableString(record.Identifier),
			nil, nil, nullableString(record.AffectedRange), nullableString(record.FixedVersion), nullableString(record.Severity), record.Confidence,
			nullableString(record.Summary), nullableString(record.StatusReason), nullableString(record.RawPayload), nullableString(record.EvidenceURL), record.CreatedAt,
		)
		if err != nil {
			commitErr = err
			return commitErr
		}
	}
	commitErr = tx.Commit()
	return commitErr
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
