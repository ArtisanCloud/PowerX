package storagegov

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"sync/atomic"
	"time"

	budget "github.com/ArtisanCloud/PowerX/internal/infra/storage_budget"
	lifecycle "github.com/ArtisanCloud/PowerX/internal/infra/storage_lifecycle"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger/writer"
)

type RuntimeStatus struct {
	BudgetConfiguration     *HostBudgetConfiguration          `json:"budget_configuration,omitempty"`
	LifecycleConfigurations map[string]LifecycleConfiguration `json:"lifecycle_configurations"`
	Configured              bool                              `json:"configured"`
	WorkerEnabled           bool                              `json:"worker_enabled"`
	AdmissionEnabled        bool                              `json:"admission_enabled"`
	Budget                  *budget.Snapshot                  `json:"budget,omitempty"`
	BudgetPolicy            *budget.BudgetPolicy              `json:"budget_policy,omitempty"`
	Lifecycle               *lifecycle.Status                 `json:"lifecycle,omitempty"`
	LogDelivery             *writer.LokiDeliveryStats         `json:"log_delivery,omitempty"`
	NotificationsConfigured bool                              `json:"notifications_configured"`
	LastObservedAt          *time.Time                        `json:"last_observed_at,omitempty"`
	LastNotificationAt      *time.Time                        `json:"last_notification_at,omitempty"`
	NotificationProblem     string                            `json:"notification_problem,omitempty"`
	Problem                 string                            `json:"problem,omitempty"`
	DatabaseCleanupEnabled  bool                              `json:"database_cleanup_enabled"`
}
type RuntimeEvent struct {
	Version    uint64    `json:"version"`
	Reason     string    `json:"reason"`
	ObservedAt time.Time `json:"observed_at"`
}

var runtimeVersion atomic.Uint64

func (s *Service) PublishChange(reason string) {
	s.mu.Lock()
	s.cached = nil
	s.mu.Unlock()
	e := RuntimeEvent{Version: runtimeVersion.Add(1), Reason: reason, ObservedAt: s.opts.Now().UTC()}
	s.eventMu.Lock()
	defer s.eventMu.Unlock()
	for ch := range s.listeners {
		select {
		case ch <- e:
		default:
		}
	}
}
func (s *Service) SubscribeRuntime() (chan RuntimeEvent, func()) {
	ch := make(chan RuntimeEvent, 16)
	s.eventMu.Lock()
	s.listeners[ch] = struct{}{}
	s.eventMu.Unlock()
	return ch, func() { s.eventMu.Lock(); delete(s.listeners, ch); s.eventMu.Unlock() }
}
func (s *Service) runtimeSnapshot(ctx context.Context) RuntimeStatus {
	out := RuntimeStatus{Configured: s.opts.Budget != nil, WorkerEnabled: s.opts.LifecycleWorkerEnabled, AdmissionEnabled: s.opts.AdmissionEnabled, NotificationsConfigured: s.opts.Notify != nil}
	if s.opts.LogDelivery != nil {
		out.LogDelivery = s.opts.LogDelivery()
	}
	if s.opts.Budget != nil {
		if s.opts.Control != nil {
			snapshot, e := s.opts.Budget.ControlSnapshot(ctx)
			if e == nil {
				c, _, _, _ := s.loadHostBudget(ctx, snapshot)
				out.BudgetConfiguration = &c
			}
		}
		b, err := s.opts.Budget.Status(ctx)
		if err != nil {
			out.Problem = "budget_unavailable"
		} else {
			out.Budget = &b
		}
		p, err := s.opts.Budget.Policy(ctx)
		if err != nil {
			out.Problem = "budget_unavailable"
		} else {
			out.BudgetPolicy = &p
		}
		if raw, err := s.opts.Budget.Document(ctx, "alerts", nil); err == nil && len(raw) > 0 {
			var d capacityDelivery
			if json.Unmarshal(raw, &d) == nil {
				out.LastObservedAt = d.ObservedAt
				out.LastNotificationAt = d.DeliveredAt
				out.NotificationProblem = d.Problem
			}
		}
	}
	if s.opts.Lifecycle != nil {
		l, err := s.opts.Lifecycle.Status(ctx)
		if err != nil {
			out.Problem = "lifecycle_unavailable"
		} else {
			out.Lifecycle = &l
			if l.Problem != "" {
				out.Problem = l.Problem
			}
		}
		out.LifecycleConfigurations = map[string]LifecycleConfiguration{}
		for _, source := range s.opts.Lifecycle.RegisteredSources() {
			if source.Supported {
				c, e := s.lifecycleConfiguration(ctx, source.ID)
				if e == nil {
					out.LifecycleConfigurations[source.ID] = c
				}
			}
		}
	}
	if s.opts.Control != nil {
		if p, c, e := s.loadRetentionControl(ctx); e == nil {
			out.DatabaseCleanupEnabled = p.Enabled && c.ExecutionReady && s.opts.DatabaseRetentionAllowed && s.opts.LifecycleWorkerEnabled
		}
	}
	return out
}
func (s *Service) Runtime(ctx context.Context) (RuntimeStatus, error) {
	if rootActor(ctx) == 0 {
		return RuntimeStatus{}, ErrRootRequired
	}
	return s.runtimeSnapshot(ctx), nil
}
func (s *Service) SaveRuntimePolicy(ctx context.Context, id string, revision int64, p lifecycle.Policy) (lifecycle.Policy, error) {
	if s.opts.Control != nil {
		return p, controlError(428, "STORAGE_CONTROL_PRECONDITION_REQUIRED")
	}
	actor := rootActor(ctx)
	if actor == 0 {
		return p, ErrRootRequired
	}
	if s.opts.Lifecycle == nil {
		return p, ErrStateUnavailable
	}
	out, err := s.opts.Lifecycle.Save(ctx, actor, id, revision, p)
	if err == nil {
		s.mu.Lock()
		s.cached = nil
		s.mu.Unlock()
		s.PublishChange("policy_changed")
	}
	return out, err
}
func (s *Service) SaveRuntimeBudget(ctx context.Context, revision int64, p budget.BudgetPolicy) (budget.BudgetPolicy, error) {
	if rootActor(ctx) == 0 {
		return p, ErrRootRequired
	}
	if s.opts.Control != nil {
		return p, controlError(428, "STORAGE_CONTROL_PRECONDITION_REQUIRED")
	}
	if s.opts.Budget == nil {
		return p, ErrStateUnavailable
	}
	out, err := s.opts.Budget.SavePolicy(ctx, revision, p)
	if err == nil {
		s.mu.Lock()
		s.cached = nil
		s.mu.Unlock()
		s.PublishChange("budget_changed")
	}
	return out, err
}
func (s *Service) PreviewRuntime(ctx context.Context, id string) (lifecycle.Preview, error) {
	if rootActor(ctx) == 0 {
		return lifecycle.Preview{}, ErrRootRequired
	}
	if s.opts.Lifecycle == nil {
		return lifecycle.Preview{}, ErrStateUnavailable
	}
	return s.opts.Lifecycle.Preview(ctx, id)
}
func (s *Service) RunRuntime(ctx context.Context, id string) (lifecycle.Run, error) {
	if rootActor(ctx) == 0 {
		return lifecycle.Run{}, ErrRootRequired
	}
	if s.opts.Lifecycle == nil {
		return lifecycle.Run{}, ErrStateUnavailable
	}
	run, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return s.opts.Lifecycle.Run(run, id)
}
func (s *Service) RunRuntimeRequest(ctx context.Context, id, key string, revision int64) (lifecycle.Run, error) {
	if rootActor(ctx) == 0 {
		return lifecycle.Run{}, ErrRootRequired
	}
	if s.opts.Lifecycle == nil {
		return lifecycle.Run{}, ErrStateUnavailable
	}

	run, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return s.opts.Lifecycle.RunWithRevision(run, id, key, revision)
}

type CapacityNotification struct {
	ID           string          `json:"id"`
	HostID       string          `json:"host_id"`
	FilesystemID string          `json:"filesystem_id"`
	Alerts       []CapacityAlert `json:"alerts"`
	Recovered    bool            `json:"recovered"`
	ObservedAt   time.Time       `json:"observed_at"`
}
type capacityDelivery struct {
	OwnerInstance string                `json:"owner_instance,omitempty"`
	OwnerUntil    *time.Time            `json:"owner_until,omitempty"`
	Version       int                   `json:"version"`
	Signature     string                `json:"signature"`
	Pending       *CapacityNotification `json:"pending,omitempty"`
	ObservedAt    *time.Time            `json:"observed_at,omitempty"`
	AttemptedAt   *time.Time            `json:"attempted_at,omitempty"`
	DeliveredAt   *time.Time            `json:"delivered_at,omitempty"`
	Problem       string                `json:"problem,omitempty"`
}

func (s *Service) observeCapacity(ctx context.Context) error {
	if s.opts.Budget == nil {
		return nil
	}
	s.mu.Lock()
	s.cached = nil
	s.mu.Unlock()
	s.observeDatabaseBacklog(ctx)
	out, err := s.Overview(ctx)
	if err != nil {
		return err
	}
	alerts := append([]CapacityAlert(nil), out.Alerts...)
	// 逐表与容量告警共用持久化去重、重试和显式接收人；不新增消息队列。
	sort.Slice(alerts, func(i, j int) bool { return alerts[i].Code+alerts[i].ResourceID < alerts[j].Code+alerts[j].ResourceID })
	raw, _ := json.Marshal(alerts)
	signature := fmt.Sprintf("%x", sha256.Sum256(raw))
	now := s.opts.Now().UTC()
	var pending *CapacityNotification
	_, err = s.opts.Budget.Document(ctx, "alerts", func(raw []byte) ([]byte, error) {
		d := capacityDelivery{Version: 1}
		if len(raw) > 0 && (json.Unmarshal(raw, &d) != nil || d.Version != 1) {
			return nil, ErrStateUnavailable
		}
		d.ObservedAt = &now
		if s.opts.Notify == nil {
			return json.Marshal(d)
		}
		if d.OwnerInstance != "" && d.OwnerInstance != s.opts.Identity.InstanceID && d.OwnerUntil != nil && now.Before(*d.OwnerUntil) {
			return json.Marshal(d)
		}
		until := now.Add(10 * time.Minute)
		d.OwnerInstance = s.opts.Identity.InstanceID
		d.OwnerUntil = &until
		previous := d.Signature
		d.ObservedAt = &now
		if signature != previous {
			d.Signature = signature
			d.Pending = &CapacityNotification{ID: fmt.Sprintf("%x", sha256.Sum256([]byte(s.opts.Identity.HostID+s.opts.Identity.FilesystemID+signature+now.Format(time.RFC3339Nano)))), HostID: s.opts.Identity.HostID, FilesystemID: s.opts.Identity.FilesystemID, Alerts: alerts, Recovered: len(alerts) == 0 && previous != "", ObservedAt: now}
			if len(alerts) == 0 && previous == "" {
				d.Pending = nil
			}
		}
		if d.Pending != nil && s.opts.Notify != nil && (d.AttemptedAt == nil || now.Sub(*d.AttemptedAt) >= 5*time.Minute) {
			d.AttemptedAt = &now
			pending = d.Pending
		}
		return json.Marshal(d)
	})
	if err != nil {
		return err
	}
	if pending != nil {
		run, cancel := context.WithTimeout(ctx, 5*time.Second)
		err = s.opts.Notify(run, *pending)
		cancel()
		_, persistErr := s.opts.Budget.Document(context.WithoutCancel(ctx), "alerts", func(raw []byte) ([]byte, error) {
			var d capacityDelivery
			if json.Unmarshal(raw, &d) != nil {
				return nil, ErrStateUnavailable
			}
			if d.Pending != nil && d.Pending.ID == pending.ID {
				if err == nil {
					d.Pending = nil
					d.DeliveredAt = &now
					d.Problem = ""
				} else {
					d.Problem = "notification_delivery_failed"
				}
			}
			return json.Marshal(d)
		})
		if persistErr != nil {
			return persistErr
		}
	}
	s.PublishChange("capacity_observed")
	return nil
}

// StartRuntime 是有界生命周期调度，不创建私有任务队列；手动操作同样是同步批次。
func (s *Service) StartRuntime(ctx context.Context, onError func(error)) {
	if !s.opts.LifecycleWorkerEnabled || s.opts.Budget == nil {
		return
	}
	s.runtimeOnce.Do(func() {
		go func() {
			observation := time.NewTicker(5 * time.Minute)
			defer observation.Stop()
			lifecycle := time.NewTicker(15 * time.Minute)
			defer lifecycle.Stop()
			s.observeRuntime(ctx, onError)
			s.runRuntimeLifecycle(ctx, onError)
			s.runRuntimeSchedule(ctx, observation.C, lifecycle.C, onError)
		}()
	})
}

// Independent timers prevent observation duration from skipping a lifecycle boundary.
func (s *Service) runRuntimeSchedule(ctx context.Context, observation, lifecycle <-chan time.Time, onError func(error)) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-observation:
			s.observeRuntime(ctx, onError)
		case <-lifecycle:
			s.runRuntimeLifecycle(ctx, onError)
		}
	}
}

func (s *Service) observeRuntime(ctx context.Context, onError func(error)) {
	run, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := s.observeCapacity(run); err != nil && onError != nil {
		onError(err)
	}
}

func (s *Service) runRuntimeLifecycle(ctx context.Context, onError func(error)) {
	cycleID, healthErr := s.startCleanupHealth(ctx)
	if healthErr != nil {
		if onError != nil {
			onError(healthErr)
		}
		return
	}
	failed := false
	reportError := func(err error) {
		failed = true
		if onError != nil {
			onError(err)
		}
	}
	defer func() {
		if err := s.finishCleanupHealth(ctx, cycleID, failed || ctx.Err() != nil); err != nil && onError != nil {
			onError(err)
		}
	}()
	if s.dbRetention != nil && s.opts.DatabaseRetentionAllowed {
		s.runDatabaseRetentionCycle(ctx, reportError)
	}
	s.runSystemTableRetention(ctx, reportError)

	if s.opts.Lifecycle == nil {
		return
	}
	cycle, stop := context.WithTimeout(ctx, 90*time.Second)
	defer stop()
	state, err := s.opts.Lifecycle.Status(cycle)
	if err != nil {
		reportError(err)
		return
	}
	for _, p := range state.Policies {
		if !p.Enabled || p.OwnerInstance != s.opts.Identity.InstanceID {
			continue
		}
		run, cancel := context.WithTimeout(cycle, 20*time.Second)
		result, err := s.opts.Lifecycle.Run(run, p.SourceID)
		cancel()
		if err != nil {
			reportError(err)
		}
		if result.State != "completed" {
			failed = true
		}
		if cycle.Err() != nil {
			return
		}
	}
}

// RuntimeChangeDirectories 仅交给宿主 SSE 监听，不向 HTTP 调用方暴露路径。
func (s *Service) RuntimeChangeDirectories() []string {
	if s.opts.Budget == nil {
		return nil
	}
	return []string{s.opts.Budget.ChangeDirectory()}
}
func (s *Service) AcknowledgeRuntime(ctx context.Context, id string) (lifecycle.Run, error) {
	if rootActor(ctx) == 0 {
		return lifecycle.Run{}, ErrRootRequired
	}
	if s.opts.Lifecycle == nil {
		return lifecycle.Run{}, ErrStateUnavailable
	}
	return s.opts.Lifecycle.AcknowledgeInterrupted(ctx, rootActor(ctx), id)
}
