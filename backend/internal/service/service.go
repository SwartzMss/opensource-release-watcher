package service

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"opensource-release-watcher/backend/internal/checker"
	"opensource-release-watcher/backend/internal/github"
	"opensource-release-watcher/backend/internal/notifier"
	"opensource-release-watcher/backend/internal/storage"
)

type Service struct {
	store    *storage.Store
	checker  *checker.Checker
	notifier notifier.Notifier
}

func New(store *storage.Store, checker *checker.Checker, notifier notifier.Notifier) *Service {
	return &Service{store: store, checker: checker, notifier: notifier}
}

func (s *Service) CreateComponent(ctx context.Context, c *storage.Component) error {
	return s.store.CreateComponent(ctx, c)
}

func (s *Service) UpdateComponent(ctx context.Context, c *storage.Component) error {
	return s.store.UpdateComponent(ctx, c)
}

func (s *Service) DeleteComponent(ctx context.Context, id int64) error {
	return s.store.DeleteComponent(ctx, id)
}

func (s *Service) GetComponent(ctx context.Context, id int64) (*storage.Component, error) {
	return s.store.GetComponent(ctx, id)
}

func (s *Service) ListComponents(ctx context.Context, opts storage.ListOptions) ([]storage.Component, int, error) {
	return s.store.ListComponents(ctx, opts)
}

func (s *Service) LatestComponentVersion(ctx context.Context, repoURL, checkStrategy string) (*github.ReleaseInfo, error) {
	return s.checker.Latest(ctx, repoURL, checkStrategy)
}

func (s *Service) CreateSubscriber(ctx context.Context, sub *storage.Subscriber) error {
	return s.store.CreateSubscriber(ctx, sub)
}

func (s *Service) UpdateSubscriber(ctx context.Context, sub *storage.Subscriber) error {
	return s.store.UpdateSubscriber(ctx, sub)
}

func (s *Service) DeleteSubscriber(ctx context.Context, id int64) error {
	return s.store.DeleteSubscriber(ctx, id)
}

func (s *Service) ListSubscribers(ctx context.Context, componentID int64) ([]storage.Subscriber, error) {
	return s.store.ListSubscribers(ctx, componentID)
}

func (s *Service) CheckComponent(ctx context.Context, id int64) (*storage.CheckRecord, error) {
	component, err := s.store.GetComponent(ctx, id)
	if err != nil {
		return nil, err
	}
	record := s.checker.Check(ctx, *component)
	if err := s.store.CreateCheckRecord(ctx, &record); err != nil {
		return nil, err
	}
	if err := s.store.UpdateComponentCheckState(ctx, *component, record); err != nil {
		return nil, err
	}
	if record.Status == "success" && record.HasUpdate {
		if err := s.notifyUpdate(ctx, *component, record); err != nil {
			record.ErrorMessage = err.Error()
		}
	}
	return &record, nil
}

func (s *Service) RunChecks(ctx context.Context, triggerType string) (*storage.SystemRun, error) {
	components, err := s.store.ListEnabledComponents(ctx)
	if err != nil {
		return nil, err
	}
	run := &storage.SystemRun{
		TriggerType: triggerType,
		Status:      "running",
		TotalCount:  len(components),
		StartedAt:   time.Now().UTC(),
	}
	if err := s.store.CreateSystemRun(ctx, run); err != nil {
		return nil, err
	}
	for _, component := range components {
		record := s.checker.Check(ctx, component)
		if err := s.store.CreateCheckRecord(ctx, &record); err != nil {
			run.FailedCount++
			continue
		}
		if err := s.store.UpdateComponentCheckState(ctx, component, record); err != nil {
			run.FailedCount++
			continue
		}
		if record.Status == "success" {
			run.SuccessCount++
			if record.HasUpdate {
				_ = s.notifyUpdate(ctx, component, record)
			}
			continue
		}
		run.FailedCount++
	}
	run.Status = "success"
	if run.FailedCount > 0 {
		run.Status = "failed"
	}
	if err := s.store.FinishSystemRun(ctx, run); err != nil {
		return nil, err
	}
	return run, nil
}

func (s *Service) ListCheckRecords(ctx context.Context, opts storage.ListOptions) ([]storage.CheckRecord, int, error) {
	return s.store.ListCheckRecords(ctx, opts)
}

func (s *Service) GetCheckRecord(ctx context.Context, id int64) (*storage.CheckRecord, error) {
	return s.store.GetCheckRecord(ctx, id)
}

func (s *Service) ListNotificationRecords(ctx context.Context, opts storage.ListOptions) ([]storage.NotificationRecord, int, error) {
	return s.store.ListNotificationRecords(ctx, opts)
}

func (s *Service) GetNotificationRecord(ctx context.Context, id int64) (*storage.NotificationRecord, error) {
	return s.store.GetNotificationRecord(ctx, id)
}

func (s *Service) ListSystemRuns(ctx context.Context, opts storage.ListOptions) ([]storage.SystemRun, int, error) {
	return s.store.ListSystemRuns(ctx, opts)
}

func (s *Service) DashboardSummary(ctx context.Context) (*storage.DashboardSummary, error) {
	return s.store.DashboardSummary(ctx)
}

func (s *Service) notifyUpdate(ctx context.Context, component storage.Component, record storage.CheckRecord) error {
	sent, err := s.store.HasSentNotification(ctx, component.ID, record.LatestVersion)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if sent {
		return nil
	}
	subscribers, err := s.store.ListSubscribers(ctx, component.ID)
	if err != nil {
		return err
	}
	recipients := uniqueRecipients(subscribers)
	subject := fmt.Sprintf("[开源组件更新] %s %s -> %s", component.Name, record.PreviousVersion, record.LatestVersion)
	body := buildMailBody(component, record)
	sendErr := s.notifier.Send(notifier.Message{
		To:      recipients,
		Subject: subject,
		Body:    body,
	})
	status := "sent"
	errorMessage := ""
	var sentAt *time.Time
	if sendErr != nil {
		status = "failed"
		errorMessage = sendErr.Error()
	} else {
		now := time.Now().UTC()
		sentAt = &now
	}
	for _, recipient := range recipients {
		_ = s.store.CreateNotificationRecord(ctx, &storage.NotificationRecord{
			ComponentID:    component.ID,
			CheckRecordID:  record.ID,
			Version:        record.LatestVersion,
			RecipientEmail: recipient,
			Subject:        subject,
			Body:           body,
			Status:         status,
			ErrorMessage:   errorMessage,
			SentAt:         sentAt,
		})
	}
	return sendErr
}

func uniqueRecipients(subscribers []storage.Subscriber) []string {
	seen := map[string]bool{}
	add := func(email string, out *[]string) {
		email = strings.TrimSpace(email)
		if email == "" || seen[email] {
			return
		}
		seen[email] = true
		*out = append(*out, email)
	}
	recipients := []string{}
	for _, sub := range subscribers {
		if sub.Enabled {
			add(sub.Email, &recipients)
		}
	}
	return recipients
}

func buildMailBody(component storage.Component, record storage.CheckRecord) string {
	publishedAt := ""
	if record.ReleasePublishedAt != nil {
		publishedAt = record.ReleasePublishedAt.Format(time.RFC3339)
	}
	return fmt.Sprintf(`组件名称：%s
仓库地址：%s
当前使用版本：%s
最新发布版本：%s
发布时间：%s
GitHub 链接：%s

Release Note 摘要：
%s

建议动作：
- 请订阅人评估是否需要升级
- 检查当前项目是否受到影响
- 如涉及安全修复，建议优先处理
`, component.Name, component.RepoURL, component.CurrentVersion, record.LatestVersion, publishedAt, record.ReleaseURL, record.ReleaseNoteSummary)
}
