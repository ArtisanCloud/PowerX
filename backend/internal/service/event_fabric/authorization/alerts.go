package authorization

import (
	"context"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/event_bus"

	pxlog "github.com/ArtisanCloud/PowerX/pkg/utils/logger"
)

// AlertEmitter 将安全事件推送到告警平台。
type AlertEmitter interface {
	Emit(ctx context.Context, alert AlertEvent)
}

// AlertEvent 描述一次安全告警。
type AlertEvent struct {
	Type        string
	Severity    string
	TenantUUID  string
	SubjectType string
	SubjectID   string
	Capability  string
	Reason      string
	RequestID   string
	GrantID     string
	Metadata    map[string]string
	Timestamp   time.Time
}

// AlertEmitterOptions 配置告警事件发布器。
type AlertEmitterOptions struct {
	EventBus event_bus.EventBus
	Topic    string
	Logger   *pxlog.Logger
	Clock    func() time.Time
}

type alertEmitter struct {
	bus    event_bus.EventBus
	topic  string
	logger *pxlog.Logger
	clock  func() time.Time
}

type noopAlertEmitter struct{}

// NewAlertEmitter 根据配置返回告警发布器。
func NewAlertEmitter(opts AlertEmitterOptions) AlertEmitter {
	topic := strings.TrimSpace(opts.Topic)
	logger := opts.Logger
	if logger == nil {
		logger = pxlog.GetGlobalLogger()
	}
	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}
	if opts.EventBus == nil || topic == "" {
		return noopAlertEmitter{}
	}
	return &alertEmitter{
		bus:    opts.EventBus,
		topic:  topic,
		logger: logger,
		clock:  clock,
	}
}

func (noopAlertEmitter) Emit(context.Context, AlertEvent) {}

func (e *alertEmitter) Emit(ctx context.Context, alert AlertEvent) {
	if e == nil || alert.Type == "" {
		return
	}

	ts := alert.Timestamp
	if ts.IsZero() {
		ts = e.clock().UTC()
	}

	tenantUUID := strings.TrimSpace(alert.TenantUUID)

	payload := map[string]any{
		"type":         alert.Type,
		"severity":     normalizeSeverity(alert.Severity),
		"tenant_uuid":  tenantUUID,
		"subject_type": alert.SubjectType,
		"subject_id":   alert.SubjectID,
		"capability":   alert.Capability,
		"reason":       alert.Reason,
		"request_id":   alert.RequestID,
		"grant_id":     alert.GrantID,
		"timestamp":    ts,
		"metadata":     alert.Metadata,
	}

	e.bus.Publish(e.topic, payload, ctx)
	e.logger.WarnF(ctx, "[authorization.alert] type=%s tenant=%s subject=%s capability=%s reason=%s",
		payload["type"], payload["tenant_uuid"], payload["subject_id"], payload["capability"], payload["reason"])
}

func normalizeSeverity(severity string) string {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "critical", "high":
		return "high"
	case "medium", "warn", "warning":
		return "medium"
	case "low":
		return "low"
	default:
		return "medium"
	}
}
