package registry

import (
	"context"
)

const toolGrantAction = "capability_registry.use_tool_grant"

func (s *Service) auditToolGrantCheck(ctx context.Context, actor, tenantID string, grants []string, err error) {
	if s.auditor == nil || len(grants) == 0 {
		return
	}
	if actor == "" {
		actor = s.systemActorLookup(ctx)
	}
	allowed := err == nil
	for _, grantID := range grants {
		s.auditor.LogRBAC(ctx, actor, grantID, toolGrantAction, allowed)
	}
	if err != nil {
		s.instrumentation.Logger(ctx).WarnF(ctx, "[registry.audit] tool grant verification failed: tenant=%s grants=%v err=%v", tenantID, grants, err)
	} else {
		s.instrumentation.Logger(ctx).InfoF(ctx, "[registry.audit] tool grant verified: tenant=%s actor=%s grants=%v", tenantID, actor, grants)
	}
}
