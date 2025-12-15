package replay

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/delivery"
	eventmetrics "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/metrics"
	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/shared"
	eventfabricmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	eventfabricrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/event_fabric"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Repository 定义回放任务持久化行为。
type Repository interface {
	Create(ctx context.Context, req *eventfabricmodel.ReplayRequest) (*eventfabricmodel.ReplayRequest, error)
	FindByUUID(ctx context.Context, id uuid.UUID) (*eventfabricmodel.ReplayRequest, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, updates map[string]interface{}) error
}

// TopicLookup 提供 Topic 查询能力。
type TopicLookup interface {
	FindByComposite(ctx context.Context, tenantKey, namespace, name string) (*eventfabricmodel.TopicDefinition, error)
	FindByUUID(ctx context.Context, id uuid.UUID) (*eventfabricmodel.TopicDefinition, error)
}

// EnvelopeFinder 查询可回放事件。
type EnvelopeFinder interface {
	ListForReplay(ctx context.Context, tenantKey string, topic uuid.UUID, filter eventfabricrepo.ReplayQuery) ([]*eventfabricmodel.EventEnvelope, error)
}

// Options 构建 Service 所需依赖。
type Options struct {
	DB        *gorm.DB
	Repo      Repository
	Envelopes EnvelopeFinder
	Topics    TopicLookup
	Delivery  delivery.Service
	Clock     func() time.Time
	Metrics   eventmetrics.Recorder
}

// Service 提供事件回放能力。
type Service struct {
	repo      Repository
	envelopes EnvelopeFinder
	topics    TopicLookup
	delivery  delivery.Service
	clock     func() time.Time
	metrics   eventmetrics.Recorder
}

// CreateTaskInput 创建回放任务输入。
type CreateTaskInput struct {
	TenantKey   string
	Topic       string
	TraceID     string
	WindowStart time.Time
	WindowEnd   time.Time
	Reason      string
	Operator    string
	Shadow      bool
}

// Task 描述回放任务状态。
type Task struct {
	ID            string
	TenantKey     string
	Topic         string
	TraceID       string
	Status        string
	Shadow        bool
	RequestedBy   string
	SubmittedAt   time.Time
	CompletedAt   *time.Time
	FailureReason string
	ResultCount   int
}

// NewService 构建回放服务。
func NewService(opts Options) *Service {
	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}
	repo := opts.Repo
	if repo == nil && opts.DB != nil {
		repo = eventfabricrepo.NewReplayRepository(opts.DB)
	}
	envelopes := opts.Envelopes
	if envelopes == nil && opts.DB != nil {
		envelopes = eventfabricrepo.NewEnvelopeRepository(opts.DB)
	}
	topics := opts.Topics
	if topics == nil && opts.DB != nil {
		topics = eventfabricrepo.NewTopicRepository(opts.DB)
	}
	return &Service{
		repo:      repo,
		envelopes: envelopes,
		topics:    topics,
		delivery:  opts.Delivery,
		clock:     clock,
		metrics: func() eventmetrics.Recorder {
			if opts.Metrics != nil {
				return opts.Metrics
			}
			return eventmetrics.NewNoop()
		}(),
	}
}

// CreateTask 创建并执行回放任务。
func (s *Service) CreateTask(ctx context.Context, input CreateTaskInput) (*Task, error) {
	if s.repo == nil || s.envelopes == nil || s.delivery == nil || s.topics == nil {
		return nil, fmt.Errorf("replay service not configured")
	}
	tenantKey := strings.TrimSpace(input.TenantKey)
	if tenantKey == "" {
		return nil, fmt.Errorf("tenant_key is required")
	}
	topicTenant, namespace, name, err := splitFullTopic(input.Topic)
	if err != nil {
		return nil, err
	}
	if topicTenant != "" && !strings.EqualFold(topicTenant, tenantKey) {
		return nil, fmt.Errorf("topic tenant mismatch: %s", input.Topic)
	}
	topicDef, err := s.topics.FindByComposite(ctx, tenantKey, namespace, name)
	if err != nil {
		return nil, err
	}
	if topicDef == nil {
		return nil, fmt.Errorf("topic %s not found", input.Topic)
	}
	if !input.WindowEnd.IsZero() && input.WindowStart.After(input.WindowEnd) {
		return nil, fmt.Errorf("time_range_start must be <= time_range_end")
	}

	record := &eventfabricmodel.ReplayRequest{
		TenantKey:      tenantKey,
		TopicUUID:      topicDef.UUID,
		TraceID:        strings.TrimSpace(input.TraceID),
		Shadow:         input.Shadow,
		VersionMode:    strings.TrimSpace(topicDef.VersioningMode),
		TimeRangeStart: input.WindowStart,
		TimeRangeEnd:   input.WindowEnd,
		Status:         eventfabricmodel.ReplayStatusPending,
		IssuedBy:       strings.TrimSpace(input.Operator),
		Reason:         strings.TrimSpace(input.Reason),
		SubmittedAt:    s.clock().UTC(),
	}

	record, err = s.repo.Create(ctx, record)
	if err != nil {
		return nil, err
	}

	if err := s.repo.UpdateStatus(ctx, record.UUID, map[string]interface{}{
		"status": eventfabricmodel.ReplayStatusRunning,
	}); err != nil {
		return nil, err
	}

	started := s.clock().UTC()
	count, execErr := s.executeReplay(ctx, record, topicDef.FullTopic)
	s.metrics.ObserveReplay(ctx, s.clock().UTC().Sub(started), execErr)
	updates := map[string]interface{}{
		"result_count": count,
	}
	if execErr != nil {
		updates["status"] = eventfabricmodel.ReplayStatusFailed
		updates["failure_reason"] = execErr.Error()
	} else {
		updates["status"] = eventfabricmodel.ReplayStatusCompleted
		completed := s.clock().UTC()
		updates["completed_at"] = &completed
	}
	if err := s.repo.UpdateStatus(ctx, record.UUID, updates); err != nil {
		return nil, err
	}
	latest, err := s.repo.FindByUUID(ctx, record.UUID)
	if err != nil {
		return nil, err
	}
	if latest == nil {
		return nil, fmt.Errorf("replay task not found after update")
	}
	if execErr != nil {
		return nil, execErr
	}
	task, convErr := s.toTask(ctx, latest, topicDef.FullTopic)
	if convErr != nil {
		return nil, convErr
	}
	return task, nil
}

// GetTask 根据 ID 查询任务状态。
func (s *Service) GetTask(ctx context.Context, id string) (*Task, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("replay service not configured")
	}
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("invalid task id: %w", err)
	}
	record, err := s.repo.FindByUUID(ctx, uid)
	if err != nil {
		return nil, err
	}
	if record == nil {
		return nil, nil
	}
	return s.toTask(ctx, record, "")
}

// CancelTask 标记任务为已取消。
func (s *Service) CancelTask(ctx context.Context, id string, operator string) error {
	if s.repo == nil {
		return fmt.Errorf("replay service not configured")
	}
	uid, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid task id: %w", err)
	}
	updates := map[string]interface{}{
		"status": eventfabricmodel.ReplayStatusCancelled,
	}
	if op := strings.TrimSpace(operator); op != "" {
		updates["failure_reason"] = fmt.Sprintf("cancelled by %s", op)
	} else {
		updates["failure_reason"] = "cancelled"
	}
	cancelled := s.clock().UTC()
	updates["cancelled_at"] = &cancelled
	return s.repo.UpdateStatus(ctx, uid, updates)
}

func (s *Service) executeReplay(ctx context.Context, request *eventfabricmodel.ReplayRequest, fullTopic string) (int, error) {
	filter := eventfabricrepo.ReplayQuery{
		Statuses:  []string{shared.DeliveryStatusSucceeded, shared.DeliveryStatusDelivering, shared.DeliveryStatusPending},
		StartTime: request.TimeRangeStart,
		EndTime:   request.TimeRangeEnd,
		TraceID:   request.TraceID,
	}
	envelopes, err := s.envelopes.ListForReplay(ctx, request.TenantKey, request.TopicUUID, filter)
	if err != nil {
		return 0, err
	}
	count := 0
	for idx, envelope := range envelopes {
		attributes := mapFromJSON(envelope.Headers)
		if attributes == nil {
			attributes = map[string]string{}
		}
		attributes["replay"] = "true"
		attributes["replay_request_id"] = request.UUID.String()
		attributes["shadow"] = fmt.Sprintf("%t", request.Shadow)
		if strings.TrimSpace(request.IssuedBy) != "" {
			attributes["principal_id"] = strings.TrimSpace(request.IssuedBy)
		}

		payload := make([]byte, len(envelope.Payload))
		copy(payload, envelope.Payload)

		newEventID := fmt.Sprintf("%s-replay-%d", envelope.EventID, idx)
		if err := s.delivery.Publish(ctx, delivery.PublishRequest{
			TenantUUID:     request.TenantKey,
			Topic:          fullTopic,
			EventID:        newEventID,
			TraceID:        envelope.TraceID,
			Version:        envelope.Version,
			Payload:        payload,
			PayloadFormat:  envelope.PayloadFormat,
			IdempotencyKey: fmt.Sprintf("replay:%s:%d", request.UUID.String(), idx),
			Attributes:     attributes,
		}); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func (s *Service) toTask(ctx context.Context, record *eventfabricmodel.ReplayRequest, presetTopic string) (*Task, error) {
	if record == nil {
		return nil, nil
	}
	topicName := presetTopic
	if topicName == "" && s.topics != nil {
		if topic, err := s.topics.FindByUUID(ctx, record.TopicUUID); err == nil && topic != nil {
			topicName = topic.FullTopic
		}
	}
	return &Task{
		ID:            record.UUID.String(),
		TenantKey:     record.TenantKey,
		Topic:         topicName,
		TraceID:       record.TraceID,
		Status:        record.Status,
		Shadow:        record.Shadow,
		RequestedBy:   record.IssuedBy,
		SubmittedAt:   record.SubmittedAt,
		CompletedAt:   record.CompletedAt,
		FailureReason: record.FailureReason,
		ResultCount:   record.ResultCount,
	}, nil
}

func splitFullTopic(topic string) (tenant string, namespace string, name string, err error) {
	trimmed := strings.TrimSpace(topic)
	if trimmed == "" {
		return "", "", "", fmt.Errorf("topic is required")
	}
	parts := strings.Split(trimmed, ".")
	if len(parts) < 3 {
		return "", "", "", fmt.Errorf("invalid topic format: %s", topic)
	}
	tenant = parts[0]
	namespace = strings.Join(parts[1:len(parts)-1], ".")
	name = parts[len(parts)-1]
	return tenant, namespace, name, nil
}

func mapFromJSON(data []byte) map[string]string {
	if len(data) == 0 {
		return nil
	}
	var result map[string]string
	if err := json.Unmarshal(data, &result); err != nil {
		return nil
	}
	return result
}
