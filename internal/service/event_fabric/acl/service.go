package acl

import (
	"context"
	"time"
)

// PrincipalAction 表示发布、订阅或回放等操作。
type PrincipalAction string

const (
	PrincipalActionPublish   PrincipalAction = "publish"
	PrincipalActionSubscribe PrincipalAction = "subscribe"
	PrincipalActionReplay    PrincipalAction = "replay"
)

// Binding 描述主体在某个主题上的权限。
type Binding struct {
	ID          string
	TopicID     string
	TenantID    string
	PrincipalID string
	Action      PrincipalAction
	GrantedBy   string
	GrantedAt   time.Time
	ExpiresAt   *time.Time
}

// GrantRequest 批量授予权限的输入。
type GrantRequest struct {
	TenantID    string
	TopicID     string
	PrincipalID string
	Actions     []PrincipalAction
	ExpiresAt   *time.Time
	OperatorID  string
}

// RevokeRequest 批量撤销权限的输入。
type RevokeRequest struct {
	TenantID    string
	TopicID     string
	PrincipalID string
	Actions     []PrincipalAction
	OperatorID  string
}

// Service 定义 ACL 领域的核心用例。
type Service interface {
	Grant(ctx context.Context, req GrantRequest) ([]*Binding, error)
	Revoke(ctx context.Context, req RevokeRequest) error
	ListBindings(ctx context.Context, tenantID string, topicID string) ([]*Binding, error)
	Can(ctx context.Context, tenantID string, principalID string, topicID string, action PrincipalAction) (bool, error)
}
