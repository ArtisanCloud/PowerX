package bus

import (
	"context"
	"errors"
	"strings"

	iamsvc "github.com/ArtisanCloud/PowerX/internal/service/iam"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
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
}

type DefaultAuthorizer struct {
	rbac *iamsvc.RBACService
}

func NewDefaultAuthorizer(db *gorm.DB) *DefaultAuthorizer {
	if db == nil {
		return &DefaultAuthorizer{}
	}
	return &DefaultAuthorizer{rbac: iamsvc.NewRBACService(db)}
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
	switch topic {
	case TopicKnowledgeIngestionJob, TopicKnowledgeCorpusCheck:
		err := a.enforceKnowledgeRead(ctx, client)
		if err != nil {
			logger.DebugF(ctx, "[ws-bus] authorize rejected: knowledge read tenant=%s topic=%s err=%v", strings.TrimSpace(client.TenantUUID), topic, err)
		} else {
			logger.DebugF(ctx, "[ws-bus] authorize allow: knowledge read tenant=%s topic=%s", strings.TrimSpace(client.TenantUUID), topic)
		}
		return err
	case TopicOrgSyncProgress:
		logger.DebugF(ctx, "[ws-bus] authorize allow: whitelist tenant=%s topic=%s", strings.TrimSpace(client.TenantUUID), topic)
		return nil
	case TopicOrgSyncProgressV1:
		logger.DebugF(ctx, "[ws-bus] authorize allow: whitelist tenant=%s topic=%s", strings.TrimSpace(client.TenantUUID), topic)
		return nil
	case TopicSystemNotification:
		logger.DebugF(ctx, "[ws-bus] authorize allow: whitelist tenant=%s topic=%s", strings.TrimSpace(client.TenantUUID), topic)
		return nil
	default:
		if IsDynamicTopicRegistered(client.TenantUUID, topic) {
			logger.DebugF(ctx, "[ws-bus] authorize allow: dynamic tenant=%s topic=%s", strings.TrimSpace(client.TenantUUID), topic)
			return nil
		}
		logger.DebugF(ctx, "[ws-bus] authorize rejected: not allowed tenant=%s topic=%s", strings.TrimSpace(client.TenantUUID), topic)
		return ErrTopicNotAllowed
	}
}

func (a *DefaultAuthorizer) enforceKnowledgeRead(ctx context.Context, client *Client) error {
	if client.IsRoot {
		return nil
	}
	if a.rbac == nil {
		return ErrPermissionDenied
	}
	actor := iamsvc.ActorContext{
		IsRoot:     client.IsRoot,
		TenantUUID: client.TenantUUID,
	}
	ok, err := a.rbac.Enforce(ctx, actor, client.TenantUUID, client.MemberID, "corex", "knowledge_space", "read")
	if err != nil {
		return err
	}
	if !ok {
		return ErrPermissionDenied
	}
	return nil
}
