package acl

import (
	"context"
	"fmt"
	"strings"
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	eventfabricrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/event_fabric"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type PrincipalAction string

const (
	PrincipalActionPublish   PrincipalAction = "publish"
	PrincipalActionSubscribe PrincipalAction = "subscribe"
	PrincipalActionReplay    PrincipalAction = "replay"
)

type Binding struct {
	ID            string          `json:"id"`
	TenantUUID    string          `json:"tenant_uuid"`
	TenantKey     string          `json:"tenant_key"`
	TopicUUID     string          `json:"topic_uuid"`
	PrincipalType string          `json:"principal_type"`
	PrincipalID   string          `json:"principal_id"`
	Action        PrincipalAction `json:"action"`
	ExpiresAt     *time.Time      `json:"expires_at,omitempty"`
	GrantedBy     string          `json:"granted_by,omitempty"`
	Justification string          `json:"justification,omitempty"`
	AuditRef      string          `json:"audit_ref,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

type GrantRequest struct {
	TenantUUID    string
	TopicUUID     string
	PrincipalType string
	PrincipalID   string
	Actions       []PrincipalAction
	ExpiresAt     *time.Time
	Justification string
	AuditRef      string
	OperatorID    string
}

type RevokeRequest struct {
	TenantUUID  string
	TopicUUID   string
	PrincipalID string
	Actions     []PrincipalAction
	OperatorID  string
}

type ListRequest struct {
	TenantUUID string
	TopicUUID  string
}

type AclStore interface {
	UpsertBindings(ctx context.Context, bindings []*model.AclBinding) ([]*model.AclBinding, error)
	RemoveBindings(ctx context.Context, tenantKey string, topic uuid.UUID, principalID string, actions []string) (int64, error)
	ListByTopic(ctx context.Context, tenantKey string, topic uuid.UUID) ([]*model.AclBinding, error)
	HasPermission(ctx context.Context, tenantKey string, topic uuid.UUID, principalID string, action string, now time.Time) (bool, error)
}

type TopicLookup interface {
	FindByUUID(ctx context.Context, id uuid.UUID) (*model.TopicDefinition, error)
}

type Options struct {
	DB         *gorm.DB
	Store      AclStore
	TopicStore TopicLookup
	Clock      func() time.Time
}

type ACLService struct {
	store  AclStore
	topics TopicLookup
	clock  func() time.Time
}

func NewACLService(opts Options) *ACLService {
	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}

	store := opts.Store
	topicStore := opts.TopicStore
	if store == nil && opts.DB != nil {
		store = eventfabricrepo.NewAclRepository(opts.DB)
	}
	if topicStore == nil && opts.DB != nil {
		topicStore = eventfabricrepo.NewTopicRepository(opts.DB)
	}

	return &ACLService{
		store:  store,
		topics: topicStore,
		clock:  clock,
	}
}

func (s *ACLService) Grant(ctx context.Context, req GrantRequest) ([]*Binding, error) {
	if s.store == nil || s.topics == nil {
		return nil, fmt.Errorf("acl service not configured")
	}
	tenantKey, err := resolveTenantKey(req.TenantUUID)
	if err != nil {
		return nil, err
	}
	topicUUID, err := uuid.Parse(strings.TrimSpace(req.TopicUUID))
	if err != nil {
		return nil, fmt.Errorf("invalid topic id: %w", err)
	}
	topic, err := s.topics.FindByUUID(ctx, topicUUID)
	if err != nil {
		return nil, err
	}
	if topic == nil {
		return nil, fmt.Errorf("topic %s not found", req.TopicUUID)
	}
	if !strings.EqualFold(topic.TenantKey, tenantKey) {
		return nil, fmt.Errorf("tenant mismatch with topic")
	}

	principalType := strings.ToLower(strings.TrimSpace(req.PrincipalType))
	if principalType == "" {
		return nil, fmt.Errorf("principal_type is required")
	}
	principalID := strings.TrimSpace(req.PrincipalID)
	if principalID == "" {
		return nil, fmt.Errorf("principal_id is required")
	}
	if len(req.Actions) == 0 {
		return nil, fmt.Errorf("no actions provided")
	}

	now := s.clock().UTC()
	modelBindings := make([]*model.AclBinding, 0, len(req.Actions))
	for _, action := range req.Actions {
		actionStr := strings.ToLower(strings.TrimSpace(string(action)))
		if actionStr == "" {
			return nil, fmt.Errorf("action cannot be empty")
		}
		modelBindings = append(modelBindings, &model.AclBinding{
			TenantKey:     topic.TenantKey,
			TopicUUID:     topic.UUID,
			PrincipalType: principalType,
			PrincipalID:   principalID,
			Action:        actionStr,
			ExpiresAt:     req.ExpiresAt,
			GrantedBy:     req.OperatorID,
			Justification: req.Justification,
			AuditRef:      req.AuditRef,
			Status:        1,
			PowerUUIDModel: coremodel.PowerUUIDModel{
				CreatedAt: now,
				UpdatedAt: now,
			},
		})
	}

	records, err := s.store.UpsertBindings(ctx, modelBindings)
	if err != nil {
		return nil, err
	}
	return convertBindings(records), nil
}

func (s *ACLService) Revoke(ctx context.Context, req RevokeRequest) error {
	if s.store == nil {
		return fmt.Errorf("acl service not configured")
	}
	tenantKey, err := resolveTenantKey(req.TenantUUID)
	if err != nil {
		return err
	}
	topicUUID, err := uuid.Parse(strings.TrimSpace(req.TopicUUID))
	if err != nil {
		return fmt.Errorf("invalid topic id: %w", err)
	}
	principalID := strings.TrimSpace(req.PrincipalID)
	if principalID == "" {
		return fmt.Errorf("principal_id is required")
	}

	var actions []string
	for _, action := range req.Actions {
		if token := strings.ToLower(strings.TrimSpace(string(action))); token != "" {
			actions = append(actions, token)
		}
	}
	_, err = s.store.RemoveBindings(ctx, tenantKey, topicUUID, principalID, actions)
	return err
}

func (s *ACLService) ListBindings(ctx context.Context, req ListRequest) ([]*Binding, error) {
	if s.store == nil {
		return nil, fmt.Errorf("acl service not configured")
	}
	tenantKey, err := resolveTenantKey(req.TenantUUID)
	if err != nil {
		return nil, err
	}
	topicUUID, err := uuid.Parse(strings.TrimSpace(req.TopicUUID))
	if err != nil {
		return nil, fmt.Errorf("invalid topic id: %w", err)
	}
	rows, err := s.store.ListByTopic(ctx, tenantKey, topicUUID)
	if err != nil {
		return nil, err
	}
	return convertBindings(rows), nil
}

func (s *ACLService) Can(ctx context.Context, tenantKey string, topicUUID uuid.UUID, principalID string, action PrincipalAction) (bool, error) {
	if s.store == nil {
		return false, fmt.Errorf("acl service not configured")
	}
	return s.store.HasPermission(ctx, strings.TrimSpace(tenantKey), topicUUID, strings.TrimSpace(principalID), strings.ToLower(string(action)), s.clock())
}

// HasPermission 满足其他服务对 ACL 查询的接口约束。
func (s *ACLService) HasPermission(ctx context.Context, tenantKey string, topic uuid.UUID, principalID string, action string, now time.Time) (bool, error) {
	if s.store == nil {
		return false, fmt.Errorf("acl service not configured")
	}
	return s.store.HasPermission(ctx,
		strings.TrimSpace(tenantKey),
		topic,
		strings.TrimSpace(principalID),
		strings.ToLower(strings.TrimSpace(action)),
		now)
}

// ListByTopic 返回原始仓储记录，便于内部服务复用。
func (s *ACLService) ListByTopic(ctx context.Context, tenantKey string, topic uuid.UUID) ([]*model.AclBinding, error) {
	if s.store == nil {
		return nil, fmt.Errorf("acl service not configured")
	}
	return s.store.ListByTopic(ctx, strings.TrimSpace(tenantKey), topic)
}

func convertBindings(records []*model.AclBinding) []*Binding {
	result := make([]*Binding, 0, len(records))
	for _, rec := range records {
		if rec == nil {
			continue
		}
		result = append(result, &Binding{
			ID:            rec.UUID.String(),
			TenantUUID:    strings.TrimSpace(rec.TenantKey),
			TenantKey:     rec.TenantKey,
			TopicUUID:     rec.TopicUUID.String(),
			PrincipalType: rec.PrincipalType,
			PrincipalID:   rec.PrincipalID,
			Action:        PrincipalAction(rec.Action),
			ExpiresAt:     rec.ExpiresAt,
			GrantedBy:     rec.GrantedBy,
			Justification: rec.Justification,
			AuditRef:      rec.AuditRef,
			CreatedAt:     rec.CreatedAt,
			UpdatedAt:     rec.UpdatedAt,
		})
	}
	return result
}

func resolveTenantKey(value string) (string, error) {
	if key := strings.TrimSpace(value); key != "" {
		return key, nil
	}
	return "", fmt.Errorf("tenant_uuid is required")
}
