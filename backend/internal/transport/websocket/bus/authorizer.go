package bus

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	eventfabricmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	eventfabricrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/event_fabric"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var (
	ErrTopicNotAllowed  = errors.New("topic not allowed")
	ErrTenantRequired   = errors.New("tenant required")
	ErrMemberRequired   = errors.New("member required")
	ErrPermissionDenied = errors.New("permission denied")
)

// Authorizer validates subscription permissions per topic.
type Authorizer interface {
	Authorize(ctx context.Context, client *Client, topic string) error
	AuthorizePublish(ctx context.Context, input PublishAuthorizeInput) error
}

type PublishAuthorizeInput struct {
	TenantUUID string
	MemberID   uint64
	UserID     uint64
	IsRoot     bool
	Topic      string
}

type topicLookup interface {
	FindByComposite(ctx context.Context, tenantKey, namespace, name string) (*eventfabricmodel.TopicDefinition, error)
}

type aclChecker interface {
	HasPermission(ctx context.Context, tenantKey string, topic uuid.UUID, principalID string, action string, now time.Time) (bool, error)
}

type DefaultAuthorizerOptions struct {
	DB         *gorm.DB
	TopicStore topicLookup
	ACLStore   aclChecker
	Clock      func() time.Time
}

type DefaultAuthorizer struct {
	topics topicLookup
	acl    aclChecker
	clock  func() time.Time
}

func NewDefaultAuthorizer(db *gorm.DB) *DefaultAuthorizer {
	return NewDefaultAuthorizerWithOptions(DefaultAuthorizerOptions{DB: db})
}

func NewDefaultAuthorizerWithOptions(opts DefaultAuthorizerOptions) *DefaultAuthorizer {
	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}
	topicStore := opts.TopicStore
	if topicStore == nil && opts.DB != nil {
		topicStore = eventfabricrepo.NewTopicRepository(opts.DB)
	}
	aclStore := opts.ACLStore
	if aclStore == nil && opts.DB != nil {
		aclStore = eventfabricrepo.NewAclRepository(opts.DB)
	}
	return &DefaultAuthorizer{topics: topicStore, acl: aclStore, clock: clock}
}

func (a *DefaultAuthorizer) Authorize(ctx context.Context, client *Client, topic string) error {
	if client == nil {
		logger.DebugF(ctx, "[ws-bus] authorize rejected: client=nil")
		return ErrPermissionDenied
	}
	topic = strings.TrimSpace(topic)
	if topic == "" {
		logger.DebugF(ctx, "[ws-bus] authorize rejected: empty topic tenant=%s", strings.TrimSpace(client.TenantUUID))
		return ErrTopicNotAllowed
	}
	if client.TenantUUID == "" {
		logger.DebugF(ctx, "[ws-bus] authorize rejected: tenant required topic=%s", topic)
		return ErrTenantRequired
	}
	if !client.IsRoot && client.MemberID == 0 {
		logger.DebugF(ctx, "[ws-bus] authorize rejected: member required tenant=%s topic=%s", strings.TrimSpace(client.TenantUUID), topic)
		return ErrMemberRequired
	}
	if err := a.authorizeAction(ctx, authorizeInput{
		TenantUUID: client.TenantUUID,
		MemberID:   client.MemberID,
		UserID:     client.UserID,
		IsRoot:     client.IsRoot,
		Topic:      topic,
		Action:     "subscribe",
	}); err != nil {
		logger.DebugF(ctx, "[ws-bus] authorize rejected: tenant=%s topic=%s action=subscribe err=%v", strings.TrimSpace(client.TenantUUID), topic, err)
		return err
	}
	logger.DebugF(ctx, "[ws-bus] authorize allow: tenant=%s topic=%s action=subscribe", strings.TrimSpace(client.TenantUUID), topic)
	return nil
}

func (a *DefaultAuthorizer) AuthorizePublish(ctx context.Context, input PublishAuthorizeInput) error {
	if strings.TrimSpace(input.TenantUUID) == "" {
		return ErrTenantRequired
	}
	if !input.IsRoot && input.MemberID == 0 {
		return ErrMemberRequired
	}
	if err := a.authorizeAction(ctx, authorizeInput{
		TenantUUID: input.TenantUUID,
		MemberID:   input.MemberID,
		UserID:     input.UserID,
		IsRoot:     input.IsRoot,
		Topic:      input.Topic,
		Action:     "publish",
	}); err != nil {
		logger.DebugF(ctx, "[ws-bus] publish authorize rejected: tenant=%s topic=%s err=%v", strings.TrimSpace(input.TenantUUID), strings.TrimSpace(input.Topic), err)
		return err
	}
	return nil
}

type authorizeInput struct {
	TenantUUID string
	MemberID   uint64
	UserID     uint64
	IsRoot     bool
	Topic      string
	Action     string
}

func (a *DefaultAuthorizer) authorizeAction(ctx context.Context, input authorizeInput) error {
	topic := strings.TrimSpace(input.Topic)
	if topic == "" {
		return ErrTopicNotAllowed
	}
	tenantUUID := strings.TrimSpace(input.TenantUUID)
	if tenantUUID == "" {
		return ErrTenantRequired
	}
	topicTenant, namespace, name, err := parseTopicName(topic)
	if err != nil {
		return ErrTopicNotAllowed
	}
	if topicTenant != "" && !strings.EqualFold(topicTenant, tenantUUID) && !isSharedTopicTenant(topicTenant) {
		return ErrTopicNotAllowed
	}
	if a.topics == nil {
		return ErrPermissionDenied
	}
	topicDef, err := a.topics.FindByComposite(ctx, tenantUUID, namespace, name)
	if err != nil {
		return err
	}
	if topicDef == nil {
		return ErrTopicNotAllowed
	}
	if !strings.EqualFold(topicDef.TenantKey, tenantUUID) && !isSharedTopicTenant(topicDef.TenantKey) {
		return ErrTopicNotAllowed
	}
	if strings.EqualFold(topicDef.TenantKey, tenantUUID) {
		if IsDynamicTopicRegistered(tenantUUID, topic) {
			return nil
		}
	}
	if input.IsRoot {
		return nil
	}
	principal := buildACLPrincipal(input.MemberID, input.UserID)
	if principal == "" {
		return ErrMemberRequired
	}
	if a.acl == nil {
		return ErrPermissionDenied
	}
	allowed, err := a.acl.HasPermission(ctx, topicDef.TenantKey, topicDef.UUID, principal, strings.ToLower(strings.TrimSpace(input.Action)), a.clock().UTC())
	if err != nil {
		return err
	}
	if !allowed {
		return ErrPermissionDenied
	}
	return nil
}

func buildACLPrincipal(memberID, userID uint64) string {
	if memberID > 0 {
		return fmt.Sprintf("member:%d", memberID)
	}
	if userID > 0 {
		return fmt.Sprintf("user:%d", userID)
	}
	return ""
}

func parseTopicName(topic string) (tenant string, namespace string, name string, err error) {
	trimmed := strings.TrimSpace(topic)
	if trimmed == "" {
		return "", "", "", fmt.Errorf("topic required")
	}
	parts := strings.Split(trimmed, ".")
	if len(parts) < 2 {
		return "", "", "", fmt.Errorf("invalid topic format")
	}
	first := strings.TrimSpace(parts[0])
	if parsed, parseErr := uuid.Parse(first); parseErr == nil && parsed != uuid.Nil && len(parts) >= 3 {
		tenant = first
		namespace = strings.Join(parts[1:len(parts)-1], ".")
		name = strings.TrimSpace(parts[len(parts)-1])
		return tenant, namespace, name, nil
	}
	namespace = strings.Join(parts[:len(parts)-1], ".")
	name = strings.TrimSpace(parts[len(parts)-1])
	return "", namespace, name, nil
}

func isSharedTopicTenant(tenantKey string) bool {
	key := strings.ToLower(strings.TrimSpace(tenantKey))
	return key == "global" || key == "system"
}
