package storagegov

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// CleanupHealth 区分调度心跳、完成与成功；不以 worker_enabled 冒充真实执行。
type CleanupHealth struct {
	OwnerInstance       string     `json:"owner_instance"`
	CycleID             string     `json:"cycle_id"`
	State               string     `json:"state"`
	StartedAt           *time.Time `json:"started_at"`
	FinishedAt          *time.Time `json:"finished_at"`
	LastSuccessAt       *time.Time `json:"last_success_at"`
	ConsecutiveFailures int        `json:"consecutive_failures"`
}

func validCleanupHealth(h CleanupHealth) bool {
	if h.ConsecutiveFailures < 0 || h.ConsecutiveFailures > 1000 {
		return false
	}
	switch h.State {
	case "not_started":
		return true
	case "running":
		return h.StartedAt != nil && h.CycleID != ""
	case "completed", "failed":
		return h.StartedAt != nil && h.FinishedAt != nil && !h.FinishedAt.Before(*h.StartedAt) && h.CycleID != ""
	default:
		return false
	}
}

func (s *Service) cleanupHealth(ctx context.Context) (CleanupHealth, error) {
	h := CleanupHealth{OwnerInstance: s.opts.Identity.InstanceID, State: "not_started"}
	if s.opts.Budget == nil {
		return h, nil
	}
	raw, err := s.opts.Budget.Document(ctx, "cleanup-health", nil)
	if err != nil {
		return h, err
	}
	if len(raw) > 0 && (json.Unmarshal(raw, &h) != nil || h.OwnerInstance != s.opts.Identity.InstanceID || !validCleanupHealth(h)) {
		return h, ErrStateUnavailable
	}
	return h, nil
}
func (s *Service) startCleanupHealth(ctx context.Context) (string, error) {
	if s.opts.Budget == nil {
		return "", nil
	}
	id := uuid.NewString()
	_, err := s.opts.Budget.Document(ctx, "cleanup-health", func(raw []byte) ([]byte, error) {
		h := CleanupHealth{OwnerInstance: s.opts.Identity.InstanceID}
		if len(raw) > 0 && (json.Unmarshal(raw, &h) != nil || h.OwnerInstance != s.opts.Identity.InstanceID || !validCleanupHealth(h)) {
			return nil, ErrStateUnavailable
		}
		now := s.opts.Now().UTC()
		h.CycleID = id
		h.State = "running"
		h.StartedAt = &now
		return json.Marshal(h)
	})
	return id, err
}
func (s *Service) finishCleanupHealth(ctx context.Context, id string, failed bool) error {
	if id == "" || s.opts.Budget == nil {
		return nil
	}
	run, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	_, err := s.opts.Budget.Document(run, "cleanup-health", func(raw []byte) ([]byte, error) {
		var h CleanupHealth
		if json.Unmarshal(raw, &h) != nil || h.OwnerInstance != s.opts.Identity.InstanceID || h.CycleID != id || h.State != "running" {
			return nil, ErrStateUnavailable
		}
		now := s.opts.Now().UTC()
		h.FinishedAt = &now
		h.State = "completed"
		if failed {
			h.State = "failed"
			h.ConsecutiveFailures = min(1000, h.ConsecutiveFailures+1)
		} else {
			h.ConsecutiveFailures = 0
			h.LastSuccessAt = &now
		}
		return json.Marshal(h)
	})
	if err == nil {
		s.PublishChange("cleanup_health_changed")
	}
	return err
}
func evaluateCleanupHealth(now time.Time, p TableMonitoringPolicy, h CleanupHealth, worker bool) []CapacityAlert {
	alerts := []CapacityAlert{}
	if !worker {
		return alerts
	}
	if h.State == "running" && h.StartedAt != nil && now.Sub(*h.StartedAt) > 5*time.Minute {
		alerts = append(alerts, CapacityAlert{Code: "cleanup_cycle_unconfirmed", Level: "warning"})
	}
	if h.ConsecutiveFailures >= p.FailureLimit {
		alerts = append(alerts, CapacityAlert{Code: "cleanup_cycle_failed", Level: "warning"})
	}
	last := p.UpdatedAt
	if h.FinishedAt != nil {
		last = *h.FinishedAt
	}
	if !last.IsZero() && now.Sub(last) > time.Duration(p.StaleAfterMinutes)*time.Minute {
		alerts = append(alerts, CapacityAlert{Code: "cleanup_worker_stalled", Level: "warning"})
	}
	return alerts
}
