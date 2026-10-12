package runtime

import (
	"context"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestCapabilityApprovalServiceScopesAndResolvesOnlyApprovedEvidence(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec("ATTACH DATABASE ':memory:' AS public").Error)
	require.NoError(t, db.Exec(`CREATE TABLE public.agent_capability_approvals (
		id integer primary key, created_at datetime, updated_at datetime, deleted_at datetime,
		uuid text, env text, tenant_uuid text, run_uuid text, snapshot_uuid text, plan_revision_uuid text, agent_uuid text, session_uuid text, capability_uuid text,
		message_uuid text, requested_by_user_uuid text, approved_by_user_uuid text, status text, evidence_ref text,
		approved_at datetime, rejected_at datetime
	)`).Error)
	service := NewCapabilityApprovalService(db)
	scope := CapabilityApprovalScope{Env: "test", TenantUUID: uuid.NewString(), RunUUID: uuid.New(), SnapshotUUID: uuid.New(), PlanRevisionUUID: uuid.New(), AgentUUID: uuid.New(), SessionUUID: uuid.New(), MessageUUID: uuid.New(), SubjectUUID: uuid.NewString()}
	capabilityUUID := uuid.New()
	pending, err := service.EnsurePending(context.Background(), scope, capabilityUUID)
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, pending.ApprovalUUID)
	_, err = service.ResolveApproved(context.Background(), scope, []uuid.UUID{pending.ApprovalUUID})
	require.Error(t, err)
	require.Error(t, service.Approve(context.Background(), scope, pending.ApprovalUUID, scope.SubjectUUID))
	require.NoError(t, service.Approve(context.Background(), scope, pending.ApprovalUUID, uuid.NewString()))
	approved, err := service.ResolveApproved(context.Background(), scope, []uuid.UUID{pending.ApprovalUUID})
	require.NoError(t, err)
	require.Equal(t, capabilityUUID, approved[0].CapabilityUUID)
	resume, err := service.LoadApprovedForResume(context.Background(), scope.Env, scope.TenantUUID, scope.SubjectUUID, scope.AgentUUID, scope.SessionUUID, pending.ApprovalUUID)
	require.NoError(t, err)
	require.Equal(t, scope.RunUUID, resume.Scope.RunUUID)
	require.Equal(t, scope.MessageUUID, resume.Scope.MessageUUID)
	_, err = service.LoadApprovedForResume(context.Background(), scope.Env, scope.TenantUUID, uuid.NewString(), scope.AgentUUID, scope.SessionUUID, pending.ApprovalUUID)
	require.Error(t, err)
	_, err = service.ResolveApproved(context.Background(), CapabilityApprovalScope{Env: scope.Env, TenantUUID: scope.TenantUUID, AgentUUID: scope.AgentUUID, SessionUUID: uuid.New(), SubjectUUID: scope.SubjectUUID}, []uuid.UUID{pending.ApprovalUUID})
	require.Error(t, err)
}

func TestApprovalScopeUsesAuthenticatedUserIdentity(t *testing.T) {
	ctx := reqctx.WithUserUUID(context.Background(), uuid.NewString())
	ctx = context.WithValue(ctx, "env", "test")
	ctx = context.WithValue(ctx, "session_uuid", uuid.NewString())
	ctx = context.WithValue(ctx, "runtime_run_uuid", uuid.NewString())
	ctx = context.WithValue(ctx, "runtime_snapshot_uuid", uuid.NewString())
	ctx = context.WithValue(ctx, "runtime_plan_revision_uuid", uuid.NewString())
	ctx = context.WithValue(ctx, "message_uuid", uuid.NewString())
	snapshot, err := NewResourceSnapshot(uuid.NewString(), uuid.New(), nil, time.Now())
	require.NoError(t, err)
	scope, err := approvalScopeFromRuntime(ctx, snapshot)
	require.NoError(t, err)
	require.NotEmpty(t, scope.SubjectUUID)
}
