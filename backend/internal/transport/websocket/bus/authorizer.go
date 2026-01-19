package bus

import (
	"context"
	"errors"
	"strings"

	iamsvc "github.com/ArtisanCloud/PowerX/internal/service/iam"
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
		return ErrPermissionDenied
	}
	topic = strings.TrimSpace(topic)
	if topic == "" {
		return ErrTopicNotAllowed
	}
	if client.TenantUUID == "" {
		return ErrTenantRequired
	}
	if !client.IsRoot && client.MemberID == 0 {
		return ErrMemberRequired
	}
	switch topic {
	case TopicKnowledgeIngestionJob:
		return a.enforceKnowledgeRead(ctx, client)
	case TopicSystemNotification:
		return nil
	default:
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
