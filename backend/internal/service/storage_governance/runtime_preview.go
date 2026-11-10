package storagegov

import (
	"context"
	lifecycle "github.com/ArtisanCloud/PowerX/internal/infra/storage_lifecycle"
)

func (s *Service) PreviewRuntimePolicy(ctx context.Context, id string, days int, maxBytes int64) (lifecycle.Preview, error) {
	if rootActor(ctx) == 0 {
		return lifecycle.Preview{}, ErrRootRequired
	}
	if s.opts.Lifecycle == nil {
		return lifecycle.Preview{}, ErrStateUnavailable
	}
	return s.opts.Lifecycle.PreviewPolicy(ctx, id, lifecycle.Policy{RetentionDays: days, MaxBytes: maxBytes})
}
