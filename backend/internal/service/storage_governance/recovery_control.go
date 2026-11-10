package storagegov

import (
	"context"
	"time"

	storagerecovery "github.com/ArtisanCloud/PowerX/internal/infra/storage_recovery"
)

type ExecutionRecovery struct {
	Prepared   bool               `json:"prepared"`
	Resources  []RecoveryResource `json:"resources,omitempty"`
	State      string             `json:"state"`
	Epoch      string             `json:"epoch,omitempty"`
	JobID      string             `json:"job_id,omitempty"`
	SnapshotID string             `json:"snapshot_id,omitempty"`
	CreatedAt  *time.Time         `json:"created_at,omitempty"`
}

// BindRecoveryAnchor 只在 bootstrap 启动消费者前接通已有维护根，不接受 HTTP 路径。
func (s *Service) BindRecoveryAnchor(store *storagerecovery.Store) error {
	if store == nil || s.opts.Control == nil {
		return ErrStateUnavailable
	}
	ctrl := s.opts.Control
	ctrl.mu.Lock()
	defer ctrl.mu.Unlock()
	if ctrl.started || s.opts.RecoveryAnchor != nil {
		return ErrStateUnavailable
	}
	if _, e := store.Read(context.Background()); e != nil {
		return e
	}
	s.opts.RecoveryAnchor = store
	if s.recoveryArchives != nil {
		if e := s.recoveryArchives.BindRecoveryAnchor(store); e != nil {
			return e
		}
	}
	return nil
}
func (s *Service) withRecoveryGuard(ctx context.Context, fn func() error) error {
	if s.opts.RecoveryAnchor == nil {
		if s.opts.RequireRecoveryAnchor {
			return controlError(503, "STORAGE_RECOVERY_ANCHOR_UNAVAILABLE")
		}
		return fn()
	}
	e := s.opts.RecoveryAnchor.WithEpoch(ctx, func(storagerecovery.Epoch) error { return fn() })
	if e == storagerecovery.ErrUnavailable || e == storagerecovery.ErrBusy {
		return controlError(503, "STORAGE_RECOVERY_ANCHOR_UNAVAILABLE")
	}
	return e
}
func (s *Service) recoveryStatus(ctx context.Context) (ExecutionRecovery, error) {
	out := ExecutionRecovery{State: "not_connected"}
	if s.opts.RecoveryAnchor == nil {
		if s.opts.RequireRecoveryAnchor {
			return out, controlError(503, "STORAGE_RECOVERY_ANCHOR_UNAVAILABLE")
		}
		return out, nil
	}
	epoch, e := s.opts.RecoveryAnchor.Read(ctx)
	if e != nil {
		return out, controlError(503, "STORAGE_RECOVERY_ANCHOR_UNAVAILABLE")
	}
	out.State = "none"
	if epoch.Epoch != "" {
		out = ExecutionRecovery{State: "pending", Epoch: epoch.Epoch, JobID: epoch.JobID, SnapshotID: epoch.SnapshotID, CreatedAt: &epoch.CreatedAt}
	}
	return out, nil
}

func (s *Service) finishRecoveryState(ctx context.Context, c ExecutionConfiguration, d executionDocument) (ExecutionConfiguration, executionDocument, error) {
	return s.finishRecoveryStatePinned(ctx, c, d, nil)
}
func (s *Service) finishRecoveryStatePinned(ctx context.Context, c ExecutionConfiguration, d executionDocument, pinned *storagerecovery.Epoch) (ExecutionConfiguration, executionDocument, error) {
	var recovery ExecutionRecovery
	var e error
	if pinned == nil {
		recovery, e = s.recoveryStatus(ctx)
	} else {
		recovery.State = "none"
		if pinned.Epoch != "" {
			recovery = ExecutionRecovery{State: "pending", Epoch: pinned.Epoch, JobID: pinned.JobID, SnapshotID: pinned.SnapshotID, CreatedAt: &pinned.CreatedAt}
		}
	}
	if e != nil {
		return c, d, e
	}
	c.Recovery = recovery
	if recovery.Epoch == "" {
		if d.RecoveryEpoch != "" {
			return c, d, controlError(503, "STORAGE_RECOVERY_ANCHOR_UNAVAILABLE")
		}
		return c, d, nil
	}
	if d.RecoveryEpoch == recovery.Epoch && c.Confirmed {
		c.Recovery.State = "acknowledged"
		return c, d, nil
	}
	c.Mode = "recovery"
	c.Confirmed = false
	c.CanPause = false
	c.CanResume = false
	c.Problem = "STORAGE_RECOVERY_REVIEW_REQUIRED"
	return c, d, nil
}
