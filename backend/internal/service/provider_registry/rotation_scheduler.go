package provider_registry

import (
	"context"
	"sync"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
)

type secretRotationScheduler struct {
	svc    *Service
	cancel context.CancelFunc
	mu     sync.Mutex
}

func newSecretRotationScheduler(svc *Service) *secretRotationScheduler {
	return &secretRotationScheduler{svc: svc}
}

func (s *secretRotationScheduler) start(ctx context.Context, env string, interval time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stop()
	if interval <= 0 {
		interval = defaultRotationInterval
	}
	runCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := s.svc.RotateEnv(runCtx, env); err != nil {
					logger.ErrorF(runCtx, "[provider_registry] secret rotation failed: %v", err)
				}
			case <-runCtx.Done():
				return
			}
		}
	}()
}

func (s *secretRotationScheduler) stop() {
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
}
