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
	ticker := time.NewTicker(s.interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if _, err := s.service.RunChecks(ctx, "scheduler"); err != nil {
					log.Printf("scheduled check failed: %v", err)
				}
			}
		}
	}()
}
