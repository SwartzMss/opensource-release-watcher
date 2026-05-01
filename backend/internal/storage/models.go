package storage

import "time"

type Component struct {
	ID              int64      `json:"id"`
	Name            string     `json:"name"`
	RepoURL         string     `json:"repo_url"`
	CurrentVersion  string     `json:"current_version"`
	LatestVersion   string     `json:"latest_version"`
	LastSeenVersion string     `json:"last_seen_version"`
	CheckStrategy   string     `json:"check_strategy"`
	Enabled         bool       `json:"enabled"`
	LastCheckStatus string     `json:"last_check_status"`
	LastCheckError  string     `json:"last_check_error"`
	LastCheckedAt   *time.Time `json:"last_checked_at,omitempty"`
	Notes           string     `json:"notes"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type Subscriber struct {
	ID          int64     `json:"id"`
	ComponentID int64     `json:"component_id"`
	Name        string    `json:"name"`
	Email       string    `json:"email"`
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type GlobalSubscriber struct {
	ID            int64     `json:"id"`
	Name          string    `json:"name"`
	Email         string    `json:"email"`
	Enabled       bool      `json:"enabled"`
	AllComponents bool      `json:"all_components"`
	ComponentIDs  []int64   `json:"component_ids,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type CheckRecord struct {
	ID                 int64      `json:"id"`
	ComponentID        int64      `json:"component_id"`
	ComponentName      string     `json:"component_name,omitempty"`
	Source             string     `json:"source"`
	PreviousVersion    string     `json:"previous_version"`
	LatestVersion      string     `json:"latest_version"`
	ReleaseTitle       string     `json:"release_title"`
	ReleaseURL         string     `json:"release_url"`
	ReleasePublishedAt *time.Time `json:"release_published_at,omitempty"`
	ReleaseNote        string     `json:"release_note,omitempty"`
	ReleaseNoteSummary string     `json:"release_note_summary"`
	HasUpdate          bool       `json:"has_update"`
	Status             string     `json:"status"`
	ErrorMessage       string     `json:"error_message"`
	CheckedAt          time.Time  `json:"checked_at"`
}

type NotificationRecord struct {
	ID             int64      `json:"id"`
	ComponentID    int64      `json:"component_id"`
	ComponentName  string     `json:"component_name,omitempty"`
	CheckRecordID  int64      `json:"check_record_id"`
	Version        string     `json:"version"`
	RecipientEmail string     `json:"recipient_email"`
	Subject        string     `json:"subject"`
	Body           string     `json:"body,omitempty"`
	Status         string     `json:"status"`
	ErrorMessage   string     `json:"error_message"`
	SentAt         *time.Time `json:"sent_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

type SystemRun struct {
	ID           int64      `json:"id"`
	TriggerType  string     `json:"trigger_type"`
	Status       string     `json:"status"`
	TotalCount   int        `json:"total_count"`
	SuccessCount int        `json:"success_count"`
	FailedCount  int        `json:"failed_count"`
	StartedAt    time.Time  `json:"started_at"`
	FinishedAt   *time.Time `json:"finished_at,omitempty"`
	ErrorMessage string     `json:"error_message"`
}

type DashboardSummary struct {
	ComponentTotal          int        `json:"component_total"`
	EnabledComponentTotal   int        `json:"enabled_component_total"`
	ComponentsWithUpdate    int        `json:"components_with_update"`
	LastCheckFailedTotal    int        `json:"last_check_failed_total"`
	NotificationFailedTotal int        `json:"notification_failed_total"`
	LastRunDurationSeconds  int        `json:"last_run_duration_seconds"`
	CheckIntervalSeconds    int        `json:"check_interval_seconds"`
	LastFullCheckAt         *time.Time `json:"last_full_check_at,omitempty"`
	NextCheckAt             *time.Time `json:"next_check_at,omitempty"`
}

type ListOptions struct {
	Page           int
	PageSize       int
	Keyword        string
	Enabled        *bool
	Status         string
	ComponentID    int64
	RecipientEmail string
	HasUpdate      *bool
}

func (o ListOptions) LimitOffset() (int, int) {
	page := o.Page
	if page < 1 {
		page = 1
	}
	pageSize := o.PageSize
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	return pageSize, (page - 1) * pageSize
}
