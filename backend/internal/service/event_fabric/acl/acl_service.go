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
	Cache      ACLResultCache
	Clock      func() time.Time
}

type ACLService struct {
	store  AclStore
	topics TopicLookup
	cache  ACLResultCache
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
		cache:  opts.Cache,
		clock:  clock,
	}
}

func (s *ACLService) cacheKey(tenantKey string, topic uuid.UUID, principalID string, action string) string {
	return BuildACLResultCacheKey(tenantKey, topic, principalID, action)
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
	if !strings.EqualFold(topic.TenantKey, tenantKey) && !isSharedTenantKey(topic.TenantKey) {
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
		cacheKey := s.cacheKey(topic.TenantKey, topic.UUID, principalID, actionStr)
		if s.cache != nil {
			if allowed, hit, err := s.cache.Get(ctx, cacheKey); err == nil && hit && allowed {
				continue
			}
		}
		allowed, err := s.store.HasPermission(ctx, topic.TenantKey, topic.UUID, principalID, actionStr, now)
		if err == nil && allowed {
			if s.cache != nil {
				_ = s.cache.Set(ctx, cacheKey, true)
			}
			continue
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

	if len(modelBindings) == 0 {
		return nil, nil
	}

	records, err := s.store.UpsertBindings(ctx, modelBindings)
	if err != nil {
		return nil, err
	}
	for _, b := range records {
		if s.cache != nil {
			_ = s.cache.Set(ctx, s.cacheKey(b.TenantKey, b.TopicUUID, b.PrincipalID, b.Action), true)
		}
	}
	return convertBindings(records), nil
}

func (s *ACLService) Revoke(ctx context.Context, req RevokeRequest) error {
	if s.store == nil || s.topics == nil {
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
	topic, err := s.topics.FindByUUID(ctx, topicUUID)
	if err != nil {
		return err
	}
	if topic == nil {
		return fmt.Errorf("topic %s not found", req.TopicUUID)
	}
	aclTenantKey := strings.TrimSpace(topic.TenantKey)
	if !strings.EqualFold(aclTenantKey, tenantKey) && !isSharedTenantKey(aclTenantKey) {
		return fmt.Errorf("tenant mismatch with topic")
	}

	var actions []string
	for _, action := range req.Actions {
		if token := strings.ToLower(strings.TrimSpace(string(action))); token != "" {
			actions = append(actions, token)
			if s.cache != nil {
				_ = s.cache.Delete(ctx, s.cacheKey(aclTenantKey, topicUUID, principalID, token))
			}
		}
	}
	_, err = s.store.RemoveBindings(ctx, aclTenantKey, topicUUID, principalID, actions)
	return err
}

func (s *ACLService) ListBindings(ctx context.Context, req ListRequest) ([]*Binding, error) {
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
	aclTenantKey := strings.TrimSpace(topic.TenantKey)
	if !strings.EqualFold(aclTenantKey, tenantKey) && !isSharedTenantKey(aclTenantKey) {
		return nil, fmt.Errorf("tenant mismatch with topic")
	}
	rows, err := s.store.ListByTopic(ctx, aclTenantKey, topicUUID)
	if err != nil {
		return nil, err
	}
	return convertBindings(rows), nil
}

func (s *ACLService) Can(ctx context.Context, tenantKey string, topicUUID uuid.UUID, principalID string, action PrincipalAction) (bool, error) {
	if s.store == nil {
		return false, fmt.Errorf("acl service not configured")
	}
	tenantKey = strings.TrimSpace(tenantKey)
	principalID = strings.TrimSpace(principalID)
	actionKey := strings.ToLower(string(action))
	cacheKey := s.cacheKey(tenantKey, topicUUID, principalID, actionKey)
	if s.cache != nil {
		if allowed, hit, err := s.cache.Get(ctx, cacheKey); err == nil && hit {
			return allowed, nil
		}
	}
	allowed, err := s.store.HasPermission(ctx, tenantKey, topicUUID, principalID, actionKey, s.clock())
	if err != nil {
		return false, err
	}
	if s.cache != nil {
		_ = s.cache.Set(ctx, cacheKey, allowed)
	}
	return allowed, nil
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

func isSharedTenantKey(value string) bool {
	key := strings.ToLower(strings.TrimSpace(value))
	return key == "global" || key == "system"
}
