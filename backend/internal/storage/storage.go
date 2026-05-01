package storage

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	store := &Store{db: db}
	if err := store.init(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) init(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, schema)
	if err != nil {
		return err
	}
	if err := s.migrateSubscriberSchema(ctx); err != nil {
		return err
	}
	return s.migrateLegacySubscribers(ctx)
}

func (s *Store) migrateSubscriberSchema(ctx context.Context) error {
	hasColumn, err := s.hasColumn(ctx, "global_subscribers", "all_components")
	if err != nil {
		return err
	}
	if !hasColumn {
		if _, err := s.db.ExecContext(ctx, `ALTER TABLE global_subscribers ADD COLUMN all_components INTEGER NOT NULL DEFAULT 0`); err != nil {
			return err
		}
		if _, err := s.db.ExecContext(ctx, `UPDATE global_subscribers SET all_components = 1`); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) migrateLegacySubscribers(ctx context.Context) error {
	hasTable, err := s.hasTable(ctx, "subscribers")
	if err != nil || !hasTable {
		return err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, component_id, name, email, enabled FROM subscribers`)
	if err != nil {
		return err
	}
	defer rows.Close()
	type legacySubscriber struct {
		name         string
		email        string
		enabled      bool
		componentIDs map[int64]struct{}
	}
	legacy := map[string]*legacySubscriber{}
	for rows.Next() {
		var legacyID, componentID int64
		var name, email string
		var enabled int
		if err := rows.Scan(&legacyID, &componentID, &name, &email, &enabled); err != nil {
			return err
		}
		item := legacy[email]
		if item == nil {
			item = &legacySubscriber{name: name, email: email, componentIDs: map[int64]struct{}{}}
			legacy[email] = item
		}
		if enabled == 1 {
			item.enabled = true
		}
		item.componentIDs[componentID] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, item := range legacy {
		subscriberID, err := s.upsertGlobalSubscriber(ctx, item.name, item.email, item.enabled, false)
		if err != nil {
			return err
		}
		componentIDs := make([]int64, 0, len(item.componentIDs))
		for componentID := range item.componentIDs {
			componentIDs = append(componentIDs, componentID)
		}
		if err := s.replaceGlobalSubscriberComponents(ctx, subscriberID, componentIDs, false); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) hasTable(ctx context.Context, table string) (bool, error) {
	var name string
	err := s.db.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type='table' AND name = ?`, table).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return name == table, nil
}

func (s *Store) hasColumn(ctx context.Context, table, column string) (bool, error) {
	rows, err := s.db.QueryContext(ctx, `PRAGMA table_info(`+table+`)`)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull int
		var defaultValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &colType, &notNull, &defaultValue, &pk); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}

func (s *Store) CreateComponent(ctx context.Context, c *Component) error {
	now := time.Now().UTC()
	if c.CheckStrategy == "" {
		c.CheckStrategy = "release_first"
	}
	c.RepoURL = repoURL(c)
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO components (
			name, repo_url, current_version,
			check_strategy, enabled, notes, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		c.Name, c.RepoURL, c.CurrentVersion,
		c.CheckStrategy, boolInt(c.Enabled), c.Notes, now, now,
	)
	if err != nil {
		return err
	}
	c.ID, err = result.LastInsertId()
	c.CreatedAt = now
	c.UpdatedAt = now
	return err
}

func (s *Store) UpdateComponent(ctx context.Context, c *Component) error {
	now := time.Now().UTC()
	c.RepoURL = repoURL(c)
	result, err := s.db.ExecContext(ctx, `
		UPDATE components
		SET name = ?, repo_url = ?, current_version = ?,
		    check_strategy = ?, enabled = ?, notes = ?, updated_at = ?
		WHERE id = ?`,
		c.Name, c.RepoURL, c.CurrentVersion,
		c.CheckStrategy, boolInt(c.Enabled), c.Notes, now, c.ID,
	)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) DeleteComponent(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM components WHERE id = ?`, id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) GetComponent(ctx context.Context, id int64) (*Component, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, repo_url, current_version, latest_version,
		       last_seen_version, check_strategy, enabled,
		       last_check_status, last_check_error, last_checked_at, notes, created_at, updated_at
		FROM components WHERE id = ?`, id)
	return scanComponent(row)
}

func (s *Store) ListComponents(ctx context.Context, opts ListOptions) ([]Component, int, error) {
	clauses := []string{"1 = 1"}
	args := []any{}
	if opts.Keyword != "" {
		clauses = append(clauses, "(name LIKE ? OR repo_url LIKE ?)")
		keyword := "%" + opts.Keyword + "%"
		args = append(args, keyword, keyword)
	}
	if opts.Enabled != nil {
		clauses = append(clauses, "enabled = ?")
		args = append(args, boolInt(*opts.Enabled))
	}
	where := strings.Join(clauses, " AND ")
	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM components WHERE "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	limit, offset := opts.LimitOffset()
	queryArgs := append(args, limit, offset)
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, repo_url, current_version, latest_version,
		       last_seen_version, check_strategy, enabled,
		       last_check_status, last_check_error, last_checked_at, notes, created_at, updated_at
		FROM components WHERE `+where+`
		ORDER BY updated_at DESC LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	items := []Component{}
	for rows.Next() {
		item, err := scanComponent(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, *item)
	}
	return items, total, rows.Err()
}

func (s *Store) ListEnabledComponents(ctx context.Context) ([]Component, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, repo_url, current_version, latest_version,
		       last_seen_version, check_strategy, enabled,
		       last_check_status, last_check_error, last_checked_at, notes, created_at, updated_at
		FROM components WHERE enabled = 1 ORDER BY name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Component{}
	for rows.Next() {
		item, err := scanComponent(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	return items, rows.Err()
}

func (s *Store) CreateSubscriber(ctx context.Context, sub *Subscriber) error {
	now := time.Now().UTC()
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO subscribers (component_id, name, email, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		sub.ComponentID, sub.Name, sub.Email, boolInt(sub.Enabled), now, now,
	)
	if err != nil {
		return err
	}
	sub.ID, err = result.LastInsertId()
	sub.CreatedAt = now
	sub.UpdatedAt = now
	return err
}

func (s *Store) CreateGlobalSubscriber(ctx context.Context, sub *GlobalSubscriber) error {
	id, err := s.upsertGlobalSubscriber(ctx, sub.Name, sub.Email, sub.Enabled, sub.AllComponents)
	if err != nil {
		return err
	}
	sub.ID = id
	now := time.Now().UTC()
	sub.CreatedAt = now
	sub.UpdatedAt = now
	return nil
}

func (s *Store) UpdateSubscriber(ctx context.Context, sub *Subscriber) error {
	now := time.Now().UTC()
	result, err := s.db.ExecContext(ctx, `
		UPDATE subscribers SET name = ?, email = ?, enabled = ?, updated_at = ? WHERE id = ?`,
		sub.Name, sub.Email, boolInt(sub.Enabled), now, sub.ID,
	)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) UpdateGlobalSubscriber(ctx context.Context, sub *GlobalSubscriber) error {
	now := time.Now().UTC()
	result, err := s.db.ExecContext(ctx, `
		UPDATE global_subscribers SET name = ?, email = ?, enabled = ?, all_components = ?, updated_at = ? WHERE id = ?`,
		sub.Name, sub.Email, boolInt(sub.Enabled), boolInt(sub.AllComponents), now, sub.ID,
	)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) DeleteSubscriber(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM subscribers WHERE id = ?`, id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) DeleteGlobalSubscriber(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM global_subscribers WHERE id = ?`, id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) ListSubscribers(ctx context.Context, componentID int64) ([]Subscriber, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, component_id, name, email, enabled, created_at, updated_at
		FROM subscribers WHERE component_id = ? ORDER BY created_at DESC`, componentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Subscriber{}
	for rows.Next() {
		var item Subscriber
		var enabled int
		if err := rows.Scan(&item.ID, &item.ComponentID, &item.Name, &item.Email, &enabled, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		item.Enabled = enabled == 1
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) ListGlobalSubscribers(ctx context.Context) ([]GlobalSubscriber, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT g.id, g.name, g.email, g.enabled, g.all_components, g.created_at, g.updated_at,
		       COALESCE(group_concat(gsc.component_id), '')
		FROM global_subscribers g
		LEFT JOIN global_subscriber_components gsc ON gsc.subscriber_id = g.id
		GROUP BY g.id
		ORDER BY g.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []GlobalSubscriber{}
	for rows.Next() {
		var item GlobalSubscriber
		var enabled int
		var allComponents int
		var componentIDs string
		if err := rows.Scan(&item.ID, &item.Name, &item.Email, &enabled, &allComponents, &item.CreatedAt, &item.UpdatedAt, &componentIDs); err != nil {
			return nil, err
		}
		item.Enabled = enabled == 1
		item.AllComponents = allComponents == 1
		item.ComponentIDs = parseIDList(componentIDs)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) GetGlobalSubscriber(ctx context.Context, id int64) (*GlobalSubscriber, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT g.id, g.name, g.email, g.enabled, g.all_components, g.created_at, g.updated_at,
		       COALESCE(group_concat(gsc.component_id), '')
		FROM global_subscribers g
		LEFT JOIN global_subscriber_components gsc ON gsc.subscriber_id = g.id
		WHERE g.id = ?
		GROUP BY g.id`, id)
	var item GlobalSubscriber
	var enabled int
	var allComponents int
	var componentIDs string
	if err := row.Scan(&item.ID, &item.Name, &item.Email, &enabled, &allComponents, &item.CreatedAt, &item.UpdatedAt, &componentIDs); err != nil {
		return nil, err
	}
	item.Enabled = enabled == 1
	item.AllComponents = allComponents == 1
	item.ComponentIDs = parseIDList(componentIDs)
	return &item, nil
}

func (s *Store) SetGlobalSubscriberComponents(ctx context.Context, subscriberID int64, allComponents bool, componentIDs []int64) error {
	return s.replaceGlobalSubscriberComponents(ctx, subscriberID, componentIDs, allComponents)
}

func (s *Store) ListSubscriberRecipients(ctx context.Context, componentID int64) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT g.email
		FROM global_subscribers g
		LEFT JOIN global_subscriber_components gsc ON gsc.subscriber_id = g.id
		WHERE g.enabled = 1
		  AND (
			g.all_components = 1
			OR gsc.component_id = ?
		  )
		ORDER BY g.email ASC`, componentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	recipients := []string{}
	for rows.Next() {
		var email string
		if err := rows.Scan(&email); err != nil {
			return nil, err
		}
		recipients = append(recipients, email)
	}
	return recipients, rows.Err()
}

func (s *Store) upsertGlobalSubscriber(ctx context.Context, name, email string, enabled, allComponents bool) (int64, error) {
	now := time.Now().UTC()
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO global_subscribers (name, email, enabled, all_components, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(email) DO UPDATE SET
			name = excluded.name,
			enabled = excluded.enabled,
			all_components = CASE
				WHEN excluded.all_components = 1 THEN 1
				ELSE global_subscribers.all_components
			END,
			updated_at = excluded.updated_at`,
		name, email, boolInt(enabled), boolInt(allComponents), now, now,
	)
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	if err != nil || id != 0 {
		return id, err
	}
	var existingID int64
	if err := s.db.QueryRowContext(ctx, `SELECT id FROM global_subscribers WHERE email = ?`, email).Scan(&existingID); err != nil {
		return 0, err
	}
	return existingID, nil
}

func (s *Store) replaceGlobalSubscriberComponents(ctx context.Context, subscriberID int64, componentIDs []int64, allComponents bool) error {
	now := time.Now().UTC()
	if _, err := s.db.ExecContext(ctx, `UPDATE global_subscribers SET all_components = ?, updated_at = ? WHERE id = ?`,
		boolInt(allComponents), now, subscriberID); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM global_subscriber_components WHERE subscriber_id = ?`, subscriberID); err != nil {
		return err
	}
	if allComponents {
		return nil
	}
	for _, componentID := range componentIDs {
		if componentID <= 0 {
			continue
		}
		if _, err := s.db.ExecContext(ctx, `
			INSERT OR IGNORE INTO global_subscriber_components (subscriber_id, component_id, created_at)
			VALUES (?, ?, ?)`, subscriberID, componentID, now); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) CreateCheckRecord(ctx context.Context, record *CheckRecord) error {
	if record.CheckedAt.IsZero() {
		record.CheckedAt = time.Now().UTC()
	}
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO check_records (
			component_id, source, previous_version, latest_version, release_title, release_url,
			release_published_at, release_note, release_note_summary, has_update, status, error_message, checked_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		record.ComponentID, record.Source, record.PreviousVersion, record.LatestVersion, record.ReleaseTitle, record.ReleaseURL,
		record.ReleasePublishedAt, record.ReleaseNote, record.ReleaseNoteSummary, boolInt(record.HasUpdate), record.Status, record.ErrorMessage, record.CheckedAt,
	)
	if err != nil {
		return err
	}
	record.ID, err = result.LastInsertId()
	return err
}

func (s *Store) ListCheckRecords(ctx context.Context, opts ListOptions) ([]CheckRecord, int, error) {
	clauses := []string{"1 = 1"}
	args := []any{}
	if opts.ComponentID > 0 {
		clauses = append(clauses, "cr.component_id = ?")
		args = append(args, opts.ComponentID)
	}
	if opts.Status != "" {
		clauses = append(clauses, "cr.status = ?")
		args = append(args, opts.Status)
	}
	if opts.HasUpdate != nil {
		clauses = append(clauses, "cr.has_update = ?")
		args = append(args, boolInt(*opts.HasUpdate))
	}
	where := strings.Join(clauses, " AND ")
	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM check_records cr WHERE "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	limit, offset := opts.LimitOffset()
	queryArgs := append(args, limit, offset)
	rows, err := s.db.QueryContext(ctx, `
		SELECT cr.id, cr.component_id, c.name, cr.source, cr.previous_version, cr.latest_version,
		       cr.release_title, cr.release_url, cr.release_published_at, cr.release_note,
		       cr.release_note_summary, cr.has_update, cr.status, cr.error_message, cr.checked_at
		FROM check_records cr
		JOIN components c ON c.id = cr.component_id
		WHERE `+where+`
		ORDER BY cr.checked_at DESC LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := []CheckRecord{}
	for rows.Next() {
		item, err := scanCheckRecord(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, *item)
	}
	return items, total, rows.Err()
}

func (s *Store) GetCheckRecord(ctx context.Context, id int64) (*CheckRecord, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT cr.id, cr.component_id, c.name, cr.source, cr.previous_version, cr.latest_version,
		       cr.release_title, cr.release_url, cr.release_published_at, cr.release_note,
		       cr.release_note_summary, cr.has_update, cr.status, cr.error_message, cr.checked_at
		FROM check_records cr
		JOIN components c ON c.id = cr.component_id
		WHERE cr.id = ?`, id)
	return scanCheckRecord(row)
}

func (s *Store) CreateNotificationRecord(ctx context.Context, record *NotificationRecord) error {
	now := time.Now().UTC()
	record.CreatedAt = now
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO notification_records (
			component_id, check_record_id, version, recipient_email, subject, body, status, error_message, sent_at, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(component_id, version, recipient_email) DO UPDATE SET
			check_record_id = excluded.check_record_id,
			subject = excluded.subject,
			body = excluded.body,
			status = excluded.status,
			error_message = excluded.error_message,
			sent_at = excluded.sent_at,
			created_at = excluded.created_at`,
		record.ComponentID, record.CheckRecordID, record.Version, record.RecipientEmail, record.Subject,
		record.Body, record.Status, record.ErrorMessage, record.SentAt, now,
	)
	if err != nil {
		return err
	}
	record.ID, err = result.LastInsertId()
	return err
}

func (s *Store) HasSentNotification(ctx context.Context, componentID int64, version string) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM notification_records
		WHERE component_id = ? AND version = ? AND status = 'sent'`, componentID, version).Scan(&count)
	return count > 0, err
}

func (s *Store) ListNotificationRecords(ctx context.Context, opts ListOptions) ([]NotificationRecord, int, error) {
	clauses := []string{"1 = 1"}
	args := []any{}
	if opts.ComponentID > 0 {
		clauses = append(clauses, "nr.component_id = ?")
		args = append(args, opts.ComponentID)
	}
	if opts.Status != "" {
		clauses = append(clauses, "nr.status = ?")
		args = append(args, opts.Status)
	}
	if opts.RecipientEmail != "" {
		clauses = append(clauses, "nr.recipient_email = ?")
		args = append(args, opts.RecipientEmail)
	}
	where := strings.Join(clauses, " AND ")
	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM notification_records nr WHERE "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	limit, offset := opts.LimitOffset()
	queryArgs := append(args, limit, offset)
	rows, err := s.db.QueryContext(ctx, `
		SELECT nr.id, nr.component_id, c.name, nr.check_record_id, nr.version, nr.recipient_email,
		       nr.subject, '', nr.status, nr.error_message, nr.sent_at, nr.created_at
		FROM notification_records nr
		JOIN components c ON c.id = nr.component_id
		WHERE `+where+`
		ORDER BY nr.created_at DESC LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := []NotificationRecord{}
	for rows.Next() {
		item, err := scanNotificationRecord(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, *item)
	}
	return items, total, rows.Err()
}

func (s *Store) GetNotificationRecord(ctx context.Context, id int64) (*NotificationRecord, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT nr.id, nr.component_id, c.name, nr.check_record_id, nr.version, nr.recipient_email,
		       nr.subject, nr.body, nr.status, nr.error_message, nr.sent_at, nr.created_at
		FROM notification_records nr
		JOIN components c ON c.id = nr.component_id
		WHERE nr.id = ?`, id)
	return scanNotificationRecord(row)
}

func (s *Store) CreateSystemRun(ctx context.Context, run *SystemRun) error {
	if run.StartedAt.IsZero() {
		run.StartedAt = time.Now().UTC()
	}
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO system_runs (trigger_type, status, total_count, success_count, failed_count, started_at, finished_at, error_message)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		run.TriggerType, run.Status, run.TotalCount, run.SuccessCount, run.FailedCount, run.StartedAt, run.FinishedAt, run.ErrorMessage,
	)
	if err != nil {
		return err
	}
	run.ID, err = result.LastInsertId()
	return err
}

func (s *Store) FinishSystemRun(ctx context.Context, run *SystemRun) error {
	now := time.Now().UTC()
	run.FinishedAt = &now
	_, err := s.db.ExecContext(ctx, `
		UPDATE system_runs
		SET status = ?, total_count = ?, success_count = ?, failed_count = ?, finished_at = ?, error_message = ?
		WHERE id = ?`,
		run.Status, run.TotalCount, run.SuccessCount, run.FailedCount, run.FinishedAt, run.ErrorMessage, run.ID,
	)
	return err
}

func (s *Store) ListSystemRuns(ctx context.Context, opts ListOptions) ([]SystemRun, int, error) {
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM system_runs`).Scan(&total); err != nil {
		return nil, 0, err
	}
	limit, offset := opts.LimitOffset()
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, trigger_type, status, total_count, success_count, failed_count, started_at, finished_at, error_message
		FROM system_runs ORDER BY started_at DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := []SystemRun{}
	for rows.Next() {
		var item SystemRun
		var finishedAt sql.NullTime
		var errorMessage sql.NullString
		if err := rows.Scan(&item.ID, &item.TriggerType, &item.Status, &item.TotalCount, &item.SuccessCount, &item.FailedCount, &item.StartedAt, &finishedAt, &errorMessage); err != nil {
			return nil, 0, err
		}
		item.FinishedAt = nullTimePtr(finishedAt)
		item.ErrorMessage = errorMessage.String
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func (s *Store) UpdateComponentCheckState(ctx context.Context, c Component, record CheckRecord) error {
	status := record.Status
	if !c.Enabled {
		status = "skipped"
	}
	lastSeen := c.LastSeenVersion
	if record.Status == "success" && record.HasUpdate {
		lastSeen = record.LatestVersion
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE components
		SET latest_version = ?, last_seen_version = ?, last_check_status = ?, last_check_error = ?, last_checked_at = ?, updated_at = ?
		WHERE id = ?`,
		record.LatestVersion, lastSeen, status, record.ErrorMessage, record.CheckedAt, time.Now().UTC(), c.ID,
	)
	return err
}

func (s *Store) DashboardSummary(ctx context.Context) (*DashboardSummary, error) {
	var summary DashboardSummary
	scalars := []struct {
		query string
		dest  *int
	}{
		{`SELECT COUNT(*) FROM components`, &summary.ComponentTotal},
		{`SELECT COUNT(*) FROM components WHERE enabled = 1`, &summary.EnabledComponentTotal},
		{`SELECT COUNT(*) FROM components WHERE latest_version IS NOT NULL AND latest_version <> '' AND latest_version <> current_version`, &summary.ComponentsWithUpdate},
		{`SELECT COUNT(*) FROM components WHERE last_check_status = 'failed'`, &summary.LastCheckFailedTotal},
		{`SELECT COUNT(*) FROM notification_records WHERE status = 'failed'`, &summary.NotificationFailedTotal},
	}
	for _, scalar := range scalars {
		if err := s.db.QueryRowContext(ctx, scalar.query).Scan(scalar.dest); err != nil {
			return nil, err
		}
	}
	var lastFullCheckAt any
	if err := s.db.QueryRowContext(ctx, `SELECT MAX(started_at) FROM system_runs`).Scan(&lastFullCheckAt); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	summary.LastFullCheckAt = timePtr(lastFullCheckAt)
	return &summary, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanComponent(row scanner) (*Component, error) {
	var item Component
	var enabled int
	var latestVersion, lastSeenVersion, lastCheckStatus, lastCheckError, notes, repoURL sql.NullString
	var lastCheckedAt sql.NullTime
	if err := row.Scan(
		&item.ID, &item.Name, &repoURL, &item.CurrentVersion, &latestVersion,
		&lastSeenVersion, &item.CheckStrategy, &enabled,
		&lastCheckStatus, &lastCheckError, &lastCheckedAt, &notes, &item.CreatedAt, &item.UpdatedAt,
	); err != nil {
		return nil, err
	}
	item.RepoURL = repoURL.String
	item.LatestVersion = latestVersion.String
	item.LastSeenVersion = lastSeenVersion.String
	item.Enabled = enabled == 1
	item.LastCheckStatus = lastCheckStatus.String
	item.LastCheckError = lastCheckError.String
	item.LastCheckedAt = nullTimePtr(lastCheckedAt)
	item.Notes = notes.String
	return &item, nil
}

func scanCheckRecord(row scanner) (*CheckRecord, error) {
	var item CheckRecord
	var releasePublishedAt sql.NullTime
	var source, componentName, previousVersion, latestVersion, releaseTitle, releaseURL, releaseNote, releaseNoteSummary, errorMessage sql.NullString
	var hasUpdate int
	if err := row.Scan(
		&item.ID, &item.ComponentID, &componentName, &source, &previousVersion, &latestVersion,
		&releaseTitle, &releaseURL, &releasePublishedAt, &releaseNote, &releaseNoteSummary,
		&hasUpdate, &item.Status, &errorMessage, &item.CheckedAt,
	); err != nil {
		return nil, err
	}
	item.ComponentName = componentName.String
	item.Source = source.String
	item.PreviousVersion = previousVersion.String
	item.LatestVersion = latestVersion.String
	item.ReleaseTitle = releaseTitle.String
	item.ReleaseURL = releaseURL.String
	item.ReleasePublishedAt = nullTimePtr(releasePublishedAt)
	item.ReleaseNote = releaseNote.String
	item.ReleaseNoteSummary = releaseNoteSummary.String
	item.HasUpdate = hasUpdate == 1
	item.ErrorMessage = errorMessage.String
	return &item, nil
}

func scanNotificationRecord(row scanner) (*NotificationRecord, error) {
	var item NotificationRecord
	var componentName, body, errorMessage sql.NullString
	var sentAt sql.NullTime
	if err := row.Scan(
		&item.ID, &item.ComponentID, &componentName, &item.CheckRecordID, &item.Version, &item.RecipientEmail,
		&item.Subject, &body, &item.Status, &errorMessage, &sentAt, &item.CreatedAt,
	); err != nil {
		return nil, err
	}
	item.ComponentName = componentName.String
	item.Body = body.String
	item.ErrorMessage = errorMessage.String
	item.SentAt = nullTimePtr(sentAt)
	return &item, nil
}

func parseIDList(value string) []int64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	ids := make([]int64, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, err := strconv.ParseInt(part, 10, 64)
		if err != nil || id <= 0 {
			continue
		}
		ids = append(ids, id)
	}
	return ids
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func nullTimePtr(value sql.NullTime) *time.Time {
	if value.Valid {
		return &value.Time
	}
	return nil
}

func timePtr(value any) *time.Time {
	switch typed := value.(type) {
	case nil:
		return nil
	case time.Time:
		return &typed
	case string:
		return parseTimePtr(typed)
	case []byte:
		return parseTimePtr(string(typed))
	}
	return nil
}

func parseTimePtr(value string) *time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05.999999999Z07:00",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
	}
	for _, layout := range layouts {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return &parsed
		}
	}
	return nil
}

func repoURL(c *Component) string {
	return c.RepoURL
}
