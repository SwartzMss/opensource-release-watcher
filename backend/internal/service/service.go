package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"opensource-release-watcher/backend/internal/checker"
	"opensource-release-watcher/backend/internal/github"
	"opensource-release-watcher/backend/internal/notifier"
	"opensource-release-watcher/backend/internal/storage"
	"opensource-release-watcher/backend/internal/version"
)

type Service struct {
	store         *storage.Store
	checker       *checker.Checker
	notifier      notifier.Notifier
	mailAuth      notifier.StatusProvider
	checkInterval time.Duration
}

func New(store *storage.Store, checker *checker.Checker, mailer notifier.Notifier, checkInterval time.Duration) *Service {
	mailAuth, _ := mailer.(notifier.StatusProvider)
	return &Service{store: store, checker: checker, notifier: mailer, mailAuth: mailAuth, checkInterval: checkInterval}
}

func (s *Service) CreateComponent(ctx context.Context, c *storage.Component) error {
	log.Printf("create component name=%s repo=%s enabled=%t", c.Name, c.RepoURL, c.Enabled)
	return s.store.CreateComponent(ctx, c)
}

func (s *Service) UpdateComponent(ctx context.Context, c *storage.Component) error {
	log.Printf("update component id=%d name=%s repo=%s enabled=%t", c.ID, c.Name, c.RepoURL, c.Enabled)
	return s.store.UpdateComponent(ctx, c)
}

func (s *Service) DeleteComponent(ctx context.Context, id int64) error {
	log.Printf("delete component id=%d", id)
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
	log.Printf("create subscriber component_id=%d email=%s enabled=%t", sub.ComponentID, sub.Email, sub.Enabled)
	return s.store.CreateSubscriber(ctx, sub)
}

func (s *Service) CreateGlobalSubscriber(ctx context.Context, sub *storage.GlobalSubscriber) error {
	log.Printf("create global subscriber email=%s enabled=%t all_components=%t", sub.Email, sub.Enabled, sub.AllComponents)
	return s.store.CreateGlobalSubscriber(ctx, sub)
}

func (s *Service) UpdateSubscriber(ctx context.Context, sub *storage.Subscriber) error {
	log.Printf("update subscriber id=%d component_id=%d email=%s enabled=%t", sub.ID, sub.ComponentID, sub.Email, sub.Enabled)
	return s.store.UpdateSubscriber(ctx, sub)
}

func (s *Service) UpdateGlobalSubscriber(ctx context.Context, sub *storage.GlobalSubscriber) error {
	log.Printf("update global subscriber id=%d email=%s enabled=%t all_components=%t", sub.ID, sub.Email, sub.Enabled, sub.AllComponents)
	return s.store.UpdateGlobalSubscriber(ctx, sub)
}

func (s *Service) DeleteSubscriber(ctx context.Context, id int64) error {
	log.Printf("delete subscriber id=%d", id)
	return s.store.DeleteSubscriber(ctx, id)
}

func (s *Service) DeleteGlobalSubscriber(ctx context.Context, id int64) error {
	log.Printf("delete global subscriber id=%d", id)
	return s.store.DeleteGlobalSubscriber(ctx, id)
}

func (s *Service) ListSubscribers(ctx context.Context, componentID int64) ([]storage.Subscriber, error) {
	return s.store.ListSubscribers(ctx, componentID)
}

func (s *Service) ListGlobalSubscribers(ctx context.Context) ([]storage.GlobalSubscriber, error) {
	return s.store.ListGlobalSubscribers(ctx)
}

func (s *Service) GetGlobalSubscriber(ctx context.Context, id int64) (*storage.GlobalSubscriber, error) {
	return s.store.GetGlobalSubscriber(ctx, id)
}

func (s *Service) SetGlobalSubscriberComponents(ctx context.Context, id int64, allComponents bool, componentIDs []int64) error {
	log.Printf("set global subscriber components id=%d all_components=%t component_ids=%v", id, allComponents, componentIDs)
	return s.store.SetGlobalSubscriberComponents(ctx, id, allComponents, componentIDs)
}

func (s *Service) CheckComponent(ctx context.Context, id int64) (*storage.CheckRecord, error) {
	component, err := s.store.GetComponent(ctx, id)
	if err != nil {
		return nil, err
	}
	log.Printf("check component started id=%d name=%s repo=%s", component.ID, component.Name, component.RepoURL)
	record := s.checker.Check(ctx, *component)
	if err := s.store.CreateCheckRecord(ctx, &record); err != nil {
		return nil, err
	}
	if err := s.store.UpdateComponentCheckState(ctx, *component, record); err != nil {
		return nil, err
	}
	if record.Status == "success" && record.LatestVersion != "" {
		if err := s.notifyUpdate(ctx, *component, record); err != nil {
			record.ErrorMessage = err.Error()
		}
	}
	log.Printf("check component finished id=%d status=%s has_update=%t latest=%s previous=%s", component.ID, record.Status, record.HasUpdate, record.LatestVersion, record.PreviousVersion)
	return &record, nil
}

func (s *Service) RunChecks(ctx context.Context, triggerType string) (*storage.SystemRun, error) {
	components, err := s.store.ListEnabledComponents(ctx)
	if err != nil {
		return nil, err
	}
	startedAt := time.Now().UTC()
	log.Printf("run checks started trigger=%s components=%d", triggerType, len(components))
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
			if record.LatestVersion != "" {
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
	log.Printf("run checks finished trigger=%s total=%d success=%d failed=%d duration=%s", triggerType, run.TotalCount, run.SuccessCount, run.FailedCount, time.Since(startedAt).Round(time.Millisecond))
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

func (s *Service) MailAuthStatus(ctx context.Context) (notifier.AuthStatus, error) {
	if s.mailAuth == nil {
		return notifier.AuthStatus{Message: "mail authentication is not available"}, nil
	}
	return s.mailAuth.Status(ctx)
}

func (s *Service) SendTestNotification(ctx context.Context, recipient string) error {
	recipient = strings.TrimSpace(recipient)
	if recipient == "" {
		return fmt.Errorf("recipient is required")
	}
	log.Printf("send test notification recipient=%s", recipient)
	now := time.Now().Format(time.RFC3339)
	return s.notifier.Send(notifier.Message{
		To:      []string{recipient},
		Subject: "[开源组件更新] 测试邮件",
		Body: fmt.Sprintf(`这是一封来自 opensource-release-watcher 的测试邮件。

如果你收到这封邮件，说明 Outlook / Microsoft Graph 发信配置可以正常工作。

发送时间：%s
`, now),
	})
}

func (s *Service) ListSystemRuns(ctx context.Context, opts storage.ListOptions) ([]storage.SystemRun, int, error) {
	return s.store.ListSystemRuns(ctx, opts)
}

func (s *Service) DashboardSummary(ctx context.Context) (*storage.DashboardSummary, error) {
	summary, err := s.store.DashboardSummary(ctx)
	if err != nil {
		return nil, err
	}
	if s.checkInterval > 0 {
		summary.CheckIntervalSeconds = int(s.checkInterval.Seconds())
		if summary.LastFullCheckAt != nil {
			nextCheckAt := summary.LastFullCheckAt.Add(s.checkInterval)
			summary.NextCheckAt = &nextCheckAt
		}
	}
	return summary, nil
}

func (s *Service) notifyUpdate(ctx context.Context, component storage.Component, record storage.CheckRecord) error {
	targets, err := s.store.ListSubscriberNotificationTargets(ctx, component.ID)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		return nil
	}
	var errs []error
	sentCount := 0
	skippedCount := 0
	for _, target := range targets {
		baseline := target.LastNotifiedVersion
		if baseline == "" {
			baseline = component.CurrentVersion
		}
		subject := fmt.Sprintf("[开源组件更新] %s %s -> %s", component.Name, baseline, record.LatestVersion)
		body := buildMailBody(component, record)
		if !version.IsNewer(record.LatestVersion, baseline) {
			skippedCount++
			continue
		}
		sent, err := s.store.HasSentNotification(ctx, component.ID, record.LatestVersion, target.Email)
		if err != nil {
			errs = append(errs, err)
			log.Printf("notify update failed component_id=%d name=%s version=%s recipient=%s err=%v", component.ID, component.Name, record.LatestVersion, target.Email, err)
			continue
		}
		if sent {
			if err := s.store.UpsertSubscriberComponentProgress(ctx, target.SubscriberID, component.ID, record.LatestVersion); err != nil {
				errs = append(errs, err)
				log.Printf("notify update progress sync failed component_id=%d version=%s recipient=%s err=%v", component.ID, record.LatestVersion, target.Email, err)
				continue
			}
			skippedCount++
			continue
		}
		log.Printf("notify update started component_id=%d name=%s version=%s recipient=%s", component.ID, component.Name, record.LatestVersion, target.Email)
		sendErr := s.notifier.Send(notifier.Message{
			To:      []string{target.Email},
			Subject: subject,
			Body:    body,
		})
		status := "sent"
		errorMessage := ""
		var sentAt *time.Time
		if sendErr != nil {
			status = "failed"
			errorMessage = sendErr.Error()
			errs = append(errs, sendErr)
		} else {
			now := time.Now().UTC()
			sentAt = &now
			sentCount++
		}
		if err := s.store.CreateNotificationRecord(ctx, &storage.NotificationRecord{
			ComponentID:    component.ID,
			CheckRecordID:  record.ID,
			Version:        record.LatestVersion,
			RecipientEmail: target.Email,
			Subject:        subject,
			Body:           body,
			Status:         status,
			ErrorMessage:   errorMessage,
			SentAt:         sentAt,
		}); err != nil {
			errs = append(errs, err)
			log.Printf("notify update record write failed component_id=%d version=%s recipient=%s err=%v", component.ID, record.LatestVersion, target.Email, err)
		}
		if sendErr == nil {
			if err := s.store.UpsertSubscriberComponentProgress(ctx, target.SubscriberID, component.ID, record.LatestVersion); err != nil {
				errs = append(errs, err)
				log.Printf("notify update progress update failed component_id=%d version=%s recipient=%s err=%v", component.ID, record.LatestVersion, target.Email, err)
			}
		}
		if sendErr != nil {
			log.Printf("notify update failed component_id=%d version=%s recipient=%s err=%v", component.ID, record.LatestVersion, target.Email, sendErr)
		} else {
			log.Printf("notify update finished component_id=%d version=%s recipient=%s", component.ID, record.LatestVersion, target.Email)
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	if sentCount == 0 && skippedCount > 0 {
		return nil
	}
	return nil
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
