package dlq

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	eventaudit "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/audit"
	eventdelivery "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/delivery"
	eventmetrics "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/metrics"
	eventfabricmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	eventfabricrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/event_fabric"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// Message 描述进入死信队列的事件摘要。
type Message struct {
	ID             string
	TenantUUID     string
	Topic          string
	EventID        string
	RetryCount     int32
	FailureStage   string
	LastErrorCode  string
	LastError      string
	CreatedAt      time.Time
	Status         string
	ReplayEligible bool
}

// ListRequest 用于分页查询死信消息。
type ListRequest struct {
	TenantUUID string
	TopicID    string
	Status     string
	Page       int
	PageSize   int
}

// ReplayRequest 描述批量重放参数。
type ReplayRequest struct {
	MessageIDs []string
	OperatorID string
	Notes      string
}

// Service 定义死信管理能力。
type Service interface {
	List(ctx context.Context, req ListRequest) ([]*Message, int64, error)
	Replay(ctx context.Context, req ReplayRequest) (int, error)
	Purge(ctx context.Context, tenantID string, topicID string) (int, error)
}

type dlqRepository interface {
	List(ctx context.Context, tenantKey string, topic uuid.UUID, statuses []string, page, pageSize int) ([]*eventfabricmodel.DlqMessage, int64, error)
	FindByUUID(ctx context.Context, id uuid.UUID) (*eventfabricmodel.DlqMessage, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, updates map[string]interface{}) error
	PurgeByTopic(ctx context.Context, tenantKey string, topic uuid.UUID) (int64, error)
}

type envelopeStore interface {
	FindByUUID(ctx context.Context, id uuid.UUID) (*eventfabricmodel.EventEnvelope, error)
}

type topicStore interface {
	FindByUUID(ctx context.Context, id uuid.UUID) (*eventfabricmodel.TopicDefinition, error)
}

type Options struct {
	DB         *gorm.DB
	Repository dlqRepository
	Envelopes  envelopeStore
	Topics     topicStore
	Delivery   eventdelivery.Service
	Audit      eventaudit.Service
	Clock      func() time.Time
	Metrics    eventmetrics.Recorder
}

type serviceImpl struct {
	repo      dlqRepository
	envelopes envelopeStore
	topics    topicStore
	delivery  eventdelivery.Service
	audit     eventaudit.Service
	clock     func() time.Time
	metrics   eventmetrics.Recorder
}

func NewService(opts Options) (Service, error) {
	clock := opts.Clock
	if clock == nil {
		clock = func() time.Time { return time.Now().UTC() }
	}

	repo := opts.Repository
	if repo == nil {
		if opts.DB == nil {
			return nil, fmt.Errorf("dlq service requires repository or db handle")
		}
		repo = eventfabricrepo.NewDlqRepository(opts.DB)
	}

	envelopes := opts.Envelopes
	if envelopes == nil {
		if opts.DB == nil {
			return nil, fmt.Errorf("dlq service requires envelope store or db handle")
		}
		envelopes = eventfabricrepo.NewEnvelopeRepository(opts.DB)
	}

	topics := opts.Topics
	if topics == nil {
		if opts.DB == nil {
			return nil, fmt.Errorf("dlq service requires topic store or db handle")
		}
		topics = eventfabricrepo.NewTopicRepository(opts.DB)
	}

	return &serviceImpl{
		repo:      repo,
		envelopes: envelopes,
		topics:    topics,
		delivery:  opts.Delivery,
		audit:     opts.Audit,
		clock:     clock,
		metrics: func() eventmetrics.Recorder {
			if opts.Metrics != nil {
				return opts.Metrics
			}
			return eventmetrics.NewNoop()
		}(),
	}, nil
}

func (s *serviceImpl) List(ctx context.Context, req ListRequest) ([]*Message, int64, error) {
	if s.repo == nil {
		return nil, 0, fmt.Errorf("dlq repository not configured")
	}
	tenantKey, err := resolveTenantKey(req.TenantUUID)
	if err != nil {
		return nil, 0, err
	}

	var topicUUID uuid.UUID
	if strings.TrimSpace(req.TopicID) != "" {
		topicUUID, err = uuid.Parse(req.TopicID)
		if err != nil {
			return nil, 0, fmt.Errorf("invalid topic id: %w", err)
		}
	}

	var statuses []string
	if strings.TrimSpace(req.Status) != "" {
		statuses = []string{strings.ToLower(req.Status)}
	}

	rows, total, err := s.repo.List(ctx, tenantKey, topicUUID, statuses, req.Page, req.PageSize)
	if err != nil {
		return nil, 0, err
	}

	messages := make([]*Message, 0, len(rows))
	for _, row := range rows {
		var retryCount int32
		if s.envelopes != nil {
			if envelope, err := s.envelopes.FindByUUID(ctx, row.EnvelopeUUID); err == nil && envelope != nil {
				retryCount = int32(envelope.RetryCount)
			}
		}

		messages = append(messages, &Message{
			ID:             row.UUID.String(),
			TenantUUID:     row.TenantKey,
			Topic:          row.TopicUUID.String(),
			EventID:        row.EventID,
			RetryCount:     retryCount,
			FailureStage:   row.FailureStage,
			LastErrorCode:  row.LastErrorCode,
			LastError:      row.LastErrorMsg,
			CreatedAt:      row.CreatedAt,
			Status:         row.Status,
			ReplayEligible: strings.EqualFold(row.Status, "queued"),
		})
	}
	return messages, total, nil
}

func (s *serviceImpl) Replay(ctx context.Context, req ReplayRequest) (int, error) {
	if s.repo == nil || s.delivery == nil {
		return 0, fmt.Errorf("dlq service not fully initialized")
	}
	if len(req.MessageIDs) == 0 {
		return 0, fmt.Errorf("no message ids provided")
	}

	var success int
	for _, id := range req.MessageIDs {
		messageID, err := uuid.Parse(strings.TrimSpace(id))
		if err != nil {
			return success, fmt.Errorf("invalid message id %s: %w", id, err)
		}

		msg, err := s.repo.FindByUUID(ctx, messageID)
		if err != nil {
			return success, err
		}
		if msg == nil {
			continue
		}

		envelope, err := s.envelopes.FindByUUID(ctx, msg.EnvelopeUUID)
		if err != nil {
			return success, err
		}
		if envelope == nil {
			continue
		}

		topic, err := s.topics.FindByUUID(ctx, msg.TopicUUID)
		if err != nil {
			return success, err
		}
		if topic == nil {
			continue
		}

		headers := mergeHeaders(envelope.Headers, msg.Headers)

		newEventID := fmt.Sprintf("%s-replay-%d", msg.EventID, s.clock().UnixNano())
		pubReq := eventdelivery.PublishRequest{
			TenantUUID:     envelope.TenantKey,
			Topic:          topic.FullTopic,
			EventID:        newEventID,
			TraceID:        envelope.TraceID,
			Version:        envelope.Version,
			Payload:        copyJSON(envelope.Payload),
			PayloadFormat:  envelope.PayloadFormat,
			IdempotencyKey: fmt.Sprintf("replay:%s", msg.EventID),
			Attributes:     headers,
		}
		pubReq.Attributes["replay_of"] = msg.EventID
		if req.OperatorID != "" {
			pubReq.Attributes["replay_operator"] = req.OperatorID
		}
		if req.Notes != "" {
			pubReq.Attributes["replay_notes"] = req.Notes
		}

		if err := s.delivery.Publish(ctx, pubReq); err != nil {
			return success, err
		}

		if err := s.repo.UpdateStatus(ctx, messageID, map[string]interface{}{
			"status":          "replayed",
			"replayed_at":     s.clock(),
			"replay_operator": req.OperatorID,
		}); err != nil {
			return success, err
		}

		success++
		s.metrics.ObserveDLQChange(ctx, -1)

		if s.audit != nil {
			meta := make(map[string]string, len(pubReq.Attributes)+2)
			for k, v := range pubReq.Attributes {
				meta[k] = v
			}
			tenantKey := strings.TrimSpace(envelope.TenantKey)
			if tenantKey != "" {
				meta["tenant_uuid"] = tenantKey
			}
			_ = s.audit.Write(ctx, eventaudit.Record{
				ID:           msg.EventID,
				TenantID:     envelope.TenantKey,
				Topic:        topic.FullTopic,
				PrincipalID:  req.OperatorID,
				Action:       "dlq.replay",
				Status:       "SUCCESS",
				LatencyMs:    0,
				TraceID:      envelope.TraceID,
				Metadata:     meta,
				HappenedAt:   s.clock(),
				ErrorMessage: "",
			})
		}
	}
	return success, nil
}

func (s *serviceImpl) Purge(ctx context.Context, tenantID string, topicID string) (int, error) {
	if s.repo == nil {
		return 0, fmt.Errorf("dlq repository not configured")
	}
	tenantKey, err := resolveTenantKey(tenantID)
	if err != nil {
		return 0, err
	}
	var topicUUID uuid.UUID
	if strings.TrimSpace(topicID) != "" {
		topicUUID, err = uuid.Parse(topicID)
		if err != nil {
			return 0, fmt.Errorf("invalid topic id: %w", err)
		}
	}
	rows, err := s.repo.PurgeByTopic(ctx, tenantKey, topicUUID)
	if err == nil && rows > 0 {
		s.metrics.ObserveDLQChange(ctx, -int64(rows))
	}
	return int(rows), err
}

func mergeHeaders(values ...datatypes.JSON) map[string]string {
	result := map[string]string{}
	for _, val := range values {
		if len(val) == 0 {
			continue
		}
		tmp := map[string]string{}
		if err := json.Unmarshal(val, &tmp); err != nil {
			continue
		}
		for k, v := range tmp {
			result[k] = v
		}
	}
	return result
}

func copyJSON(data datatypes.JSON) []byte {
	if len(data) == 0 {
		return []byte("{}")
	}
	buf := make([]byte, len(data))
	copy(buf, data)
	return buf
}

func resolveTenantKey(value string) (string, error) {
	if key := strings.TrimSpace(value); key != "" {
		return key, nil
	}
	return "", fmt.Errorf("tenant_uuid is required")
}
