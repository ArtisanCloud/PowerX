package authorization

import (
	"context"
	"fmt"
	"time"

	eventfabricmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	pxlog "github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/google/uuid"
)

// ChallengeDispatcher 负责向安全运营侧派发 Challenge 审批事件，并在超时时触发告警。
type ChallengeDispatcher interface {
	DispatchChallenge(ctx context.Context, ticket *eventfabricmodel.AuthorizationApprovalTicket, payload ChallengeDispatchPayload) error
	NotifyTimeout(ctx context.Context, ticket *eventfabricmodel.AuthorizationApprovalTicket) error
}

// ChallengeDispatchPayload 补充 Challenge 的上下文信息。
type ChallengeDispatchPayload struct {
	GrantUUID          *uuid.UUID     `json:"grant_uuid,omitempty"`
	TenantID           uuid.UUID      `json:"tenant_id"`
	RequestFingerprint uuid.UUID      `json:"request_fingerprint"`
	SubjectType        string         `json:"subject_type"`
	SubjectID          string         `json:"subject_id"`
	Capabilities       []string       `json:"capabilities,omitempty"`
	Conditions         map[string]any `json:"conditions,omitempty"`
	Metadata           map[string]any `json:"metadata,omitempty"`
	IssuedAt           time.Time      `json:"issued_at"`
	SLAExpiresAt       time.Time      `json:"sla_expires_at"`
}

// ChallengeDispatcherOptions 配置 ChallengeDispatcher。
type ChallengeDispatcherOptions struct {
	EventBus event_bus.EventBus
	Topic    string
	Logger   *pxlog.Logger
	Clock    func() time.Time
}

type challengeDispatcher struct {
	eventBus event_bus.EventBus
	topic    string
	logger   *pxlog.Logger
	clock    func() time.Time
}

// NewChallengeDispatcher 构建 ChallengeDispatcher。
func NewChallengeDispatcher(opts ChallengeDispatcherOptions) ChallengeDispatcher {
	topic := opts.Topic
	if topic == "" {
		topic = "event_fabric.authorization.challenge"
	}
	logger := opts.Logger
	if logger == nil {
		logger = pxlog.GetGlobalLogger()
	}
	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}
	return &challengeDispatcher{
		eventBus: opts.EventBus,
		topic:    topic,
		logger:   logger,
		clock:    clock,
	}
}

func (d *challengeDispatcher) DispatchChallenge(ctx context.Context, ticket *eventfabricmodel.AuthorizationApprovalTicket, payload ChallengeDispatchPayload) error {
	if ticket == nil {
		return fmt.Errorf("challenge ticket is nil")
	}
	event := d.buildEvent("queued", ticket, payload)
	return d.publish(ctx, event)
}

func (d *challengeDispatcher) NotifyTimeout(ctx context.Context, ticket *eventfabricmodel.AuthorizationApprovalTicket) error {
	if ticket == nil {
		return fmt.Errorf("challenge ticket is nil")
	}
	event := d.buildEvent("timeout", ticket, ChallengeDispatchPayload{
		TenantID:           ticket.TenantID,
		RequestFingerprint: ticket.RequestFingerprint,
		IssuedAt:           ticket.CreatedAt,
		SLAExpiresAt:       ticket.SLAExpiresAt,
	})
	return d.publish(ctx, event)
}

func (d *challengeDispatcher) publish(ctx context.Context, event ChallengeEvent) error {
	if d.eventBus == nil {
		d.logger.InfoF(ctx, "[authorization.challenge] event bus not configured, drop event=%+v", event)
		return nil
	}
	d.eventBus.Publish(d.topic, event, ctx)
	return nil
}

func (d *challengeDispatcher) buildEvent(eventType string, ticket *eventfabricmodel.AuthorizationApprovalTicket, payload ChallengeDispatchPayload) ChallengeEvent {
	now := d.clock().UTC()
	issuedAt := payload.IssuedAt
	if issuedAt.IsZero() {
		issuedAt = ticket.CreatedAt
	}
	if issuedAt.IsZero() {
		issuedAt = now
	}
	slaExpiresAt := payload.SLAExpiresAt
	if slaExpiresAt.IsZero() {
		slaExpiresAt = ticket.SLAExpiresAt
	}
	fingerprint := payload.RequestFingerprint
	if fingerprint == uuid.Nil {
		fingerprint = ticket.RequestFingerprint
	}
	tenantID := payload.TenantID
	if tenantID == uuid.Nil {
		tenantID = ticket.TenantID
	}

	return ChallengeEvent{
		Type:               eventType,
		TicketID:           ticket.UUID,
		TenantID:           tenantID,
		GrantID:            ticket.GrantID,
		RequestFingerprint: fingerprint,
		SubjectType:        payload.SubjectType,
		SubjectID:          payload.SubjectID,
		Capabilities:       payload.Capabilities,
		Conditions:         payload.Conditions,
		Metadata:           payload.Metadata,
		IssuedAt:           issuedAt,
		SLAExpiresAt:       slaExpiresAt,
		DispatchedAt:       now,
	}
}

// ChallengeEvent 发布到 EventBus 的统一事件格式。
type ChallengeEvent struct {
	Type               string         `json:"type"`
	TicketID           uuid.UUID      `json:"ticket_id"`
	TenantID           uuid.UUID      `json:"tenant_id"`
	GrantID            *uuid.UUID     `json:"grant_id,omitempty"`
	RequestFingerprint uuid.UUID      `json:"request_fingerprint"`
	SubjectType        string         `json:"subject_type,omitempty"`
	SubjectID          string         `json:"subject_id,omitempty"`
	Capabilities       []string       `json:"capabilities,omitempty"`
	Conditions         map[string]any `json:"conditions,omitempty"`
	Metadata           map[string]any `json:"metadata,omitempty"`
	IssuedAt           time.Time      `json:"issued_at"`
	SLAExpiresAt       time.Time      `json:"sla_expires_at"`
	DispatchedAt       time.Time      `json:"dispatched_at"`
}
