package storage

const schema = `
PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS components (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  repo_owner TEXT NOT NULL,
  repo_name TEXT NOT NULL,
  repo_url TEXT,
  current_version TEXT NOT NULL,
  latest_version TEXT,
  last_seen_version TEXT,
  owner_name TEXT NOT NULL,
  owner_email TEXT NOT NULL,
  check_strategy TEXT NOT NULL DEFAULT 'release_first',
  enabled INTEGER NOT NULL DEFAULT 1,
  last_check_status TEXT,
  last_check_error TEXT,
  last_checked_at DATETIME,
  notes TEXT,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  UNIQUE(repo_owner, repo_name)
);

CREATE TABLE IF NOT EXISTS subscribers (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  component_id INTEGER NOT NULL,
  name TEXT NOT NULL,
  email TEXT NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  UNIQUE(component_id, email),
  FOREIGN KEY(component_id) REFERENCES components(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS check_records (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  component_id INTEGER NOT NULL,
  source TEXT,
  previous_version TEXT,
  latest_version TEXT,
  release_title TEXT,
  release_url TEXT,
  release_published_at DATETIME,
  release_note TEXT,
  release_note_summary TEXT,
  has_update INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL,
  error_message TEXT,
  checked_at DATETIME NOT NULL,
  FOREIGN KEY(component_id) REFERENCES components(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS notification_records (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  component_id INTEGER NOT NULL,
  check_record_id INTEGER NOT NULL,
  version TEXT NOT NULL,
  recipient_email TEXT NOT NULL,
  subject TEXT NOT NULL,
  body TEXT NOT NULL,
  status TEXT NOT NULL,
  error_message TEXT,
  sent_at DATETIME,
  created_at DATETIME NOT NULL,
  UNIQUE(component_id, version, recipient_email),
  FOREIGN KEY(component_id) REFERENCES components(id) ON DELETE CASCADE,
  FOREIGN KEY(check_record_id) REFERENCES check_records(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS system_runs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  trigger_type TEXT NOT NULL,
  status TEXT NOT NULL,
  total_count INTEGER NOT NULL DEFAULT 0,
  success_count INTEGER NOT NULL DEFAULT 0,
  failed_count INTEGER NOT NULL DEFAULT 0,
  started_at DATETIME NOT NULL,
  finished_at DATETIME,
  error_message TEXT
);

CREATE INDEX IF NOT EXISTS idx_check_records_component_id ON check_records(component_id);
CREATE INDEX IF NOT EXISTS idx_check_records_checked_at ON check_records(checked_at);
CREATE INDEX IF NOT EXISTS idx_notification_records_component_id ON notification_records(component_id);
CREATE INDEX IF NOT EXISTS idx_notification_records_created_at ON notification_records(created_at);
`
