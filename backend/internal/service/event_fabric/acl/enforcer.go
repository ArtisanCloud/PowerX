package acl

import (
	"context"
	"strings"

	"github.com/google/uuid"
)

type ACLEnforcer struct {
	svc *ACLService
}

func NewACLEnforcer(svc *ACLService) *ACLEnforcer {
	return &ACLEnforcer{svc: svc}
}

func (e *ACLEnforcer) CanPublish(ctx context.Context, tenantKey string, topic uuid.UUID, principalID string) (bool, error) {
	return e.can(ctx, tenantKey, topic, principalID, PrincipalActionPublish)
}

func (e *ACLEnforcer) CanSubscribe(ctx context.Context, tenantKey string, topic uuid.UUID, principalID string) (bool, error) {
	return e.can(ctx, tenantKey, topic, principalID, PrincipalActionSubscribe)
}

func (e *ACLEnforcer) CanReplay(ctx context.Context, tenantKey string, topic uuid.UUID, principalID string) (bool, error) {
	return e.can(ctx, tenantKey, topic, principalID, PrincipalActionReplay)
}

func (e *ACLEnforcer) can(ctx context.Context, tenantKey string, topic uuid.UUID, principalID string, action PrincipalAction) (bool, error) {
	if e == nil || e.svc == nil {
		return false, nil
	}
	tenantKey = strings.TrimSpace(tenantKey)
	return e.svc.Can(ctx, tenantKey, topic, principalID, action)
}
