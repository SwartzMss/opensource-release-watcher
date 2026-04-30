export interface ApiResponse<T> {
  code: number;
  message: string;
  data: T;
}

export interface PageData<T> {
  items: T[];
  total: number;
  page: number;
  page_size: number;
}

export interface AuthUser {
  username: string;
}

export interface ComponentItem {
  id: number;
  name: string;
  repo_url: string;
  current_version: string;
  latest_version: string;
  check_strategy: 'release_first' | 'tag_only';
  enabled: boolean;
  last_check_status: 'success' | 'failed' | 'skipped' | '';
  last_check_error: string;
  last_checked_at?: string;
  notes: string;
  created_at: string;
  updated_at: string;
}

export interface Subscriber {
  id: number;
  component_id: number;
  name: string;
  email: string;
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface DashboardSummary {
  component_total: number;
  enabled_component_total: number;
  components_with_update: number;
  last_check_failed_total: number;
  notification_failed_total: number;
  last_full_check_at?: string;
}

export interface CheckRecord {
  id: number;
  component_id: number;
  component_name: string;
  source: 'release' | 'tag' | '';
  previous_version: string;
  latest_version: string;
  release_url: string;
  release_title: string;
  release_published_at?: string;
  release_note_summary: string;
  release_note?: string;
  has_update: boolean;
  status: 'success' | 'failed' | 'skipped';
  error_message: string;
  checked_at: string;
}

export interface LatestVersionInfo {
  source: 'release' | 'tag';
  version: string;
  title: string;
  url: string;
  published_at?: string;
  note: string;
}

export interface NotificationRecord {
  id: number;
  component_id: number;
  component_name: string;
  check_record_id: number;
  version: string;
  recipient_email: string;
  subject: string;
  body?: string;
  status: 'sent' | 'failed';
  error_message: string;
  sent_at?: string;
  created_at: string;
}

export interface MailAuthStatus {
  configured: boolean;
  connected: boolean;
  message?: string;
}

export interface SystemRun {
  id: number;
  trigger_type: 'scheduler' | 'manual';
  status: 'running' | 'success' | 'failed';
  total_count: number;
  success_count: number;
  failed_count: number;
  started_at: string;
  finished_at?: string;
  error_message: string;
}
