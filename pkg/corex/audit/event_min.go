// pkg/corex/audit/event_min.go
package audit

import (
	"context"
	"time"
)

type Severity string

const (
	SeverityInfo Severity = "INFO"
	SeverityWarn          = "WARN"
	SeverityErr           = "ERROR"
)

type Outcome string

const (
	OutcomeSuccess Outcome = "SUCCESS"
	OutcomeDenied          = "DENIED"
	OutcomeFailed          = "FAILED"
)

type AuditEvent struct {
	OccurredAt    time.Time
	TenantID      int64
	CorrelationID string
	Source        string
	Operation     string
	ResourceType  string
	ResourceID    string
	ResourceName  string
	Outcome       Outcome
	Severity      Severity
	Meta          map[string]any
}

func NewEventFromCtx(ctx context.Context) *EventBuilder {
	// 从 ctx 读取 tenant_id、user_id、request_id/trace_id 等（与 logger 同源）
	// …略…
	return &EventBuilder{evt: &AuditEvent{OccurredAt: time.Now()}}
}

type EventBuilder struct{ evt *AuditEvent }

func (b *EventBuilder) Source(s string) *EventBuilder     { b.evt.Source = s; return b }
func (b *EventBuilder) Operation(op string) *EventBuilder { b.evt.Operation = op; return b }
func (b *EventBuilder) Resource(t, id, name string) *EventBuilder {
	b.evt.ResourceType, b.evt.ResourceID, b.evt.ResourceName = t, id, name
	return b
}
func (b *EventBuilder) Outcome(o Outcome) *EventBuilder     { b.evt.Outcome = o; return b }
func (b *EventBuilder) Severity(s Severity) *EventBuilder   { b.evt.Severity = s; return b }
func (b *EventBuilder) Meta(m map[string]any) *EventBuilder { b.evt.Meta = m; return b }
func (b *EventBuilder) Build() *AuditEvent                  { return b.evt }

func httpOutcome(code int) Outcome {
	if code >= 200 && code < 400 {
		return OutcomeSuccess
	}
	if code == 403 {
		return OutcomeDenied
	}
	return OutcomeFailed
}
func sevByHTTP(code int) Severity {
	if code >= 500 {
		return SeverityErr
	}
	if code >= 400 {
		return SeverityWarn
	}
	return SeverityInfo
}
