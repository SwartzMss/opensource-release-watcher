package storage

const schema = `
PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS components (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  repo_url TEXT NOT NULL,
  current_version TEXT NOT NULL,
  latest_version TEXT,
  last_seen_version TEXT,
  check_strategy TEXT NOT NULL DEFAULT 'release_first',
  enabled INTEGER NOT NULL DEFAULT 1,
  last_check_status TEXT,
  last_check_error TEXT,
  last_checked_at DATETIME,
  notes TEXT,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  UNIQUE(repo_url)
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

CREATE TABLE IF NOT EXISTS global_subscribers (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  email TEXT NOT NULL UNIQUE,
  enabled INTEGER NOT NULL DEFAULT 1,
  all_components INTEGER NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS global_subscriber_components (
  subscriber_id INTEGER NOT NULL,
  component_id INTEGER NOT NULL,
  last_notified_version TEXT NOT NULL DEFAULT '',
  created_at DATETIME NOT NULL,
  PRIMARY KEY (subscriber_id, component_id),
  FOREIGN KEY(subscriber_id) REFERENCES global_subscribers(id) ON DELETE CASCADE,
  FOREIGN KEY(component_id) REFERENCES components(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS schema_migrations (
  name TEXT PRIMARY KEY,
  applied_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS component_security_profiles (
  component_id INTEGER PRIMARY KEY,
  security_mode TEXT,
  security_lookup_mode TEXT NOT NULL DEFAULT 'commit_first',
  security_commit_sha TEXT,
  security_package_name TEXT,
  security_ecosystem TEXT,
  security_aliases TEXT,
  security_notes TEXT,
  security_tag_pattern TEXT,
  last_security_status TEXT,
  last_security_reason TEXT,
  last_security_raw_payload TEXT,
  last_security_checked_at DATETIME,
  last_security_summary TEXT,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  FOREIGN KEY(component_id) REFERENCES components(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS component_security_records (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  component_id INTEGER NOT NULL,
  version TEXT NOT NULL,
  commit_sha TEXT,
  risk_type TEXT NOT NULL,
  risk_status TEXT NOT NULL,
  source TEXT NOT NULL,
  identifier TEXT,
  package_name TEXT,
  ecosystem TEXT,
  affected_range TEXT,
  fixed_version TEXT,
  severity TEXT,
  confidence REAL,
  summary TEXT,
  status_reason TEXT,
  raw_payload TEXT,
  evidence_url TEXT,
  created_at DATETIME NOT NULL,
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
CREATE INDEX IF NOT EXISTS idx_component_security_records_component_id ON component_security_records(component_id);
CREATE INDEX IF NOT EXISTS idx_component_security_records_created_at ON component_security_records(created_at);
CREATE INDEX IF NOT EXISTS idx_notification_records_component_id ON notification_records(component_id);
CREATE INDEX IF NOT EXISTS idx_notification_records_created_at ON notification_records(created_at);
`
