package service

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"opensource-release-watcher/backend/internal/checker"
	"opensource-release-watcher/backend/internal/github"
	"opensource-release-watcher/backend/internal/notifier"
	"opensource-release-watcher/backend/internal/security"
	"opensource-release-watcher/backend/internal/storage"
	"opensource-release-watcher/backend/internal/version"
)

type Service struct {
	store           *storage.Store
	checker         *checker.Checker
	security        *security.Checker
	notifier        notifier.Notifier
	mailAuth        notifier.StatusProvider
	checkInterval   time.Duration
	githubToken     string
	httpProxy       string
	httpsProxy      string
	noProxy         string
	securityQueue   chan securityJob
	securityMu      sync.Mutex
	securityQueued  map[int64]struct{}
	securityRunning map[int64]struct{}
	securityPending map[int64]securityJob
	runtimeMu       sync.RWMutex
	runtimeStatus   RuntimeStatus
}

type securityJob struct {
	component    storage.Component
	runID        int64
	checkRecord  storage.CheckRecord
	forceResolve bool
	trigger      string
}

func New(store *storage.Store, checker *checker.Checker, securityChecker *security.Checker, mailer notifier.Notifier, checkInterval time.Duration, githubToken, httpProxy, httpsProxy, noProxy string) *Service {
	mailAuth, _ := mailer.(notifier.StatusProvider)
	svc := &Service{
		store:           store,
		checker:         checker,
		security:        securityChecker,
		notifier:        mailer,
		mailAuth:        mailAuth,
		checkInterval:   checkInterval,
		githubToken:     strings.TrimSpace(githubToken),
		httpProxy:       strings.TrimSpace(httpProxy),
		httpsProxy:      strings.TrimSpace(httpsProxy),
		noProxy:         strings.TrimSpace(noProxy),
		securityQueue:   make(chan securityJob, 128),
		securityQueued:  make(map[int64]struct{}),
		securityRunning: make(map[int64]struct{}),
		securityPending: make(map[int64]securityJob),
		runtimeStatus: RuntimeStatus{
			GitHubTokenStatus: "待检测",
			ProxyStatus:       "待检测",
		},
	}
	svc.startSecurityWorkers(2)
	svc.startRuntimeStatusProbe()
	return svc
}

func (s *Service) CreateComponent(ctx context.Context, c *storage.Component) error {
	log.Printf("create component name=%s repo=%s enabled=%t", c.Name, c.RepoURL, c.Enabled)
	if err := s.validateComponentVersion(ctx, *c); err != nil {
		return err
	}
	if err := s.store.CreateComponent(ctx, c); err != nil {
		return err
	}
	if err := s.store.ClearComponentSecurityCommitCache(ctx, c.ID); err != nil {
		log.Printf("clear security commit cache failed component_id=%d trigger=create_component err=%v", c.ID, err)
	}
	if _, err := s.runComponentCheck(ctx, *c, "create_component", true); err != nil {
		return err
	}
	return nil
}

func (s *Service) UpdateComponent(ctx context.Context, c *storage.Component) error {
	log.Printf("update component id=%d name=%s repo=%s current_version=%s enabled=%t", c.ID, c.Name, c.RepoURL, c.CurrentVersion, c.Enabled)
	if err := s.validateComponentVersion(ctx, *c); err != nil {
		return err
	}
	if err := s.store.UpdateComponent(ctx, c); err != nil {
		return err
	}
	if err := s.store.ClearComponentSecurityCommitCache(ctx, c.ID); err != nil {
		log.Printf("clear security commit cache failed component_id=%d trigger=update_component err=%v", c.ID, err)
	}
	if _, err := s.runComponentCheck(ctx, *c, "update_component", true); err != nil {
		return err
	}
	return nil
}

func (s *Service) DeleteComponent(ctx context.Context, id int64) error {
	log.Printf("delete component id=%d", id)
	return s.store.DeleteComponent(ctx, id)
}

func (s *Service) GetComponent(ctx context.Context, id int64) (*storage.Component, error) {
	return s.store.GetComponent(ctx, id)
}

func (s *Service) ListComponentSecurityRecords(ctx context.Context, componentID int64) ([]storage.ComponentSecurityRecord, error) {
	return s.store.ListComponentSecurityRecords(ctx, componentID)
}

func (s *Service) ListSecurityRecords(ctx context.Context, opts storage.ListOptions) ([]storage.ComponentSecurityRecord, int, error) {
	return s.store.ListSecurityRecords(ctx, opts)
}

func (s *Service) ListComponents(ctx context.Context, opts storage.ListOptions) ([]storage.Component, int, error) {
	return s.store.ListComponents(ctx, opts)
}

func (s *Service) LatestComponentVersion(ctx context.Context, repoURL, checkStrategy string) (*github.ReleaseInfo, error) {
	return s.checker.Latest(ctx, repoURL, checkStrategy)
}

func (s *Service) validateComponentVersion(ctx context.Context, component storage.Component) error {
	log.Printf("validate component version repo=%s strategy=%s version=%s", component.RepoURL, component.CheckStrategy, component.CurrentVersion)
	ok, err := s.checker.HasVersion(ctx, component.RepoURL, component.CheckStrategy, component.CurrentVersion)
	if err != nil {
		log.Printf("validate component version failed repo=%s strategy=%s version=%s err=%v", component.RepoURL, component.CheckStrategy, component.CurrentVersion, err)
		return err
	}
	if !ok {
		err := fmt.Errorf("当前版本必须存在于 GitHub Release 或 Tag 历史中")
		log.Printf("validate component version rejected repo=%s strategy=%s version=%s err=%v", component.RepoURL, component.CheckStrategy, component.CurrentVersion, err)
		return err
	}
	log.Printf("validate component version passed repo=%s strategy=%s version=%s", component.RepoURL, component.CheckStrategy, component.CurrentVersion)
	return nil
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
	record, err := s.runComponentCheck(ctx, *component, "manual_check", false)
	if err != nil {
		return nil, err
	}
	log.Printf("check component finished id=%d status=%s has_update=%t latest=%s previous=%s", component.ID, record.Status, record.HasUpdate, record.LatestVersion, record.PreviousVersion)
	return record, nil
}

func (s *Service) runComponentCheck(ctx context.Context, component storage.Component, trigger string, forceSecurityResolve bool) (*storage.CheckRecord, error) {
	run := &storage.ComponentCheckRun{
		ComponentID:    component.ID,
		TriggerType:    trigger,
		Status:         "running",
		VersionStatus:  "running",
		SecurityStatus: "queued",
	}
	if err := s.store.CreateComponentCheckRun(ctx, run); err != nil {
		return nil, err
	}
	log.Printf("component check run created run_id=%d component_id=%d trigger=%s", run.ID, component.ID, run.TriggerType)
	record := s.checker.Check(ctx, component)
	record.RunID = run.ID
	if err := s.store.CreateCheckRecord(ctx, &record); err != nil {
		s.finishComponentCheckRunFailed(context.Background(), run.ID, "failed", err)
		return nil, err
	}
	if err := s.store.UpdateComponentCheckState(ctx, component, record); err != nil {
		s.finishComponentCheckRunFailed(context.Background(), run.ID, record.Status, err)
		return nil, err
	}
	if err := s.store.UpdateComponentCheckRunVersion(ctx, run.ID, record); err != nil {
		s.finishComponentCheckRunFailed(context.Background(), run.ID, record.Status, err)
		return nil, err
	}
	log.Printf("component check run version finished run_id=%d component_id=%d status=%s has_update=%t latest=%s previous=%s", run.ID, component.ID, record.Status, record.HasUpdate, record.LatestVersion, record.PreviousVersion)
	s.enqueueSecuritySync(component, run.ID, record, forceSecurityResolve, trigger)
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
		checkRun := &storage.ComponentCheckRun{
			ComponentID:    component.ID,
			TriggerType:    triggerType,
			Status:         "running",
			VersionStatus:  "running",
			SecurityStatus: "queued",
		}
		if err := s.store.CreateComponentCheckRun(ctx, checkRun); err != nil {
			run.FailedCount++
			continue
		}
		log.Printf("component check run created run_id=%d component_id=%d trigger=%s", checkRun.ID, component.ID, checkRun.TriggerType)
		record := s.checker.Check(ctx, component)
		record.RunID = checkRun.ID
		if err := s.store.CreateCheckRecord(ctx, &record); err != nil {
			s.finishComponentCheckRunFailed(context.Background(), checkRun.ID, "failed", err)
			run.FailedCount++
			continue
		}
		if err := s.store.UpdateComponentCheckState(ctx, component, record); err != nil {
			s.finishComponentCheckRunFailed(context.Background(), checkRun.ID, record.Status, err)
			run.FailedCount++
			continue
		}
		if err := s.store.UpdateComponentCheckRunVersion(ctx, checkRun.ID, record); err != nil {
			s.finishComponentCheckRunFailed(context.Background(), checkRun.ID, record.Status, err)
			run.FailedCount++
			continue
		}
		log.Printf("component check run version finished run_id=%d component_id=%d status=%s has_update=%t latest=%s previous=%s", checkRun.ID, component.ID, record.Status, record.HasUpdate, record.LatestVersion, record.PreviousVersion)
		if record.Status == "success" {
			run.SuccessCount++
			s.enqueueSecuritySync(component, checkRun.ID, record, false, "scheduler_run")
			continue
		}
		s.enqueueSecuritySync(component, checkRun.ID, record, false, "scheduler_run")
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

func (s *Service) finishComponentCheckRunFailed(ctx context.Context, runID int64, versionStatus string, err error) {
	if runID <= 0 || err == nil {
		return
	}
	status := "failed"
	if versionStatus == "success" {
		status = "partial_failed"
	}
	if finishErr := s.store.FinishComponentCheckRun(ctx, &storage.ComponentCheckRun{
		ID:             runID,
		Status:         status,
		VersionStatus:  versionStatus,
		SecurityStatus: "skipped",
		ErrorMessage:   err.Error(),
	}); finishErr != nil {
		log.Printf("component check run finish failed run_id=%d err=%v", runID, finishErr)
	}
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

type RuntimeStatus struct {
	GitHubTokenStatus  string `json:"github_token_status"`
	GitHubTokenMessage string `json:"github_token_message,omitempty"`
	ProxyStatus        string `json:"proxy_status"`
	ProxyMessage       string `json:"proxy_message,omitempty"`
	CheckedAt          string `json:"checked_at,omitempty"`
}

func (s *Service) RuntimeStatus(ctx context.Context) (RuntimeStatus, error) {
	s.runtimeMu.RLock()
	status := s.runtimeStatus
	s.runtimeMu.RUnlock()
	return status, nil
}

func (s *Service) startRuntimeStatusProbe() {
	go func() {
		s.refreshRuntimeStatus(context.Background())
		ticker := time.NewTicker(2 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			s.refreshRuntimeStatus(context.Background())
		}
	}()
}

func (s *Service) refreshRuntimeStatus(parent context.Context) {
	ctx, cancel := context.WithTimeout(parent, 45*time.Second)
	defer cancel()
	log.Printf("runtime status probe started")
	status := RuntimeStatus{
		GitHubTokenStatus: "未配置",
		ProxyStatus:       "未配置",
	}
	if s.hasProxyConfig() {
		if err := probeGitHubEndpoint(ctx, "", false); err != nil {
			status.ProxyStatus = "异常"
			status.ProxyMessage = err.Error()
		} else {
			status.ProxyStatus = "正常"
		}
	} else {
		status.ProxyMessage = "未配置 HTTP_PROXY / HTTPS_PROXY"
	}
	if s.githubToken != "" {
		if err := probeGitHubEndpoint(ctx, s.githubToken, true); err != nil {
			status.GitHubTokenStatus = "异常"
			status.GitHubTokenMessage = err.Error()
		} else {
			status.GitHubTokenStatus = "正常"
		}
	} else {
		status.GitHubTokenMessage = "GITHUB_TOKEN 未配置，GitHub API 仍会使用匿名额度"
	}
	status.CheckedAt = time.Now().UTC().Format(time.RFC3339)
	s.runtimeMu.Lock()
	s.runtimeStatus = status
	s.runtimeMu.Unlock()
	log.Printf("runtime status probe finished proxy=%s github_token=%s checked_at=%s", status.ProxyStatus, status.GitHubTokenStatus, status.CheckedAt)
}

func (s *Service) hasProxyConfig() bool {
	return s.httpProxy != "" || s.httpsProxy != ""
}

func probeGitHubEndpoint(ctx context.Context, token string, authenticated bool) error {
	client := &http.Client{Timeout: 10 * time.Second}
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/rate_limit", nil)
		if err != nil {
			return err
		}
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("User-Agent", "opensource-release-watcher")
		if authenticated {
			req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(token))
		}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
		} else {
			func() {
				defer resp.Body.Close()
				if authenticated && (resp.StatusCode < 200 || resp.StatusCode >= 300) {
					lastErr = fmt.Errorf("github token probe returned %s", resp.Status)
					return
				}
				lastErr = nil
			}()
		}
		if lastErr == nil {
			return nil
		}
		if attempt < 3 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(attempt) * time.Second):
			}
		}
	}
	return lastErr
}

func (s *Service) SendTestNotification(ctx context.Context, recipient string) error {
	recipient = strings.TrimSpace(recipient)
	if recipient == "" {
		return fmt.Errorf("recipient is required")
	}
	log.Printf("send test notification recipient=%s", recipient)
	now := time.Now().Format(time.RFC3339)
	if err := s.notifier.Send(notifier.Message{
		To:      []string{recipient},
		Subject: "[开源组件更新] 测试邮件",
		Body: fmt.Sprintf(`这是一封来自 opensource-release-watcher 的测试邮件。

如果你收到这封邮件，说明当前邮件发信配置可以正常工作。

发送时间：%s
`, now),
	}); err != nil {
		log.Printf("send test notification failed recipient=%s err=%v", recipient, err)
		return err
	}
	log.Printf("send test notification finished recipient=%s", recipient)
	return nil
}

func (s *Service) enqueueSecuritySync(component storage.Component, runID int64, checkRecord storage.CheckRecord, forceResolve bool, trigger string) {
	if s.security == nil {
		if runID > 0 {
			if err := s.notifyCheckRunSummary(context.Background(), component, checkRecord, storage.ComponentSecurityProfile{}, nil, runID, "skipped", "security checker unavailable"); err != nil {
				log.Printf("check run summary notification failed component_id=%d run_id=%d err=%v", component.ID, runID, err)
			}
		}
		return
	}
	job := securityJob{component: component, runID: runID, checkRecord: checkRecord, forceResolve: forceResolve, trigger: trigger}
	s.securityMu.Lock()
	if _, running := s.securityRunning[component.ID]; running {
		replaced, hadPending := s.securityPending[component.ID]
		s.securityPending[component.ID] = job
		s.securityMu.Unlock()
		log.Printf("security sync deferred component_id=%d trigger=%s already running", component.ID, trigger)
		if hadPending && replaced.runID > 0 && replaced.runID != runID {
			s.finishComponentCheckRunSuperseded(context.Background(), replaced.runID, replaced.checkRecord.Status, "newer security sync superseded pending job")
		}
		return
	}
	if _, queued := s.securityQueued[component.ID]; queued {
		replaced, hadPending := s.securityPending[component.ID]
		s.securityPending[component.ID] = job
		s.securityMu.Unlock()
		log.Printf("security sync deferred component_id=%d trigger=%s already queued", component.ID, trigger)
		if hadPending && replaced.runID > 0 && replaced.runID != runID {
			s.finishComponentCheckRunSuperseded(context.Background(), replaced.runID, replaced.checkRecord.Status, "newer security sync superseded pending job")
		}
		return
	}
	s.securityQueued[component.ID] = struct{}{}
	s.securityMu.Unlock()

	select {
	case s.securityQueue <- job:
		log.Printf("security sync queued component_id=%d trigger=%s force_resolve=%t", component.ID, trigger, forceResolve)
	default:
		log.Printf("security queue full component_id=%d trigger=%s, running inline", component.ID, trigger)
		go s.executeSecurityJob(job)
	}
}

func (s *Service) startSecurityWorkers(workerCount int) {
	if s.security == nil || workerCount <= 0 {
		return
	}
	for i := 0; i < workerCount; i++ {
		workerID := i + 1
		go func() {
			log.Printf("security worker started id=%d", workerID)
			for job := range s.securityQueue {
				s.executeSecurityJob(job)
			}
		}()
	}
}

func (s *Service) executeSecurityJob(job securityJob) {
	s.securityMu.Lock()
	delete(s.securityQueued, job.component.ID)
	s.securityRunning[job.component.ID] = struct{}{}
	s.securityMu.Unlock()
	defer func() {
		s.securityMu.Lock()
		delete(s.securityRunning, job.component.ID)
		next, hasPending := s.securityPending[job.component.ID]
		if hasPending {
			delete(s.securityPending, job.component.ID)
		}
		s.securityMu.Unlock()
		if hasPending {
			log.Printf("security sync starting deferred job component_id=%d trigger=%s run_id=%d", next.component.ID, next.trigger, next.runID)
			go s.executeSecurityJob(next)
		}
	}()
	s.runSecuritySync(job)
}

func (s *Service) finishComponentCheckRunSuperseded(ctx context.Context, runID int64, versionStatus, reason string) {
	if runID <= 0 {
		return
	}
	if versionStatus == "" {
		versionStatus = "success"
	}
	if err := s.store.FinishComponentCheckRun(ctx, &storage.ComponentCheckRun{
		ID:             runID,
		Status:         "partial_failed",
		VersionStatus:  versionStatus,
		SecurityStatus: "skipped",
		ErrorMessage:   reason,
	}); err != nil {
		log.Printf("component check run supersede finish failed run_id=%d err=%v", runID, err)
	}
}

func (s *Service) runSecuritySync(job securityJob) {
	if s.security == nil {
		return
	}
	startedAt := time.Now().UTC()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	profile, err := s.store.GetComponentSecurityProfile(ctx, job.component.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			profile = &storage.ComponentSecurityProfile{
				ComponentID:        job.component.ID,
				SecurityLookupMode: "commit_first",
			}
			log.Printf("security profile missing component_id=%d trigger=%s, using defaults", job.component.ID, job.trigger)
		} else {
			log.Printf("security profile load failed component_id=%d trigger=%s err=%v", job.component.ID, job.trigger, err)
			return
		}
	}
	if job.forceResolve {
		profile.SecurityCommitSHA = ""
	}
	if profile.SecurityLookupMode == "" {
		profile.SecurityLookupMode = "commit_first"
	}
	report, err := s.security.Check(ctx, job.component, *profile, job.forceResolve)
	if err != nil {
		log.Printf("security check failed component_id=%d trigger=%s err=%v", job.component.ID, job.trigger, err)
		if job.runID > 0 {
			if notifyErr := s.notifyCheckRunSummary(context.Background(), job.component, job.checkRecord, storage.ComponentSecurityProfile{}, nil, job.runID, "failed", err.Error()); notifyErr != nil {
				log.Printf("check run summary notification failed component_id=%d run_id=%d err=%v", job.component.ID, job.runID, notifyErr)
			}
		}
		return
	}
	for i := range report.Records {
		report.Records[i].RunID = job.runID
	}
	log.Printf("component check run security finished run_id=%d component_id=%d status=%s records=%d suggested_version=%s", job.runID, job.component.ID, report.Profile.LastSecurityStatus, len(report.Records), report.Profile.SecuritySuggestedVersion)
	current, err := s.store.GetComponent(ctx, job.component.ID)
	if err != nil {
		log.Printf("security current component load failed component_id=%d trigger=%s err=%v", job.component.ID, job.trigger, err)
		if job.runID > 0 {
			s.finishComponentCheckRunSuperseded(context.Background(), job.runID, job.checkRecord.Status, err.Error())
		}
		return
	}
	if current.CurrentVersion != job.component.CurrentVersion {
		reason := fmt.Sprintf("security result stale: checked version %s, current version %s", job.component.CurrentVersion, current.CurrentVersion)
		log.Printf("security stale result skipped component_id=%d trigger=%s %s", job.component.ID, job.trigger, reason)
		if job.runID > 0 {
			s.finishComponentCheckRunSuperseded(context.Background(), job.runID, job.checkRecord.Status, reason)
		}
		return
	}
	if err := s.store.SaveComponentSecurityState(ctx, report.Profile, report.Records); err != nil {
		log.Printf("security state save failed component_id=%d trigger=%s err=%v", job.component.ID, job.trigger, err)
		if job.runID > 0 {
			if notifyErr := s.notifyCheckRunSummary(context.Background(), job.component, job.checkRecord, storage.ComponentSecurityProfile{}, nil, job.runID, "failed", err.Error()); notifyErr != nil {
				log.Printf("check run summary notification failed component_id=%d run_id=%d err=%v", job.component.ID, job.runID, notifyErr)
			}
		}
		return
	}
	if job.runID > 0 {
		if err := s.notifyCheckRunSummary(context.Background(), job.component, job.checkRecord, report.Profile, report.Records, job.runID, report.Profile.LastSecurityStatus, ""); err != nil {
			log.Printf("check run summary notification failed component_id=%d run_id=%d err=%v", job.component.ID, job.runID, err)
		}
	}
	if report.Profile.SecuritySuggestedVersion != "" {
		log.Printf("security suggested version component_id=%d trigger=%s version=%s", job.component.ID, job.trigger, report.Profile.SecuritySuggestedVersion)
	}
	log.Printf(
		"security check persisted component_id=%d trigger=%s status=%s reason=%s commit=%s records=%d duration=%s",
		job.component.ID,
		job.trigger,
		report.Profile.LastSecurityStatus,
		report.Profile.LastSecurityReason,
		report.Profile.SecurityCommitSHA,
		len(report.Records),
		time.Since(startedAt).Round(time.Millisecond),
	)
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

func (s *Service) notifyCheckRunSummary(ctx context.Context, component storage.Component, record storage.CheckRecord, profile storage.ComponentSecurityProfile, records []storage.ComponentSecurityRecord, runID int64, securityStatus, runError string) error {
	affectedCount := countAffectedSecurityRecords(records)
	fingerprint := checkRunFingerprint(record, profile, records, securityStatus)
	log.Printf("notify check run summary started run_id=%d component_id=%d version_status=%s security_status=%s affected=%d fingerprint=%s", runID, component.ID, record.Status, securityStatus, affectedCount, fingerprint)
	run := &storage.ComponentCheckRun{
		ID:                         runID,
		Status:                     "success",
		SecurityStatus:             securityStatus,
		SecuritySuggestedVersion:   profile.SecuritySuggestedVersion,
		AffectedVulnerabilityCount: affectedCount,
		NotificationFingerprint:    fingerprint,
		ErrorMessage:               runError,
	}
	if record.Status != "" && record.Status != "success" {
		run.Status = "failed"
	} else if securityStatus == "failed" || securityStatus == "check_failed" {
		run.Status = "partial_failed"
	}

	targets, err := s.store.ListSubscriberNotificationTargets(ctx, component.ID)
	if err != nil {
		run.ErrorMessage = err.Error()
		_ = s.store.FinishComponentCheckRun(ctx, run)
		return err
	}
	if len(targets) == 0 {
		log.Printf("notify check run summary skipped run_id=%d component_id=%d reason=no_subscribers", runID, component.ID)
	}
	var errs []error
	sentAny := false
	skippedCount := 0
	for _, target := range targets {
		baseline := target.LastNotifiedVersion
		if baseline == "" {
			baseline = component.CurrentVersion
		}
		hasVersionUpdate := record.Status == "success" && record.LatestVersion != "" && version.IsNewer(record.LatestVersion, baseline)
		hasSecurityRisk := affectedCount > 0
		if !hasVersionUpdate && !hasSecurityRisk {
			skippedCount++
			log.Printf("notify check run summary skipped recipient=%s run_id=%d component_id=%d reason=no_change baseline=%s latest=%s affected=%d", target.Email, runID, component.ID, baseline, record.LatestVersion, affectedCount)
			continue
		}
		sent, err := s.store.HasSentNotificationFingerprint(ctx, component.ID, "component_check_summary", fingerprint, target.Email)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if sent {
			skippedCount++
			log.Printf("notify check run summary skipped recipient=%s run_id=%d component_id=%d reason=duplicate fingerprint=%s", target.Email, runID, component.ID, fingerprint)
			if hasVersionUpdate {
				if err := s.store.UpsertSubscriberComponentProgress(ctx, target.SubscriberID, component.ID, record.LatestVersion); err != nil {
					errs = append(errs, err)
				}
			}
			continue
		}
		subject := buildCheckRunSubject(component, hasVersionUpdate, hasSecurityRisk)
		body := buildCheckRunMailBody(component, record, profile, records, affectedCount, hasVersionUpdate, hasSecurityRisk)
		log.Printf("notify check run summary sending recipient=%s run_id=%d component_id=%d version_update=%t security_risk=%t", target.Email, runID, component.ID, hasVersionUpdate, hasSecurityRisk)
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
			sentAny = true
		}
		versionKey := record.LatestVersion
		if versionKey == "" {
			versionKey = profile.SecuritySuggestedVersion
		}
		if versionKey == "" && len(fingerprint) >= 12 {
			versionKey = fingerprint[:12]
		}
		if err := s.store.CreateNotificationRecord(ctx, &storage.NotificationRecord{
			RunID:          runID,
			ComponentID:    component.ID,
			CheckRecordID:  record.ID,
			Type:           "component_check_summary",
			Fingerprint:    fingerprint,
			Version:        versionKey,
			RecipientEmail: target.Email,
			Subject:        subject,
			Body:           body,
			Status:         status,
			ErrorMessage:   errorMessage,
			SentAt:         sentAt,
		}); err != nil {
			errs = append(errs, err)
			log.Printf("notify check run record write failed component_id=%d run_id=%d recipient=%s err=%v", component.ID, runID, target.Email, err)
		}
		if sendErr == nil && hasVersionUpdate {
			if err := s.store.UpsertSubscriberComponentProgress(ctx, target.SubscriberID, component.ID, record.LatestVersion); err != nil {
				errs = append(errs, err)
			}
		}
		if sendErr != nil {
			log.Printf("notify check run summary failed recipient=%s run_id=%d component_id=%d err=%v", target.Email, runID, component.ID, sendErr)
		} else {
			log.Printf("notify check run summary finished recipient=%s run_id=%d component_id=%d", target.Email, runID, component.ID)
		}
	}
	if sentAny {
		now := time.Now().UTC()
		run.NotifiedAt = &now
	}
	if len(errs) > 0 {
		run.ErrorMessage = errors.Join(errs...).Error()
		if run.Status == "success" {
			run.Status = "partial_failed"
		}
	}
	if err := s.store.FinishComponentCheckRun(ctx, run); err != nil {
		errs = append(errs, err)
	}
	log.Printf("component check run finished run_id=%d component_id=%d status=%s security_status=%s affected=%d notified=%t skipped=%d", runID, component.ID, run.Status, run.SecurityStatus, run.AffectedVulnerabilityCount, sentAny, skippedCount)
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

func buildCheckRunSubject(component storage.Component, hasVersionUpdate, hasSecurityRisk bool) string {
	switch {
	case hasVersionUpdate && hasSecurityRisk:
		return fmt.Sprintf("[开源组件提醒] %s 发现新版本和安全风险", component.Name)
	case hasSecurityRisk:
		return fmt.Sprintf("[开源组件安全] %s 发现安全风险", component.Name)
	default:
		return fmt.Sprintf("[开源组件更新] %s 发现新版本", component.Name)
	}
}

func buildCheckRunMailBody(component storage.Component, record storage.CheckRecord, profile storage.ComponentSecurityProfile, records []storage.ComponentSecurityRecord, affectedCount int, hasVersionUpdate, hasSecurityRisk bool) string {
	publishedAt := ""
	if record.ReleasePublishedAt != nil {
		publishedAt = record.ReleasePublishedAt.Format(time.RFC3339)
	}
	versionLine := "未发现新版本"
	if hasVersionUpdate {
		versionLine = fmt.Sprintf("%s -> %s", record.PreviousVersion, record.LatestVersion)
	}
	securityLine := "未发现安全风险"
	if hasSecurityRisk {
		securityLine = fmt.Sprintf("发现 %d 个漏洞", affectedCount)
		if profile.SecuritySuggestedVersion != "" {
			securityLine += fmt.Sprintf("，建议升级至 %s", profile.SecuritySuggestedVersion)
		}
	}
	vulnerabilityDetails := buildVulnerabilityDetails(records)
	return fmt.Sprintf(`组件名称：%s
仓库地址：%s
当前使用版本：%s
版本检查：%s
安全检查：%s
发布时间：%s
GitHub 链接：%s

漏洞明细：
%s

Release Note 摘要：
%s

建议动作：
- 结合版本更新和安全风险统一评估升级
- 如存在安全风险，建议优先确认受影响范围
- 详情请进入系统查看组件和漏洞信息
`, component.Name, component.RepoURL, component.CurrentVersion, versionLine, securityLine, publishedAt, record.ReleaseURL, vulnerabilityDetails, record.ReleaseNoteSummary)
}

func buildVulnerabilityDetails(records []storage.ComponentSecurityRecord) string {
	affected := make([]storage.ComponentSecurityRecord, 0, len(records))
	for _, record := range records {
		if record.RiskStatus == "affected" {
			affected = append(affected, record)
		}
	}
	if len(affected) == 0 {
		return "- 本次未命中公开已知漏洞"
	}
	const maxMailVulnerabilities = 5
	lines := make([]string, 0, minInt(len(affected), maxMailVulnerabilities)+1)
	for i, record := range affected {
		if i >= maxMailVulnerabilities {
			lines = append(lines, fmt.Sprintf("- 还有 %d 个漏洞未在邮件中展开，请进入系统查看完整列表", len(affected)-maxMailVulnerabilities))
			break
		}
		lines = append(lines, formatMailVulnerability(record))
	}
	return strings.Join(lines, "\n")
}

func formatMailVulnerability(record storage.ComponentSecurityRecord) string {
	identifier := strings.TrimSpace(record.Identifier)
	if identifier == "" {
		identifier = "未命名漏洞"
	}
	summary := strings.TrimSpace(record.Summary)
	if summary == "" {
		summary = "暂无摘要"
	}
	severity := strings.TrimSpace(record.Severity)
	if severity == "" {
		severity = "未知"
	}
	fixedVersion := strings.TrimSpace(record.FixedVersion)
	if fixedVersion == "" {
		fixedVersion = "暂无明确修复版本"
	}
	link := strings.TrimSpace(record.EvidenceURL)
	if link == "" && record.Identifier != "" {
		link = "https://osv.dev/vulnerability/" + record.Identifier
	}
	if link == "" {
		link = "暂无链接"
	}
	return fmt.Sprintf("- %s：%s\n  严重性：%s\n  建议升级至：%s\n  链接：%s", identifier, summary, severity, fixedVersion, link)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func countAffectedSecurityRecords(records []storage.ComponentSecurityRecord) int {
	count := 0
	for _, record := range records {
		if record.RiskStatus == "affected" {
			count++
		}
	}
	return count
}

func checkRunFingerprint(record storage.CheckRecord, profile storage.ComponentSecurityProfile, records []storage.ComponentSecurityRecord, securityStatus string) string {
	ids := make([]string, 0, len(records))
	for _, record := range records {
		if record.RiskStatus != "affected" {
			continue
		}
		if record.Identifier != "" {
			ids = append(ids, record.Identifier)
		}
	}
	sort.Strings(ids)
	payload := strings.Join([]string{
		record.LatestVersion,
		record.Status,
		securityStatus,
		profile.SecuritySuggestedVersion,
		strings.Join(ids, ","),
	}, "|")
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}
