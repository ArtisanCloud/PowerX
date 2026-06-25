package skillsintegration

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	dbseed "github.com/ArtisanCloud/PowerX/cmd/database/seed"
	agentpkg "github.com/ArtisanCloud/PowerX/internal/server/agent"
	agentmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	agentschema "github.com/ArtisanCloud/PowerX/internal/server/agent/schemas"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	modelagent "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/agent"
	skillmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/skills"
	tenantmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type releaseHandoffCall struct {
	TaskID        string
	ChildAgentID  uint64
	TeamID        uint64
	Message       string
	Context       map[string]any
	FailurePolicy string
}

type releaseHandoffRecorder struct {
	mu        sync.Mutex
	calls     []releaseHandoffCall
	failTasks map[string]bool
}

func (r *releaseHandoffRecorder) invoke(ctx context.Context, in agentpkg.AgentHandoffInput) (*agentpkg.AgentHandoffOutput, error) {
	if r.failTasks != nil && r.failTasks[in.TaskID] {
		return nil, errors.New("mock release child agent failed")
	}
	r.mu.Lock()
	r.calls = append(r.calls, releaseHandoffCall{
		TaskID:        in.TaskID,
		ChildAgentID:  in.ChildAgentID,
		TeamID:        in.TeamID,
		Message:       in.Message,
		Context:       cloneMap(in.Context),
		FailurePolicy: in.FailurePolicy,
	})
	r.mu.Unlock()
	return &agentpkg.AgentHandoffOutput{
		TaskID:         in.TaskID,
		HandoffTraceID: "trace-" + in.TaskID,
		Status:         "completed",
		Result: map[string]any{
			"content": releaseReadinessContent(in.TaskID),
		},
	}, nil
}

func TestSkillAgentA2AReleaseReadinessSeed(t *testing.T) {
	db := setupReleaseReadinessDB(t)

	for i := 0; i < 3; i++ {
		require.NoError(t, dbseed.SeedA2AReleaseReadinessDemo(db))
	}

	require.Equal(t, int64(4), countAgentsByKeys(t, db, releaseAgentKeys()))
	require.Equal(t, int64(4), countReleaseSkills(t, db))

	var team modelagent.AgentTeam
	require.NoError(t, db.Where("team_name = ?", dbseed.ReleaseReadinessTeamName).First(&team).Error)
	require.Equal(t, modelagent.TeamStatusActive, team.Status)

	var members int64
	require.NoError(t, db.Model(&modelagent.AgentTeamMember{}).Where("team_id = ? AND enabled = ?", team.ID, true).Count(&members).Error)
	require.Equal(t, int64(3), members)

	var bindings int64
	require.NoError(t, db.Model(&agentmodel.AgentSkillBinding{}).Where("skill_id IN ?", releaseSkillIDs()).Where("enabled = ?", true).Count(&bindings).Error)
	require.Equal(t, int64(4), bindings)
}

func TestSkillAgentA2AReleaseReadinessMVP(t *testing.T) {
	db := setupReleaseReadinessDB(t)
	require.NoError(t, dbseed.SeedA2AReleaseReadinessDemo(db))
	ids := loadReleaseAgentIDs(t, db)
	team := loadReleaseTeam(t, db)

	m := agentpkg.NewAgentManager()
	parent := &multiPlanStubAgent{}
	require.NoError(t, m.Register(dbseed.ReleaseCoordinatorAgentKey, parent, &agentschema.AgentMeta{FlowID: "release.final"}))
	require.NoError(t, m.SetDefaultAgent(dbseed.ReleaseCoordinatorAgentKey, "release.final"))
	recorder := &releaseHandoffRecorder{}
	m.SetAgentHandoffInvoker(recorder.invoke)
	m.SetSkillInvoker(func(ctx context.Context, in agentpkg.SkillInvokeInput) (*agentpkg.SkillInvokeOutput, error) {
		return &agentpkg.SkillInvokeOutput{
			TraceID:      "trace-report-synthesis",
			Status:       "completed",
			ProtocolUsed: "skill",
			SkillID:      in.SkillID,
			Version:      "1.0.0",
			Result: map[string]any{
				"content": "发布准备报告：风险摘要已完成；发布流程已完成；回滚步骤已完成；通知计划已完成。",
			},
		}, nil
	})

	out, err := m.ExecutePlan(context.Background(), releaseReadinessPlan(ids, team.ID), agentschema.ExecutionMeta{
		RequestID:  "req-release-readiness",
		TenantUUID: team.TenantUUID,
		TraceID:    "trace-release-readiness",
		Metadata: map[string]any{
			"agent_id": ids[dbseed.ReleaseCoordinatorAgentKey],
			"run_id":   "run-release-readiness",
			"plan_id":  "release_readiness_multi_agent_mvp",
		},
	})
	require.NoError(t, err)
	require.NotNil(t, out)
	require.True(t, out.Success)
	require.Contains(t, fmt.Sprintf("%v", out.Data["result"]), "发布准备报告")

	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	require.Len(t, recorder.calls, 3)
	for _, call := range recorder.calls {
		require.Equal(t, team.ID, call.TeamID)
		require.NotZero(t, call.ChildAgentID)
		require.Equal(t, "continue", call.FailurePolicy)
		require.NotContains(t, call.Context, "session_messages")
		require.Equal(t, dbseed.ReleaseReadinessTeamName, call.Context["team_name"])
	}
}

func TestSkillAgentA2AReleaseReadinessPartialFailure(t *testing.T) {
	db := setupReleaseReadinessDB(t)
	require.NoError(t, dbseed.SeedA2AReleaseReadinessDemo(db))
	ids := loadReleaseAgentIDs(t, db)
	team := loadReleaseTeam(t, db)

	m := agentpkg.NewAgentManager()
	parent := &multiPlanStubAgent{}
	require.NoError(t, m.Register(dbseed.ReleaseCoordinatorAgentKey, parent, &agentschema.AgentMeta{FlowID: "release.final"}))
	require.NoError(t, m.SetDefaultAgent(dbseed.ReleaseCoordinatorAgentKey, "release.final"))
	m.SetAgentHandoffInvoker((&releaseHandoffRecorder{failTasks: map[string]bool{"notification_schedule": true}}).invoke)
	m.SetSkillInvoker(func(ctx context.Context, in agentpkg.SkillInvokeInput) (*agentpkg.SkillInvokeOutput, error) {
		deps := in.Context["_deps"]
		return &agentpkg.SkillInvokeOutput{
			TraceID:      "trace-report-partial",
			Status:       "completed",
			ProtocolUsed: "skill",
			SkillID:      in.SkillID,
			Version:      "1.0.0",
			Result: map[string]any{
				"content": fmt.Sprintf("发布准备报告：部分成功，通知计划失败，deps=%v", deps),
			},
		}, nil
	})

	out, err := m.ExecutePlan(context.Background(), releaseReadinessPlan(ids, team.ID), agentschema.ExecutionMeta{
		RequestID:  "req-release-readiness-partial",
		TenantUUID: team.TenantUUID,
		TraceID:    "trace-release-readiness-partial",
		Metadata: map[string]any{
			"agent_id": ids[dbseed.ReleaseCoordinatorAgentKey],
			"run_id":   "run-release-readiness-partial",
			"plan_id":  "release_readiness_multi_agent_mvp",
		},
	})
	require.NoError(t, err)
	require.NotNil(t, out)
	require.True(t, out.Success)
	require.Contains(t, fmt.Sprintf("%v", out.Data["result"]), "部分成功")
	require.Contains(t, fmt.Sprintf("%v", out.Data["result"]), "通知计划失败")
}

func TestSkillAgentA2AReleaseReadinessContextIsolation(t *testing.T) {
	db := setupReleaseReadinessDB(t)
	require.NoError(t, dbseed.SeedA2AReleaseReadinessDemo(db))
	ids := loadReleaseAgentIDs(t, db)
	team := loadReleaseTeam(t, db)

	m := agentpkg.NewAgentManager()
	recorder := &releaseHandoffRecorder{}
	m.SetAgentHandoffInvoker(recorder.invoke)
	_, err := m.ExecutePlan(context.Background(), flowschema.ExecutionPlan{
		PlanID: "release_readiness_context_isolation",
		Tasks: []flowschema.PlanTask{
			{
				TaskID:        "knowledge_analysis",
				FlowID:        "release.knowledge",
				NodeKind:      "agent_handoff",
				NodeRef:       dbseed.ReleaseKnowledgeAnalystAgentKey,
				AgentID:       dbseed.ReleaseKnowledgeAnalystAgentKey,
				TeamID:        fmt.Sprintf("%d", team.ID),
				HandoffTaskID: "knowledge_analysis",
				FailurePolicy: "continue",
				Stage:         1,
				Params: map[string]any{
					"team_name":       dbseed.ReleaseReadinessTeamName,
					"child_agent_id":  ids[dbseed.ReleaseKnowledgeAnalystAgentKey],
					"child_agent_key": dbseed.ReleaseKnowledgeAnalystAgentKey,
					"message":         "分析发布风险",
					"context": map[string]any{
						"release_name": "PowerX Core v0.9.2",
						"release_date": "2026-06-18",
						"focus_areas":  []string{"Agent Skill Bridge", "插件安装", "回滚风险"},
					},
				},
			},
		},
	}, agentschema.ExecutionMeta{
		RequestID:  "req-context-isolation",
		TenantUUID: team.TenantUUID,
		TraceID:    "trace-context-isolation",
		Metadata:   map[string]any{"agent_id": ids[dbseed.ReleaseCoordinatorAgentKey]},
	})
	require.NoError(t, err)

	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	require.Len(t, recorder.calls, 1)
	ctx := recorder.calls[0].Context
	require.Equal(t, "PowerX Core v0.9.2", ctx["release_name"])
	require.Equal(t, "2026-06-18", ctx["release_date"])
	require.NotContains(t, ctx, "session_messages")
	require.NotContains(t, ctx, "global_candidates")
	require.NotContains(t, ctx, "unauthorized_skills")
}

func setupReleaseReadinessDB(t *testing.T) *gorm.DB {
	t.Helper()
	dbName := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared&parseTime=true&_loc=UTC", dbName)), &gorm.Config{})
	require.NoError(t, err)

	prevSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = prevSchema })

	require.NoError(t, db.AutoMigrate(
		&tenantmodel.Tenant{},
		&modelagent.AgentTeam{},
		&modelagent.AgentTeamMember{},
		&modelagent.AgentHandoffTask{},
		&modelagent.AgentSharedContextRef{},
	))
	require.NoError(t, createReleaseReadinessSQLiteTables(db))
	return db
}

func createReleaseReadinessSQLiteTables(db *gorm.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS agents (
			id integer primary key autoincrement,
			uuid text,
			created_at datetime,
			updated_at datetime,
			deleted_at datetime,
			env text,
			tenant_uuid text,
			key text,
			name text,
			description text,
			type_id text,
			scene text,
			prompt_seed text,
			persona text,
			source text,
			owner_plugin_id text,
			owner_tenant_uuid text,
			managed_by_plugin numeric,
			scope text,
			visibility text,
			status text,
			default_persona_id integer,
			blueprint_refs text,
			intent_cards_ref text,
			tool_allowlist text,
			session_singleton numeric,
			default_ttl_days integer,
			default_max_kb integer,
			default_max_tokens integer,
			kb_strategy text,
			meta text
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS agent_key_uniq_tenant ON agents(env, tenant_uuid, key)`,
		`CREATE TABLE IF NOT EXISTS agent_skill_bindings (
			id integer primary key autoincrement,
			created_at datetime,
			updated_at datetime,
			deleted_at datetime,
			env text,
			tenant_uuid text,
			agent_id integer,
			skill_id text,
			priority integer,
			enabled numeric
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS agent_skill_uniq ON agent_skill_bindings(env, tenant_uuid, agent_id, skill_id)`,
		`CREATE TABLE IF NOT EXISTS skills_registry_records (
			id integer primary key autoincrement,
			uuid text,
			created_at datetime,
			updated_at datetime,
			deleted_at datetime,
			skill_id text,
			version text,
			source text,
			status text,
			is_latest_published numeric,
			bundle_uri text,
			checksum text,
			signature text,
			manifest_json text,
			source_url text,
			source_ref text,
			import_type text,
			updated_by text,
			published_at datetime,
			latest_switched_at datetime,
			approval_note text,
			integrity_policy_ref text
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uk_skill_registry_skill_version ON skills_registry_records(skill_id, version)`,
	}
	for _, stmt := range stmts {
		if err := db.Exec(stmt).Error; err != nil {
			return err
		}
	}
	return nil
}

func releaseReadinessPlan(ids map[string]uint64, teamID uint64) flowschema.ExecutionPlan {
	return flowschema.ExecutionPlan{
		PlanID: "release_readiness_multi_agent_mvp",
		Tasks: []flowschema.PlanTask{
			releaseHandoffTask("knowledge_analysis", "release.knowledge", dbseed.ReleaseKnowledgeAnalystAgentKey, ids[dbseed.ReleaseKnowledgeAnalystAgentKey], teamID, "分析 PowerX Core v0.9.2 发布相关知识、历史风险和 Agent Skill Bridge / 插件安装风险。", 1, nil),
			releaseHandoffTask("workflow_planning", "release.workflow", dbseed.ReleaseWorkflowPlannerAgentKey, ids[dbseed.ReleaseWorkflowPlannerAgentKey], teamID, "基于风险分析生成发布流程、验证清单和回滚步骤。", 2, []string{"knowledge_analysis"}),
			releaseHandoffTask("notification_schedule", "release.notification", dbseed.ReleaseNotificationSchedulerAgentKey, ids[dbseed.ReleaseNotificationSchedulerAgentKey], teamID, "根据发布流程生成通知计划、提醒计划和值班升级路径。", 3, []string{"workflow_planning"}),
			{
				TaskID:        "report_synthesis",
				FlowID:        dbseed.ReleaseReportSynthesisSkillID,
				NodeKind:      "skill",
				NodeRef:       dbseed.ReleaseReportSynthesisSkillID,
				SourceScope:   "system",
				AgentID:       dbseed.ReleaseCoordinatorAgentKey,
				FailurePolicy: "fail-fast",
				Stage:         4,
				DependsOn:     []string{"knowledge_analysis", "workflow_planning", "notification_schedule"},
				Params: map[string]any{
					"release_name": "PowerX Core v0.9.2",
					"release_date": "2026-06-18",
				},
			},
		},
	}
}

func releaseHandoffTask(taskID, flowID, childKey string, childID uint64, teamID uint64, message string, stage int, dependsOn []string) flowschema.PlanTask {
	return flowschema.PlanTask{
		TaskID:        taskID,
		FlowID:        flowID,
		NodeKind:      "agent_handoff",
		NodeRef:       childKey,
		AgentID:       childKey,
		TeamID:        fmt.Sprintf("%d", teamID),
		HandoffTaskID: taskID,
		FailurePolicy: "continue",
		Stage:         stage,
		DependsOn:     dependsOn,
		Params: map[string]any{
			"team_name":       dbseed.ReleaseReadinessTeamName,
			"child_agent_id":  childID,
			"child_agent_key": childKey,
			"message":         message,
			"context": map[string]any{
				"release_name": "PowerX Core v0.9.2",
				"release_date": "2026-06-18",
				"focus_areas":  []string{"Agent Skill Bridge", "插件安装", "回滚风险"},
			},
		},
	}
}

func loadReleaseAgentIDs(t *testing.T, db *gorm.DB) map[string]uint64 {
	t.Helper()
	var rows []agentmodel.Agent
	require.NoError(t, db.Where("key IN ?", releaseAgentKeys()).Find(&rows).Error)
	require.Len(t, rows, 4)
	out := map[string]uint64{}
	for _, row := range rows {
		out[row.Key] = row.ID
	}
	return out
}

func loadReleaseTeam(t *testing.T, db *gorm.DB) modelagent.AgentTeam {
	t.Helper()
	var team modelagent.AgentTeam
	require.NoError(t, db.Where("team_name = ?", dbseed.ReleaseReadinessTeamName).First(&team).Error)
	return team
}

func countAgentsByKeys(t *testing.T, db *gorm.DB, keys []string) int64 {
	t.Helper()
	var count int64
	require.NoError(t, db.Model(&agentmodel.Agent{}).Where("key IN ?", keys).Count(&count).Error)
	return count
}

func countReleaseSkills(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var count int64
	require.NoError(t, db.Model(&skillmodel.SkillRegistryRecord{}).
		Where("skill_id IN ?", releaseSkillIDs()).
		Where("status = ? AND is_latest_published = ?", skillmodel.SkillStatusPublished, true).
		Count(&count).Error)
	return count
}

func releaseAgentKeys() []string {
	return []string{
		dbseed.ReleaseCoordinatorAgentKey,
		dbseed.ReleaseKnowledgeAnalystAgentKey,
		dbseed.ReleaseWorkflowPlannerAgentKey,
		dbseed.ReleaseNotificationSchedulerAgentKey,
	}
}

func releaseSkillIDs() []string {
	return []string{
		dbseed.ReleaseKnowledgeAnalysisSkillID,
		dbseed.ReleaseWorkflowPlanningSkillID,
		dbseed.ReleaseNotificationScheduleSkillID,
		dbseed.ReleaseReportSynthesisSkillID,
	}
}

func releaseReadinessContent(taskID string) string {
	switch taskID {
	case "knowledge_analysis":
		return "风险摘要：Agent Skill Bridge、插件安装、回滚风险需要重点验证。"
	case "workflow_planning":
		return "发布流程：冻结变更、执行发布、验证 checklist、准备回滚步骤。"
	case "notification_schedule":
		return "通知计划：发布前通知、发布中提醒、异常升级路径。"
	default:
		return "发布准备子任务完成。"
	}
}

func cloneMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
