package runtime

import (
	"context"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/server/agent"
	agentconfig "github.com/ArtisanCloud/PowerX/internal/server/agent/config"
	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	agentschema "github.com/ArtisanCloud/PowerX/internal/server/agent/schemas"
	runtimescheduler "github.com/ArtisanCloud/PowerX/internal/service/runtime_scheduler"
	"github.com/ArtisanCloud/PowerX/pkg/corex/agent/evidence"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/runtime_scheduler"
	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestSchedulerCompletionVerifierReadsBackTenantJob(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec("ATTACH DATABASE ':memory:' AS public").Error)
	require.NoError(t, db.Exec(`CREATE TABLE public.scheduler_jobs (id integer primary key, uuid text, created_at datetime, updated_at datetime, deleted_at datetime, tenant_uuid text, owner_type text, owner_id text, name text, schedule_type text, schedule_expr text, timezone text, topic text, payload_json json, status text, next_run_at datetime, last_run_at datetime, misfire_policy text, overlap_policy text, retry_policy_json json, idempotency_key text, actor_type text, actor_user_id integer, actor_user_uuid text, actor_member_id integer, actor_member_uuid text, created_by text, updated_by text, last_error text, trace_id text)`).Error)
	tenantUUID := uuid.NewString()
	job := &model.SchedulerJob{TenantUUID: tenantUUID, OwnerType: model.OwnerTypePlugin, OwnerID: "com.test", Name: "job", ScheduleType: model.ScheduleTypeInterval, ScheduleExpr: "1m", Status: model.JobStatusActive}
	job.UUID = uuid.New()
	require.NoError(t, db.Exec("INSERT INTO public.scheduler_jobs (uuid, tenant_uuid, owner_type, owner_id, name, schedule_type, schedule_expr, status) VALUES (?, ?, ?, ?, ?, ?, ?, ?)", job.UUID.String(), job.TenantUUID, job.OwnerType, job.OwnerID, job.Name, job.ScheduleType, job.ScheduleExpr, job.Status).Error)
	ctx := reqctx.WithClaims(context.Background(), &reqctx.CoreXClaims{TenantUUID: tenantUUID, PluginID: "com.test", RegisteredClaims: jwt.RegisteredClaims{Audience: jwt.ClaimStrings{"powerx:api"}}})
	ctx = reqctx.WithTenantUUID(ctx, tenantUUID)
	verifier := NewSchedulerCompletionVerifier(runtimescheduler.NewService(runtimescheduler.Options{DB: db}))
	resource := ResourceDescriptor{CapabilityID: runtimescheduler.CapabilityID}
	out := &agentschema.ExecutionResult{Data: map[string]any{"result": map[string]any{"job": map[string]any{"uuid": job.UUID.String(), "status": model.JobStatusActive}}}}
	artifact, err := verifier.VerifyCompletion(ctx, resource, flowschema.PlanTask{}, out)
	require.NoError(t, err)
	require.Equal(t, "scheduler_job/"+job.UUID.String()+"/"+model.JobStatusActive, artifact)
	out.Data["result"] = map[string]any{"job": map[string]any{"uuid": job.UUID.String(), "status": model.JobStatusPaused}}
	_, err = verifier.VerifyCompletion(ctx, resource, flowschema.PlanTask{}, out)
	require.Error(t, err)

	otherTenantUUID := uuid.NewString()
	otherCtx := reqctx.WithClaims(context.Background(), &reqctx.CoreXClaims{TenantUUID: otherTenantUUID, PluginID: "com.test", RegisteredClaims: jwt.RegisteredClaims{Audience: jwt.ClaimStrings{"powerx:api"}}})
	otherCtx = reqctx.WithTenantUUID(otherCtx, otherTenantUUID)
	out.Data["result"] = map[string]any{"job": map[string]any{"uuid": job.UUID.String(), "status": model.JobStatusActive}}
	_, err = verifier.VerifyCompletion(otherCtx, resource, flowschema.PlanTask{}, out)
	require.Error(t, err)
}

func TestRunApprovedResumeExecutesSchedulerAndPersistsVerifiedCompletion(t *testing.T) {
	agentconfig.SetGlobalAIConfig(&agentconfig.AIConfig{Defaults: agentconfig.AIDefaults{LLM: agentconfig.LLMDefaults{RequestTimeout: 5 * time.Minute}}, Runtime: agentconfig.AIRuntime{MaxPlanRevisions: 2, MaxObservations: 4, MaxSteps: 16, MaxCapabilityCalls: 8, MaxConcurrentTasks: 4}})
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec("ATTACH DATABASE ':memory:' AS public").Error)
	require.NoError(t, db.Exec(`CREATE TABLE public.scheduler_jobs (id integer primary key, uuid text, created_at datetime, updated_at datetime, deleted_at datetime, tenant_uuid text, owner_type text, owner_id text, name text, schedule_type text, schedule_expr text, timezone text, topic text, payload_json json, status text, next_run_at datetime, last_run_at datetime, misfire_policy text, overlap_policy text, retry_policy_json json, idempotency_key text, actor_type text, actor_user_id integer, actor_user_uuid text, actor_member_id integer, actor_member_uuid text, created_by text, updated_by text, last_error text, trace_id text)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE public.agent_verification_evidences (id integer primary key, uuid text, created_at datetime, updated_at datetime, deleted_at datetime, env text, tenant_uuid text, run_uuid text, snapshot_uuid text, task_refs json, verdict_class text, reason_code text, user_action text, required_input_fields json, required_permission_codes json, artifact_ref text)`).Error)

	tenantUUID, capabilityUUID, runUUID, revisionUUID := uuid.NewString(), uuid.New(), uuid.New(), uuid.New()
	snapshot, err := NewResourceSnapshot(tenantUUID, uuid.New(), []ResourceDescriptor{{
		ResourceUUID: capabilityUUID, Kind: ResourceKindCapability, DisplayName: "scheduler jobs", TenantUUID: tenantUUID,
		CapabilityUUID: capabilityUUID, CapabilityID: runtimescheduler.CapabilityID, DiscoveryGranted: true, InvocationGranted: true,
		RuntimeContract: CapabilityRuntimeContract{VerificationRequired: true, SideEffectEvidenceSchema: "scheduler.job/v1", BusinessCompletionRequired: true, HumanApprovalRequired: true},
	}}, time.Now())
	require.NoError(t, err)

	claims := &reqctx.CoreXClaims{TenantUUID: tenantUUID, IsRoot: true, RegisteredClaims: jwt.RegisteredClaims{Audience: jwt.ClaimStrings{"powerx:api"}}}
	ctx := reqctx.WithClaims(context.Background(), claims)
	ctx = reqctx.WithTenantUUID(ctx, tenantUUID)
	ctx = reqctx.WithEnv(ctx, "test")
	ctx = ContextWithResourceSnapshot(ctx, snapshot)
	ctx = ContextWithPlanRevisionService(ctx, NewPlanRevisionService(db))
	ctx = ContextWithVerificationEvidenceService(ctx, NewVerificationEvidenceService(db))
	registry := NewCompletionVerifierRegistry()
	scheduler := runtimescheduler.NewService(runtimescheduler.Options{DB: db})
	require.NoError(t, registry.Register(runtimescheduler.CapabilityID, NewSchedulerCompletionVerifier(scheduler)))
	ctx = ContextWithCompletionVerifierRegistry(ctx, registry)
	ctx = context.WithValue(ctx, "runtime_run_uuid", runUUID.String())
	ctx = context.WithValue(ctx, "runtime_plan_revision_uuid", revisionUUID.String())
	ctx = context.WithValue(ctx, "runtime_approved_resume", true)
	ctx = context.WithValue(ctx, "session_id", uuid.NewString())
	ctx = context.WithValue(ctx, "message_id", uuid.NewString())
	ctx, err = ContextWithCapabilityApprovals(ctx, []CapabilityApproval{{ApprovalUUID: uuid.New(), CapabilityUUID: capabilityUUID, EvidenceRef: "approval:test"}})
	require.NoError(t, err)

	mgr := agent.NewAgentManager()
	mgr.SetToolingInvoker(func(callCtx context.Context, in agent.ToolingInvokeInput) (*agent.ToolingInvokeOutput, error) {
		require.Equal(t, runtimescheduler.CapabilityID, in.CapabilityID)
		job, createErr := scheduler.CreateJob(callCtx, runtimescheduler.JobSpec{
			TenantUUID: tenantUUID, OwnerType: model.OwnerTypeCore, OwnerID: "runtime", Name: "approved-resume-job",
			ScheduleType: model.ScheduleTypeInterval, ScheduleExpr: "1m", Payload: map[string]any{"action": "verified"},
		}, "runtime", "scheduler-invocation-trace")
		if createErr != nil {
			return nil, createErr
		}
		envelope, compileErr := evidence.Compile(callCtx, evidence.Draft{Schema: evidence.DraftSchema, Kind: "analysis", Data: []evidence.DraftDatum{}, Calculations: []evidence.Calculation{}, Hypotheses: []string{}, Gaps: []string{"completion_recorded"}, Actions: []string{}}, map[string]any{}, nil, tenantUUID, revisionUUID.String(), "scheduler-invocation-trace")
		if compileErr != nil {
			return nil, compileErr
		}
		return &agent.ToolingInvokeOutput{TraceID: "scheduler-invocation-trace", Status: "completed", ProtocolUsed: "internal", Result: map[string]any{
			"job": map[string]any{"uuid": job.UUID.String(), "status": job.Status}, "response_envelope": envelope,
		}}, nil
	})
	engine := &Engine{mgr: mgr}
	plan := flowschema.ExecutionPlan{PlanID: "approved_scheduler_resume", Tasks: []flowschema.PlanTask{{
		TaskID: "create_scheduler_job", NodeKind: "tooling", NodeRef: runtimescheduler.CapabilityID,
		Params: map[string]any{"capability_uuid": capabilityUUID.String(), "capability_id": runtimescheduler.CapabilityID, "payload": map[string]any{}},
	}}}
	sink := &captureSink{}
	require.NoError(t, engine.RunApprovedResume(ctx, plan, sink))

	var persisted dbmodel.AgentVerificationEvidence
	require.NoError(t, db.Table("public.agent_verification_evidences").Where("run_uuid = ? AND reason_code = ?", runUUID.String(), "execution.verified_pending").First(&persisted).Error)
	require.Equal(t, VerificationPass, persisted.VerdictClass)
	require.Equal(t, "execution.verified_pending", persisted.ReasonCode)
	require.Contains(t, persisted.ArtifactRef, "capability_verification/"+capabilityUUID.String()+"/invocation/scheduler-invocation-trace")
	var taskEvidence dbmodel.AgentVerificationEvidence
	require.NoError(t, db.Table("public.agent_verification_evidences").Where("run_uuid = ? AND reason_code = ?", runUUID.String(), "capability.verified").First(&taskEvidence).Error)
	require.Equal(t, "[\"create_scheduler_job\"]", string(taskEvidence.TaskRefs))
	require.Equal(t, persisted.ArtifactRef, taskEvidence.ArtifactRef)
	require.Contains(t, sink.events, "final")
}

func TestRunResolvedPlanExecutesInitialRevisionWithoutObservations(t *testing.T) {
	agentconfig.SetGlobalAIConfig(&agentconfig.AIConfig{Defaults: agentconfig.AIDefaults{LLM: agentconfig.LLMDefaults{RequestTimeout: 5 * time.Minute}}, Runtime: agentconfig.AIRuntime{MaxPlanRevisions: 2, MaxObservations: 4, MaxSteps: 16, MaxCapabilityCalls: 8, MaxConcurrentTasks: 4}})
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec("ATTACH DATABASE ':memory:' AS public").Error)
	require.NoError(t, db.Exec(`CREATE TABLE public.agent_plan_revisions (
 id integer primary key, uuid text, created_at datetime, updated_at datetime, deleted_at datetime,
 env text, tenant_uuid text, run_uuid text, snapshot_uuid text, parent_revision_uuid text, reason_code text,
 plan json, trigger_observation_uuids json, superseded_task_refs json, new_task_refs json,
 plan_revisions_consumed integer, observations_consumed integer
)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE public.agent_verification_evidences (
 id integer primary key, uuid text, created_at datetime, updated_at datetime, deleted_at datetime,
 env text, tenant_uuid text, run_uuid text, snapshot_uuid text, task_refs json, verdict_class text,
 reason_code text, user_action text, required_input_fields json, required_permission_codes json, artifact_ref text
)`).Error)

	tenantUUID, runUUID := uuid.NewString(), uuid.New()
	snapshot, err := NewResourceSnapshot(tenantUUID, uuid.New(), nil, time.Now())
	require.NoError(t, err)
	ctx := reqctx.WithTenantUUID(context.Background(), tenantUUID)
	ctx = reqctx.WithEnv(ctx, "test")
	ctx = ContextWithResourceSnapshot(ctx, snapshot)
	ctx = ContextWithPlanRevisionService(ctx, NewPlanRevisionService(db))
	ctx = ContextWithVerificationEvidenceService(ctx, NewVerificationEvidenceService(db))
	ctx = context.WithValue(ctx, "runtime_run_uuid", runUUID.String())

	mgr := agent.NewAgentManager()
	mgr.SetSkillInvoker(func(callCtx context.Context, in agent.SkillInvokeInput) (*agent.SkillInvokeOutput, error) {
		envelope, compileErr := evidence.Compile(callCtx, evidence.Draft{
			Schema: evidence.DraftSchema, Kind: "analysis", Data: []evidence.DraftDatum{}, Calculations: []evidence.Calculation{}, Hypotheses: []string{}, Gaps: []string{"no_observation_needed"}, Actions: []string{},
		}, map[string]any{}, nil, tenantUUID, runUUID.String(), "initial-plan-skill-trace")
		if compileErr != nil {
			return nil, compileErr
		}
		return &agent.SkillInvokeOutput{
			TraceID: "initial-plan-skill-trace", Status: "completed", ProtocolUsed: "skill", SkillID: in.SkillID,
			Result: map[string]any{"response_envelope": envelope},
		}, nil
	})
	engine := &Engine{mgr: mgr}
	plan := &flowschema.ExecutionPlan{PlanID: "initial_no_observation", Tasks: []flowschema.PlanTask{{
		TaskID: "summarize", NodeKind: "skill", NodeRef: "marketing.review_summarize",
	}}}
	sink := &captureSink{}

	out, err := engine.runResolvedPlan(ctx, plan, sink, nil)
	require.NoError(t, err)
	require.NotNil(t, out)
	require.Contains(t, sink.events, dto.EventFinal)
	require.Contains(t, sink.events, dto.EventEnd)
	var revisionCount int64
	require.NoError(t, db.Table("public.agent_plan_revisions").Where("run_uuid = ?", runUUID.String()).Count(&revisionCount).Error)
	require.Equal(t, int64(1), revisionCount)
}
