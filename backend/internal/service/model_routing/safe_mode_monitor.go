package model_routing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/cache"
)

// TelemetrySnapshot carries aggregate routing metrics for a tenant scope.
type TelemetrySnapshot struct {
	Env           string
	TenantScope   string
	HitRate       float64
	FallbackRate  float64
	DecisionCount uint64
	Window        time.Duration
}

// SafeModeMonitorOptions tunes the automatic safe-mode thresholds.
type SafeModeMonitorOptions struct {
	MinHitRate           float64
	MaxFallbackRate      float64
	RecoveryHitRate      float64
	RecoveryFallbackRate float64
	MinDecisions         uint64
	Cooldown             time.Duration
	AutoToggleTTL        time.Duration
	ReasonPrefix         string
}

func (o *SafeModeMonitorOptions) normalize() {
	if o.MinHitRate <= 0 {
		o.MinHitRate = 0.9
	}
	if o.MaxFallbackRate <= 0 {
		o.MaxFallbackRate = 0.20
	}
	if o.RecoveryHitRate <= 0 {
		o.RecoveryHitRate = 0.93
	}
	if o.RecoveryFallbackRate <= 0 {
		o.RecoveryFallbackRate = 0.10
	}
	if o.MinDecisions == 0 {
		o.MinDecisions = 50
	}
	if o.Cooldown <= 0 {
		o.Cooldown = 2 * time.Minute
	}
	if o.AutoToggleTTL <= 0 {
		o.AutoToggleTTL = 15 * time.Minute
	}
	if strings.TrimSpace(o.ReasonPrefix) == "" {
		o.ReasonPrefix = "auto-monitor"
	}
}

// SafeModeAlert describes the automatic action for downstream alerts.
type SafeModeAlert struct {
	Env         string
	TenantScope string
	Triggered   string
	Enabled     bool
	Snapshot    TelemetrySnapshot
	Timestamp   time.Time
}

// SafeModeAlertPublisher forwards monitor events to alerting sinks.
type SafeModeAlertPublisher interface {
	PublishSafeModeAlert(ctx context.Context, alert SafeModeAlert)
}

type noopAlertPublisher struct{}

func (noopAlertPublisher) PublishSafeModeAlert(context.Context, SafeModeAlert) {}

// SafeModeMonitor evaluates telemetry snapshots to toggle safe-mode automatically.
type SafeModeMonitor struct {
	service *Service
	cache   cache.ICache
	opts    SafeModeMonitorOptions
	alert   SafeModeAlertPublisher
	clock   func() time.Time
}

// NewSafeModeMonitor wires the monitor with defaults and optional alert publisher.
func NewSafeModeMonitor(svc *Service, cache cache.ICache, opts SafeModeMonitorOptions, alert SafeModeAlertPublisher) *SafeModeMonitor {
	if svc == nil {
		return nil
	}
	if cache == nil {
		cache = svc.cache
	}
	opts.normalize()
	if alert == nil {
		alert = noopAlertPublisher{}
	}
	clock := svc.clock
	if clock == nil {
		clock = time.Now
	}
	return &SafeModeMonitor{
		service: svc,
		cache:   cache,
		opts:    opts,
		alert:   alert,
		clock:   clock,
	}
}

// Evaluate inspects a telemetry snapshot and toggles safe-mode if thresholds are breached.
func (m *SafeModeMonitor) Evaluate(ctx context.Context, snapshot TelemetrySnapshot) error {
	if m == nil || m.service == nil {
		return errors.New("monitor not configured")
	}
	scope := strings.TrimSpace(snapshot.TenantScope)
	if scope == "" {
		return errors.New("tenant scope required")
	}
	env := strings.TrimSpace(snapshot.Env)
	if env == "" {
		env = "default"
	}
	if snapshot.DecisionCount < m.opts.MinDecisions {
		return nil
	}

	currentState, _ := m.service.SafeModeState(ctx, env, scope)
	record, _ := m.loadRecord(ctx, env, scope)
	now := m.clock()
	shouldEnable := snapshot.HitRate < m.opts.MinHitRate || snapshot.FallbackRate > m.opts.MaxFallbackRate
	shouldDisable := currentState != nil &&
		currentState.Enabled &&
		record.AutoEnabled &&
		snapshot.HitRate >= m.opts.RecoveryHitRate &&
		snapshot.FallbackRate <= m.opts.RecoveryFallbackRate

	if shouldEnable && (currentState == nil || !currentState.Enabled) {
		if m.inCooldown(record, now) {
			return nil
		}
		reason := fmt.Sprintf("%s:hit=%.2f,fallback=%.2f", m.opts.ReasonPrefix, snapshot.HitRate, snapshot.FallbackRate)
		if _, err := m.service.ToggleSafeMode(ctx, env, scope, true, m.opts.AutoToggleTTL, "auto-monitor", reason); err != nil {
			return err
		}
		record = monitorRecord{
			AutoEnabled: true,
			LastTrigger: now.Unix(),
		}
		_ = m.saveRecord(ctx, env, scope, record)
		m.service.inst.RecordMetric(ctx, "agent.routing.safe_mode_auto_toggle_total", 1, map[string]string{
			"tenant_scope": scope,
			"env":          env,
			"action":       "enable",
		})
		m.alert.PublishSafeModeAlert(ctx, SafeModeAlert{
			Env:         env,
			TenantScope: scope,
			Triggered:   "enable",
			Enabled:     true,
			Snapshot:    snapshot,
			Timestamp:   now,
		})
		return nil
	}

	if shouldDisable {
		if _, err := m.service.ToggleSafeMode(ctx, env, scope, false, 0, "auto-monitor", "auto-monitor:recovered"); err != nil {
			return err
		}
		_ = m.deleteRecord(ctx, env, scope)
		m.service.inst.RecordMetric(ctx, "agent.routing.safe_mode_auto_toggle_total", 1, map[string]string{
			"tenant_scope": scope,
			"env":          env,
			"action":       "disable",
		})
		m.alert.PublishSafeModeAlert(ctx, SafeModeAlert{
			Env:         env,
			TenantScope: scope,
			Triggered:   "disable",
			Enabled:     false,
			Snapshot:    snapshot,
			Timestamp:   now,
		})
		return nil
	}

	if shouldEnable && currentState != nil && currentState.Enabled {
		// Already enabled manually; just refresh record timestamp for cooldown.
		record.AutoEnabled = record.AutoEnabled || strings.HasPrefix(currentState.Reason, m.opts.ReasonPrefix)
		record.LastTrigger = now.Unix()
		_ = m.saveRecord(ctx, env, scope, record)
	}
	return nil
}

func (m *SafeModeMonitor) monitorKey(env, tenant string) string {
	return fmt.Sprintf("agent:modelhub:safe_mode_monitor:%s:%s", env, tenant)
}

type monitorRecord struct {
	AutoEnabled bool  `json:"auto_enabled"`
	LastTrigger int64 `json:"last_trigger"`
}

func (m *SafeModeMonitor) inCooldown(record monitorRecord, now time.Time) bool {
	if record.LastTrigger == 0 {
		return false
	}
	last := time.Unix(record.LastTrigger, 0)
	return now.Sub(last) < m.opts.Cooldown
}

func (m *SafeModeMonitor) loadRecord(ctx context.Context, env, tenant string) (monitorRecord, error) {
	var rec monitorRecord
	if m.cache == nil {
		return rec, nil
	}
	raw, err := m.cache.Get(ctx, m.monitorKey(env, tenant))
	if err != nil || len(raw) == 0 {
		return rec, err
	}
	_ = json.Unmarshal(raw, &rec)
	return rec, nil
}

func (m *SafeModeMonitor) saveRecord(ctx context.Context, env, tenant string, rec monitorRecord) error {
	if m.cache == nil {
		return nil
	}
	payload, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	return m.cache.Set(ctx, m.monitorKey(env, tenant), payload, 0)
}

func (m *SafeModeMonitor) deleteRecord(ctx context.Context, env, tenant string) error {
	if m.cache == nil {
		return nil
	}
	return m.cache.Delete(ctx, m.monitorKey(env, tenant))
}
