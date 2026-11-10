// Package servicechannel 实现插件服务的可靠事件合同，复用 Event Fabric 的持久化及 ACL。
package servicechannel

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	cap "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	eventaudit "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/audit"
	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/delivery"
	pluginservice "github.com/ArtisanCloud/PowerX/internal/service/plugin"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/database"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	r "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/event_fabric"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	Contract          = "event_fabric.service.v1"
	Endpoint          = "core://event-fabric/service"
	PublishCapability = "com.corex.eventfabric.service_publish"
	ConsumeCapability = "com.corex.eventfabric.service_consume"
)

type Request struct {
	ContractVersion   string   `json:"contract_version"`
	Operation         string   `json:"operation"`
	TenantUUID        string   `json:"tenant_uuid"`
	PluginID          string   `json:"plugin_id"`
	SubscriberID      string   `json:"subscriber_id,omitempty"`
	Topics            []string `json:"topics,omitempty"`
	TopicKeys         []string `json:"topic_keys,omitempty"`
	BatchSize         int      `json:"batch_size,omitempty"`
	SupportedVersions []string `json:"supported_versions,omitempty"`
	CompatibilityMode string   `json:"compatibility_mode,omitempty"`
	Event             *Event   `json:"event,omitempty"`
	DeliveryID        string   `json:"delivery_id,omitempty"`
	Reason            string   `json:"reason,omitempty"`
}
type Event struct {
	EventID        string            `json:"event_id"`
	Topic          string            `json:"topic"`
	Version        string            `json:"version"`
	Payload        json.RawMessage   `json:"payload"`
	IdempotencyKey string            `json:"idempotency_key"`
	TraceID        string            `json:"trace_id,omitempty"`
	Attributes     map[string]string `json:"attributes,omitempty"`
}
type Message struct {
	ContractVersion   string            `json:"contract_version"`
	TenantUUID        string            `json:"tenant_uuid"`
	SubscriberID      string            `json:"subscriber_id"`
	DeliveryID        string            `json:"delivery_id"`
	EventID           string            `json:"event_id"`
	IdempotencyKey    string            `json:"idempotency_key"`
	Attributes        map[string]string `json:"attributes,omitempty"`
	Topic             string            `json:"topic"`
	Version           string            `json:"version"`
	Payload           json.RawMessage   `json:"payload"`
	TraceID           string            `json:"trace_id"`
	PublishedBy       string            `json:"published_by"`
	PublisherPluginID string            `json:"publisher_plugin_id"`
	Attempt           int               `json:"attempt"`
	MaxAttempts       int               `json:"max_attempts"`
	AckDeadline       time.Time         `json:"ack_deadline"`
}
type authority interface {
	CurrentAccess(context.Context) (cap.CredentialAccess, error)
}
type Service struct {
	db          *gorm.DB
	repo        *r.ServiceDeliveryRepository
	access      authority
	clock       func() time.Time
	negotiator  delivery.DefaultVersionNegotiator
	usageGuard  func(context.Context, string) error
	audit       eventaudit.Service
	schemaReady bool
}

func NewService(db *gorm.DB, audit ...eventaudit.Service) *Service {
	s := &Service{db: db, repo: r.NewServiceDeliveryRepository(db), access: cap.NewGrantStatusService(db), clock: time.Now, usageGuard: pluginservice.NewPluginDrainJobService(db).EnsurePluginAcceptsNewUsage}
	if db != nil && db.Config != nil && db.ConnPool != nil {
		s.schemaReady = database.CheckEventServiceDeliverySchema(db) == nil
	}
	if len(audit) > 0 {
		s.audit = audit[0]
	}
	return s
}
func failure(status int, code string) error { return dto.NewErrorWithCode(status, code, code, nil) }
func valid(v string, max int) bool {
	return v != "" && len(v) <= max && strings.TrimSpace(v) == v && !strings.ContainsAny(v, "\x00\r\n")
}

// Check 每次操作及每次发送前实时求值，不信任请求的主体及已有流的旧授权快照。
func (s *Service) Check(ctx context.Context, in Request, capability string) (cap.CredentialAccess, error) {
	var empty cap.CredentialAccess
	if s == nil || s.db == nil {
		return empty, failure(503, "EVENT_FABRIC_UNAVAILABLE")
	}
	if in.Operation != "resolve_topics" && len(in.TopicKeys) > 0 {
		return empty, failure(400, "EVENT_FABRIC_INVALID_ARGUMENT")
	}
	if in.ContractVersion != Contract {
		return empty, failure(400, "EVENT_FABRIC_CONTRACT_REQUIRED")
	}
	claims := reqctx.GetClaims(ctx)
	if !cap.ServiceCredential(ctx) || claims == nil {
		return empty, failure(401, "EVENT_FABRIC_SERVICE_REQUIRED")
	}
	if claims.Issuer == "powerx-sts" && reqctx.STSIssuanceStatus(ctx) != 0 {
		if reqctx.STSIssuanceStatus(ctx) == 503 {
			return empty, failure(503, "EVENT_FABRIC_AUTHORIZATION_UNAVAILABLE")
		}
		return empty, failure(401, "EVENT_FABRIC_SERVICE_REQUIRED")
	}
	tenant, err := reqctx.CanonicalTenantUUID(claims.TenantUUID)
	if err != nil || tenant != reqctx.GetTenantUUID(ctx) || tenant != in.TenantUUID {
		return empty, failure(403, "EVENT_FABRIC_TENANT_FORBIDDEN")
	}
	access, err := s.access.CurrentAccess(ctx)
	if err == nil {
		err = access.Require(capability)
	}
	if err != nil {
		var grant *cap.DirectGrantError
		if errors.As(err, &grant) {
			return empty, failure(grant.HTTPStatus(), "EVENT_FABRIC_GRANT_REQUIRED")
		}
		return empty, failure(503, "EVENT_FABRIC_AUTHORIZATION_UNAVAILABLE")
	}
	if !valid(in.PluginID, 128) || !access.OwnsPlugin(in.PluginID) {
		return empty, failure(403, "EVENT_FABRIC_OWNER_FORBIDDEN")
	}
	if capability == ConsumeCapability && in.SubscriberID != in.PluginID {
		return empty, failure(403, "EVENT_FABRIC_SUBSCRIBER_FORBIDDEN")
	}
	if !s.schemaReady {
		return empty, failure(503, "EVENT_FABRIC_SCHEMA_NOT_READY")
	}
	return access, nil
}

func (s *Service) topic(ctx context.Context, tenant, name string) (*m.TopicDefinition, error) {
	topic, err := s.repo.Topic(ctx, tenant, name, uuid.Nil)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, failure(403, "EVENT_FABRIC_TOPIC_FORBIDDEN")
	}
	if err != nil {
		return nil, failure(503, "EVENT_FABRIC_UNAVAILABLE")
	}
	return topic, nil
}
func (s *Service) can(ctx context.Context, tenant string, topic uuid.UUID, principal, action string) error {
	ok, err := r.NewAclRepository(s.db).HasPermission(ctx, tenant, topic, principal, action, s.clock().UTC())
	if err != nil {
		return failure(503, "EVENT_FABRIC_AUTHORIZATION_UNAVAILABLE")
	}
	if !ok {
		return failure(403, "EVENT_FABRIC_TOPIC_FORBIDDEN")
	}
	return nil
}
func eventKey(tenant, plugin, id string) string {
	h := sha256.Sum256([]byte(tenant + "\x00" + plugin + "\x00" + id))
	return r.ServiceEventPrefix + hex.EncodeToString(h[:])
}

func (s *Service) Publish(ctx context.Context, in Request) (result map[string]any, err error) {
	start := time.Now()
	defer func() { s.record(ctx, in, result, err, start) }()
	access, err := s.Check(ctx, in, PublishCapability)
	if err != nil {
		return nil, err
	}
	e := in.Event
	if in.Operation != "publish" || e == nil || !valid(e.EventID, 128) || !valid(e.IdempotencyKey, 128) || !valid(e.Version, 32) || !valid(e.Topic, 255) || !json.Valid(e.Payload) || len(e.Payload) > 1<<20 || (e.TraceID != "" && !valid(e.TraceID, 128)) || len(e.Attributes) > 64 {
		return nil, failure(400, "EVENT_FABRIC_INVALID_ARGUMENT")
	}
	if s.usageGuard != nil {
		if err := s.usageGuard(ctx, in.PluginID); err != nil {
			return nil, err
		}
	}
	for k, v := range e.Attributes {
		if !valid(k, 64) || len(v) > 2048 || k == "principal_id" || k == "tenant_uuid" || k == "plugin_id" || k == "published_by" || strings.HasPrefix(k, "core_") {
			return nil, failure(400, "EVENT_FABRIC_RESERVED_ATTRIBUTE")
		}
	}
	topic, err := s.topic(ctx, in.TenantUUID, e.Topic)
	if err != nil {
		return nil, err
	}
	if err = s.can(ctx, in.TenantUUID, topic.UUID, in.PluginID, "publish"); err != nil {
		return nil, err
	}
	bindings, err := r.NewAclRepository(s.db).ListByTopic(ctx, in.TenantUUID, topic.UUID)
	if err != nil {
		return nil, failure(503, "EVENT_FABRIC_AUTHORIZATION_UNAVAILABLE")
	}
	recipients := []string{}
	seen := map[string]bool{}
	now := s.clock().UTC()
	for _, b := range bindings {
		if b.Action == "subscribe" && b.Status == 1 && (b.ExpiresAt == nil || b.ExpiresAt.After(now)) && !seen[b.PrincipalID] {
			recipients = append(recipients, b.PrincipalID)
			seen[b.PrincipalID] = true
		}
	}
	headers, _ := json.Marshal(map[string]any{"event_id": e.EventID, "publisher_plugin_id": in.PluginID, "attributes": e.Attributes, "core_subject": access.Subject})
	digest := sha256.Sum256(e.Payload)
	ack := topic.AckTimeoutSec
	if ack <= 0 {
		ack = 30
	}
	max := topic.MaxRetry
	if max <= 0 {
		max = 5
	}
	trace := reqctx.GetTraceID(ctx)
	if trace == "" {
		trace = e.TraceID
	}
	if !valid(trace, 128) {
		trace = uuid.NewString()
	}
	storedSubject := access.Subject
	if len(storedSubject) > 128 {
		sum := sha256.Sum256([]byte(storedSubject))
		storedSubject = "sha256:" + hex.EncodeToString(sum[:])
	}
	envelope := &m.EventEnvelope{TenantKey: in.TenantUUID, EventID: eventKey(in.TenantUUID, in.PluginID, e.IdempotencyKey), TopicUUID: topic.UUID, Version: e.Version, PayloadFormat: "json", Payload: datatypes.JSON(e.Payload), PayloadDigest: hex.EncodeToString(digest[:]), Headers: datatypes.JSON(headers), PublishedBy: storedSubject, PublishedAt: now, IdempotencyKey: e.IdempotencyKey, TraceID: trace, Status: "pending", MaxRetry: max, AckTimeoutSec: ack}
	if err = s.repo.Publish(ctx, envelope, recipients); err != nil {
		if errors.Is(err, r.ErrServiceEventConflict) {
			return nil, failure(409, "EVENT_FABRIC_IDEMPOTENCY_CONFLICT")
		}
		return nil, failure(503, "EVENT_FABRIC_UNAVAILABLE")
	}
	return map[string]any{"contract_version": Contract, "accepted": true, "event_id": e.EventID, "trace_id": envelope.TraceID}, nil
}

// ValidateSubscription 在无待投递事件时也校验每一个 topic，拒绝未授权的空订阅。
func (s *Service) ValidateSubscription(ctx context.Context, in Request) error {
	if _, err := s.Check(ctx, in, ConsumeCapability); err != nil {
		return err
	}
	if in.Operation != "subscribe" || len(in.Topics) == 0 || len(in.Topics) > 100 || in.BatchSize < 1 || in.BatchSize > 100 || len(in.SupportedVersions) == 0 || len(in.SupportedVersions) > 32 {
		return failure(400, "EVENT_FABRIC_INVALID_ARGUMENT")
	}
	if in.CompatibilityMode != "strict" && in.CompatibilityMode != "backward" && in.CompatibilityMode != "any" {
		return failure(400, "EVENT_FABRIC_INVALID_ARGUMENT")
	}
	for _, name := range in.Topics {
		if !valid(name, 255) {
			return failure(400, "EVENT_FABRIC_INVALID_ARGUMENT")
		}
		topic, err := s.topic(ctx, in.TenantUUID, name)
		if err != nil {
			return err
		}
		if err = s.can(ctx, in.TenantUUID, topic.UUID, in.PluginID, "subscribe"); err != nil {
			return err
		}
	}
	for _, version := range in.SupportedVersions {
		if !valid(version, 32) {
			return failure(400, "EVENT_FABRIC_INVALID_ARGUMENT")
		}
	}
	return nil
}

func (s *Service) Poll(ctx context.Context, in Request) ([]Message, error) {
	if err := s.ValidateSubscription(ctx, in); err != nil {
		return nil, err
	}
	rows, err := s.repo.Ready(ctx, in.TenantUUID, in.SubscriberID, in.Topics, s.clock().UTC(), in.BatchSize)
	if err != nil {
		return nil, failure(503, "EVENT_FABRIC_UNAVAILABLE")
	}
	out := []Message{}
	for _, row := range rows {
		var message *Message
		err = s.repo.WithDelivery(ctx, in.TenantUUID, in.SubscriberID, row.UUID, func(current *m.DeliveryAttempt, envelope *m.EventEnvelope) (*m.DeliveryAttempt, *m.DlqMessage, error) {
			claimAccess, e := s.Check(ctx, in, ConsumeCapability)
			if e != nil {
				return nil, nil, e
			}
			topic, e := s.topicByEnvelope(ctx, in, envelope)
			if e != nil {
				return nil, nil, e
			}
			selected := false
			for _, v := range in.Topics {
				if v == topic.FullTopic {
					selected = true
				}
			}
			if !selected {
				return nil, nil, nil
			}
			negotiation := s.negotiator.Negotiate(delivery.CompatibilityMode(in.CompatibilityMode), envelope.Version, in.SupportedVersions)
			if !negotiation.Compatible {
				return nil, nil, failure(409, "EVENT_FABRIC_VERSION_INCOMPATIBLE")
			}
			now := s.clock().UTC()
			if current.Status == "succeeded" || current.Status == "failed" || current.ScheduledAt != nil && current.ScheduledAt.After(now) {
				return nil, nil, nil
			}
			var next *m.DeliveryAttempt
			var dead *m.DlqMessage
			claim := current
			if current.Status == "delivering" {
				next, dead = retry(current, envelope, now, "ack_timeout")
				if next == nil {
					return nil, dead, nil
				}
				claim = next
			}
			deadline := now.Add(time.Duration(envelope.AckTimeoutSec) * time.Second)
			claim.ClaimedBy = subjectFingerprint(claimAccess.Subject)
			claim.Status = "delivering"
			claim.LastAttemptAt = &now
			claim.ScheduledAt = &deadline
			message = makeMessage(claim, envelope, topic.FullTopic, deadline)
			return next, dead, nil
		})
		if err != nil {
			return nil, translate(err)
		}
		if message != nil {
			out = append(out, *message)
		}
	}
	return out, nil
}

func (s *Service) topicByEnvelope(ctx context.Context, in Request, envelope *m.EventEnvelope) (*m.TopicDefinition, error) {
	topic, err := s.repo.Topic(ctx, in.TenantUUID, "", envelope.TopicUUID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, failure(403, "EVENT_FABRIC_TOPIC_FORBIDDEN")
	}
	if err != nil {
		return nil, failure(503, "EVENT_FABRIC_UNAVAILABLE")
	}
	if err := s.can(ctx, in.TenantUUID, topic.UUID, in.PluginID, "subscribe"); err != nil {
		return nil, err
	}
	return topic, nil
}

func makeMessage(row *m.DeliveryAttempt, envelope *m.EventEnvelope, topic string, deadline time.Time) *Message {
	var h struct {
		EventID    string            `json:"event_id"`
		PluginID   string            `json:"publisher_plugin_id"`
		Attributes map[string]string `json:"attributes"`
		Subject    string            `json:"core_subject"`
	}
	_ = json.Unmarshal(envelope.Headers, &h)
	if h.Subject == "" {
		h.Subject = envelope.PublishedBy
	}
	return &Message{ContractVersion: Contract, TenantUUID: envelope.TenantKey, SubscriberID: row.SubscriberID, DeliveryID: row.UUID.String(), EventID: h.EventID, IdempotencyKey: envelope.IdempotencyKey, Attributes: h.Attributes, Topic: topic, Version: envelope.Version, Payload: json.RawMessage(envelope.Payload), TraceID: envelope.TraceID, PublishedBy: h.Subject, PublisherPluginID: h.PluginID, Attempt: row.DeliveryNo, MaxAttempts: envelope.MaxRetry, AckDeadline: deadline}
}

// retry 为每次重新交付生成新 UUID。旧 delivery_id 永远不能确认新一次尝试。
func retry(row *m.DeliveryAttempt, envelope *m.EventEnvelope, now time.Time, reason string) (*m.DeliveryAttempt, *m.DlqMessage) {
	row.Status = "failed"
	row.LastErrorCode = reason
	envelope.RetryCount = row.DeliveryNo
	envelope.LastError = reason
	if row.DeliveryNo >= envelope.MaxRetry {
		envelope.Status = "failed"
		return nil, &m.DlqMessage{TenantKey: row.TenantKey, TopicUUID: envelope.TopicUUID, EnvelopeUUID: envelope.UUID, EventID: envelope.EventID, FailureStage: "delivery", LastErrorCode: reason, LastErrorMsg: reason, PayloadSnapshot: envelope.Payload, Headers: envelope.Headers, Status: "queued", TraceID: envelope.TraceID}
	}
	next := &m.DeliveryAttempt{TenantKey: row.TenantKey, EnvelopeUUID: row.EnvelopeUUID, EventID: row.EventID, SubscriberID: row.SubscriberID, DeliveryNo: row.DeliveryNo + 1, Status: "pending", ScheduledAt: &now, TraceID: row.TraceID}
	next.UUID = uuid.New()
	return next, nil
}

func (s *Service) Settle(ctx context.Context, in Request) (result map[string]any, err error) {
	start := time.Now()
	defer func() { s.record(ctx, in, result, err, start) }()
	receiptAccess, checkErr := s.Check(ctx, in, ConsumeCapability)
	if checkErr != nil {
		return nil, checkErr
	}
	id, err := uuid.Parse(in.DeliveryID)
	if err != nil || id == uuid.Nil || (in.Operation != "ack" && in.Operation != "nack") || len(in.Reason) > 256 {
		return nil, failure(400, "EVENT_FABRIC_INVALID_ARGUMENT")
	}
	outcome, remaining, delay := "acked", 0, 0
	err = s.repo.WithDelivery(ctx, in.TenantUUID, in.SubscriberID, id, func(row *m.DeliveryAttempt, envelope *m.EventEnvelope) (*m.DeliveryAttempt, *m.DlqMessage, error) {
		if _, e := s.topicByEnvelope(ctx, in, envelope); e != nil {
			return nil, nil, e
		}
		if row.ClaimedBy != subjectFingerprint(receiptAccess.Subject) {
			return nil, nil, failure(403, "EVENT_FABRIC_DELIVERY_FORBIDDEN")
		}
		if row.Status == "succeeded" && in.Operation == "ack" {
			return nil, nil, nil
		}
		now := s.clock().UTC()
		if row.Status != "delivering" || row.ScheduledAt == nil || !row.ScheduledAt.After(now) {
			return nil, nil, failure(409, "EVENT_FABRIC_RECEIPT_STALE")
		}
		if in.Operation == "nack" {
			next, dead := retry(row, envelope, now.Add(time.Second), "consumer_nack")
			row.NackReason = in.Reason
			outcome = "dead_lettered"
			if next != nil {
				outcome = "retry_scheduled"
				remaining = envelope.MaxRetry - next.DeliveryNo + 1
				delay = 1
			}
			return next, dead, nil
		}
		row.Status = "succeeded"
		row.AckedAt = &now
		return nil, nil, nil
	})
	if err != nil {
		return nil, translate(err)
	}
	return map[string]any{"contract_version": Contract, "confirmed": true, "delivery_id": in.DeliveryID, "outcome": outcome, "remaining_attempts": remaining, "next_delay_seconds": delay}, nil
}

// record 复用统一审计，仅记录可信租户、主体和关联标识，不保存请求正文、凭据或原始依赖错误。
func (s *Service) record(ctx context.Context, in Request, result map[string]any, err error, start time.Time) {
	if s == nil || s.audit == nil || reqctx.GetClaims(ctx) == nil {
		return
	}
	auditContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	c := reqctx.GetClaims(ctx)
	actor := reqctx.GetSubject(ctx)
	if actor == "" {
		actor = c.Subject
	}
	if access, e := s.access.CurrentAccess(auditContext); e == nil && access.Subject != "" {
		actor = access.Subject
	}
	trace := reqctx.GetTraceID(ctx)
	if v, ok := result["trace_id"].(string); ok {
		trace = v
	}
	if !valid(trace, 128) {
		trace = uuid.NewString()
	}
	id, topic := in.DeliveryID, ""
	if in.Event != nil {
		id = in.Event.EventID
		topic = in.Event.Topic
	}
	status, code := "SUCCESS", ""
	if err != nil {
		status = "FAILED"
		code = dto.CodeOf(err)
		if code == "" {
			code = "EVENT_FABRIC_UNAVAILABLE"
		}
	}
	_ = s.audit.Write(auditContext, eventaudit.Record{ID: id, TenantID: reqctx.GetTenantUUID(ctx), Topic: topic, PrincipalID: actor, Action: "service_" + in.Operation, Status: status, TraceID: trace, LatencyMs: time.Since(start).Milliseconds(), HappenedAt: start.UTC(), ErrorMessage: code, Metadata: map[string]string{"contract_version": Contract, "requested_plugin_id": in.PluginID, "subscriber_id": in.SubscriberID, "delivery_id": in.DeliveryID, "code": code}})
}
func translate(err error) error {
	if dto.CodeOf(err) != "" {
		return err
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return failure(403, "EVENT_FABRIC_DELIVERY_FORBIDDEN")
	}
	return failure(503, "EVENT_FABRIC_UNAVAILABLE")
}

func subjectFingerprint(subject string) string {
	sum := sha256.Sum256([]byte(subject))
	return hex.EncodeToString(sum[:])
}
