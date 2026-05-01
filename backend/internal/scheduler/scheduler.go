package scheduler

import (
	"context"
	"log"
	"time"

	"opensource-release-watcher/backend/internal/service"
)

type Scheduler struct {
	service  *service.Service
	interval time.Duration
}

func New(service *service.Service, interval time.Duration) *Scheduler {
	return &Scheduler{service: service, interval: interval}
}

func (s *Scheduler) Start(ctx context.Context) {
	if s.interval <= 0 {
		return
	}
	log.Printf("scheduler started interval=%s", s.interval)
	ticker := time.NewTicker(s.interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				log.Printf("scheduler stopped")
				return
			case <-ticker.C:
				log.Printf("scheduled check triggered")
				if _, err := s.service.RunChecks(ctx, "scheduler"); err != nil {
					log.Printf("scheduled check failed: %v", err)
				}
			}
		}
	}()
}
