package directory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	aclservice "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/acl"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	eventfabricrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/event_fabric"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/google/uuid"
	"gorm.io/datatypes"
)

const lifecycleChangedEvent = "event_fabric.topic.lifecycle.changed"

// Topic DTO 用于对外返回主题信息。
type Topic struct {
	ID              string               `json:"id"`
	ScopeType       string               `json:"scope_type"`
	ScopeID         string               `json:"scope_id"`
	TenantUUID      string               `json:"tenant_uuid"`
	TenantKey       string               `json:"tenant_key"`
	Namespace       string               `json:"namespace"`
	Name            string               `json:"name"`
	FullTopic       string               `json:"full_topic"`
	PayloadFormat   string               `json:"payload_format"`
	MaxRetry        int32                `json:"max_retry"`
	AckTimeoutSec   int32                `json:"ack_timeout_sec"`
	VersioningMode  string               `json:"versioning_mode"`
	RetentionPolicy string               `json:"retention_policy"`
	Lifecycle       model.TopicLifecycle `json:"lifecycle"`
	CreatedAt       time.Time            `json:"created_at"`
	UpdatedAt       time.Time            `json:"updated_at"`
	DeprecatedAt    *time.Time           `json:"deprecated_at,omitempty"`
}

// CreateTopicInput 主题创建入参。
type CreateTopicInput struct {
	TenantUUID      string                 `json:"tenant_uuid,omitempty"`
	Namespace       string                 `json:"namespace"`
	Name            string                 `json:"name"`
	PayloadFormat   string                 `json:"payload_format"`
	MaxRetry        int32                  `json:"max_retry"`
	AckTimeoutSec   int32                  `json:"ack_timeout_sec"`
	VersioningMode  string                 `json:"versioning_mode"`
	RetentionPolicy string                 `json:"retention_policy"`
	Metadata        map[string]interface{} `json:"metadata"`
	CreatedBy       string                 `json:"created_by"`
}

// UpsertTopicInput 主题声明式同步入参。
type UpsertTopicInput = CreateTopicInput

// UpdateLifecycleInput 主题生命周期变更入参。
type UpdateLifecycleInput struct {
	TopicID      string
	TargetState  model.TopicLifecycle
	ChangeReason string
	DeprecatedAt *time.Time
}

// DirectoryService 提供主题目录能力。
type DirectoryService struct {
	store             TopicStore
	acl               ACLGranter
	eventBus          event_bus.EventBus
	clock             Clock
	actorResolver     ActorResolver
	defaultMaxRetry   int
	defaultAckTimeout time.Duration
}

// NewDirectoryService 构造目录服务。
func NewDirectoryService(opts Options) *DirectoryService {
	store := opts.Store
	if store == nil && opts.DB != nil {
		store = eventfabricrepo.NewTopicRepository(opts.DB)
	}

	svc := &DirectoryService{
		store:             store,
		acl:               opts.ACL,
		eventBus:          opts.EventBus,
		clock:             opts.Clock,
		actorResolver:     opts.ActorResolver,
		defaultMaxRetry:   opts.DefaultMaxRetry,
		defaultAckTimeout: opts.DefaultAckTimeout,
	}
	if svc.clock == nil {
		svc.clock = time.Now
	}
	if svc.defaultMaxRetry <= 0 {
		svc.defaultMaxRetry = 5
	}
	if svc.defaultAckTimeout <= 0 {
		svc.defaultAckTimeout = 30 * time.Second
	}
	return svc
}

// FindTopicByFullName 根据租户+命名空间+名称获取主题实体。
func (s *DirectoryService) FindTopicByFullName(ctx context.Context, tenantKey, namespace, name string) (*model.TopicDefinition, error) {
	if s.store == nil {
		return nil, errors.New("topic repository not configured")
	}
	return s.store.FindByComposite(ctx, strings.TrimSpace(tenantKey), normalizeSegment(namespace), normalizeSegment(name))
}

// CreateTopic 创建新主题。
func (s *DirectoryService) CreateTopic(ctx context.Context, input CreateTopicInput) (*Topic, error) {
	if s.store == nil {
		return nil, errors.New("topic repository not configured")
	}
	record, err := s.prepareTopicDefinition(ctx, input)
	if err != nil {
		return nil, err
	}

	existing, err := s.store.FindByComposite(ctx, record.TenantKey, record.Namespace, record.Name)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, fmt.Errorf("topic %s.%s.%s already exists", record.TenantKey, record.Namespace, record.Name)
	}

	result, err := s.store.Create(ctx, record)
	if err != nil {
		return nil, err
	}
	if err := s.grantTopicDefaultACL(ctx, result, record.CreatedBy); err != nil {
		return nil, err
	}
	return convertTopic(result), nil
}

func (s *DirectoryService) prepareTopicDefinition(ctx context.Context, input CreateTopicInput) (*model.TopicDefinition, error) {
	if err := validateCreateInput(input); err != nil {
		return nil, err
	}
	tenantKey, err := resolveTenantKey(input.TenantUUID)
	if err != nil {
		return nil, err
	}
	namespace := normalizeSegment(input.Namespace)
	name := normalizeSegment(input.Name)

	payloadFormat := strings.TrimSpace(input.PayloadFormat)
	if payloadFormat == "" {
		payloadFormat = "json"
	}

	maxRetry := int(input.MaxRetry)
	if maxRetry <= 0 {
		maxRetry = s.defaultMaxRetry
	}

	ackTimeout := int(input.AckTimeoutSec)
	if ackTimeout <= 0 {
		ackTimeout = int(s.defaultAckTimeout / time.Second)
	}

	versioningMode := strings.TrimSpace(input.VersioningMode)
	if versioningMode == "" {
		versioningMode = "strict"
	}

	retentionPolicy, err := normalizeJSON(input.RetentionPolicy)
	if err != nil {
		return nil, fmt.Errorf("invalid retention policy: %w", err)
	}

	metadataJSON := []byte("{}")
	if len(input.Metadata) > 0 {
		metadataJSON, err = json.Marshal(input.Metadata)
		if err != nil {
			return nil, fmt.Errorf("invalid metadata: %w", err)
		}
	}

	createdBy := input.CreatedBy
	if createdBy == "" && s.actorResolver != nil {
		createdBy = s.actorResolver(ctx)
	}

	return &model.TopicDefinition{
		ScopeType:       model.TopicScopeTenant,
		ScopeID:         tenantKey,
		TenantKey:       tenantKey,
		Namespace:       namespace,
		Name:            name,
		Lifecycle:       model.TopicLifecycleActive,
		PayloadFormat:   payloadFormat,
		RetentionPolicy: datatypes.JSON(retentionPolicy),
		VersioningMode:  versioningMode,
		MaxRetry:        maxRetry,
		AckTimeoutSec:   ackTimeout,
		Metadata:        datatypes.JSON(metadataJSON),
		CreatedBy:       createdBy,
		Status:          1,
	}, nil
}

// UpsertTopic 按 tenant + namespace + name 同步主题声明。
func (s *DirectoryService) UpsertTopic(ctx context.Context, input UpsertTopicInput) (*Topic, bool, error) {
	if s.store == nil {
		return nil, false, errors.New("topic repository not configured")
	}
	prepared, err := s.prepareTopicDefinition(ctx, CreateTopicInput(input))
	if err != nil {
		return nil, false, err
	}

	existing, err := s.store.FindByComposite(ctx, prepared.TenantKey, prepared.Namespace, prepared.Name)
	if err != nil {
		return nil, false, err
	}
	if existing == nil {
		result, err := s.store.Create(ctx, prepared)
		if err != nil {
			return nil, false, err
		}
		if err := s.grantTopicDefaultACL(ctx, result, prepared.CreatedBy); err != nil {
			return nil, false, err
		}
		return convertTopic(result), true, nil
	}

	existing.PayloadFormat = prepared.PayloadFormat
	existing.RetentionPolicy = prepared.RetentionPolicy
	existing.VersioningMode = prepared.VersioningMode
	existing.MaxRetry = prepared.MaxRetry
	existing.AckTimeoutSec = prepared.AckTimeoutSec
	existing.Metadata = prepared.Metadata
	existing.CreatedBy = prepared.CreatedBy
	existing.Status = 1
	if existing.Lifecycle == model.TopicLifecycleRetired || existing.Lifecycle == model.TopicLifecycleDeprecated {
		existing.Lifecycle = model.TopicLifecycleActive
		existing.DeprecatedAt = nil
	}
	updated, err := s.store.Update(ctx, existing)
	if err != nil {
		return nil, false, err
	}
	return convertTopic(updated), false, nil
}

// UpdateLifecycle 修改主题生命周期状态。
func (s *DirectoryService) UpdateLifecycle(ctx context.Context, input UpdateLifecycleInput) (*Topic, error) {
	if s.store == nil {
		return nil, errors.New("topic repository not configured")
	}
	if strings.TrimSpace(input.TopicID) == "" {
		return nil, errors.New("topic id is required")
	}
	if strings.TrimSpace(string(input.TargetState)) == "" {
		return nil, errors.New("target lifecycle state is required")
	}

	id, err := uuid.Parse(input.TopicID)
	if err != nil {
		return nil, fmt.Errorf("invalid topic id: %w", err)
	}

	record, err := s.store.FindByUUID(ctx, id)
	if err != nil {
		return nil, err
	}
	if record == nil {
		return nil, fmt.Errorf("topic %s not found", input.TopicID)
	}

	target := model.TopicLifecycle(strings.TrimSpace(string(input.TargetState)))
	switch target {
	case model.TopicLifecycleActive, model.TopicLifecycleDeprecated, model.TopicLifecycleRetired:
	default:
		return nil, fmt.Errorf("unsupported lifecycle state %s", target)
	}
	if record.Lifecycle == target {
		return convertTopic(record), nil
	}

	now := s.clock()
	record.Lifecycle = target
	if target == model.TopicLifecycleDeprecated || target == model.TopicLifecycleRetired {
		if input.DeprecatedAt != nil {
			record.DeprecatedAt = input.DeprecatedAt
		} else {
			record.DeprecatedAt = &now
		}
	} else {
		record.DeprecatedAt = nil
	}

	updated, err := s.store.Update(ctx, record)
	if err != nil {
		return nil, err
	}
	s.publishLifecycleEvent(ctx, updated, input.ChangeReason)
	return convertTopic(updated), nil
}

// ListTopics 查询主题列表。
func (s *DirectoryService) ListTopics(ctx context.Context, query eventfabricrepo.QueryContext) ([]*Topic, int64, error) {
	if s.store == nil {
		return nil, 0, errors.New("topic repository not configured")
	}
	records, total, err := s.store.List(ctx, query)
	if err != nil {
		return nil, 0, err
	}
	topics := make([]*Topic, 0, len(records))
	for _, rec := range records {
		topics = append(topics, convertTopic(rec))
	}
	return topics, total, nil
}

func (s *DirectoryService) grantTopicDefaultACL(ctx context.Context, topic *model.TopicDefinition, operator string) error {
	if s.acl == nil || topic == nil {
		return nil
	}
	_, err := s.acl.Grant(ctx, aclservice.GrantRequest{
		TenantUUID:    topic.TenantKey,
		TopicUUID:     topic.UUID.String(),
		PrincipalType: "role",
		PrincipalID:   "role:role_admin",
		Actions: []aclservice.PrincipalAction{
			aclservice.PrincipalActionPublish,
			aclservice.PrincipalActionSubscribe,
			aclservice.PrincipalActionReplay,
		},
		Justification: "topic default admin access",
		OperatorID:    strings.TrimSpace(operator),
	})
	if err != nil {
		return fmt.Errorf("grant default acl failed: %w", err)
	}
	return nil
}

func (s *DirectoryService) publishLifecycleEvent(ctx context.Context, topic *model.TopicDefinition, reason string) {
	if s.eventBus == nil || topic == nil {
		return
	}
	payload := map[string]interface{}{
		"topic_id":    topic.UUID.String(),
		"scope_type":  topic.ScopeType,
		"scope_id":    topic.ScopeID,
		"tenant_key":  topic.TenantKey,
		"tenant_uuid": strings.TrimSpace(topic.TenantKey),
		"namespace":   topic.Namespace,
		"name":        topic.Name,
		"lifecycle":   topic.Lifecycle,
		"change_time": s.clock().UTC(),
	}
	if reason != "" {
		payload["change_reason"] = reason
	}
	s.eventBus.Publish(lifecycleChangedEvent, payload, ctx)
}

func convertTopic(record *model.TopicDefinition) *Topic {
	if record == nil {
		return nil
	}

	tenantDisplay := strings.TrimSpace(record.TenantKey)
	if tenantDisplay == "" {
		tenantDisplay = "global"
	}
	scopeType := strings.TrimSpace(string(record.ScopeType))
	if scopeType == "" {
		scopeType = string(model.TopicScopeTenant)
	}
	scopeID := strings.TrimSpace(record.ScopeID)
	if scopeID == "" {
		scopeID = tenantDisplay
	}

	retention := string(record.RetentionPolicy)
	if strings.TrimSpace(retention) == "" {
		retention = "{}"
	}

	return &Topic{
		ID:              record.UUID.String(),
		ScopeType:       scopeType,
		ScopeID:         scopeID,
		TenantUUID:      tenantDisplay,
		TenantKey:       record.TenantKey,
		Namespace:       record.Namespace,
		Name:            record.Name,
		FullTopic:       record.FullTopic,
		PayloadFormat:   record.PayloadFormat,
		MaxRetry:        int32(record.MaxRetry),
		AckTimeoutSec:   int32(record.AckTimeoutSec),
		VersioningMode:  record.VersioningMode,
		RetentionPolicy: retention,
		Lifecycle:       record.Lifecycle,
		CreatedAt:       record.CreatedAt,
		UpdatedAt:       record.UpdatedAt,
		DeprecatedAt:    record.DeprecatedAt,
	}
}

func resolveTenantKey(value string) (string, error) {
	if key := strings.TrimSpace(value); key != "" {
		return key, nil
	}
	return "", fmt.Errorf("tenant_uuid is required")
}

func normalizeSegment(val string) string {
	return strings.TrimSpace(strings.ToLower(val))
}

func normalizeJSON(raw string) ([]byte, error) {
	if strings.TrimSpace(raw) == "" {
		return []byte("{}"), nil
	}
	var tmp interface{}
	if err := json.Unmarshal([]byte(raw), &tmp); err != nil {
		return nil, err
	}
	return []byte(raw), nil
}
