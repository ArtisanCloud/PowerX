package seed

import (
	"context"
	"fmt"
	"strings"
	"time"

	agentmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	agentrepo "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/repository"
	modelagent "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/agent"
	skillmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/skills"
	tenantmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	repoagent "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/agent"
	tenantrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/tenant"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	ReleaseCoordinatorAgentKey           = "release.coordinator"
	ReleaseKnowledgeAnalystAgentKey      = "release.knowledge_analyst"
	ReleaseWorkflowPlannerAgentKey       = "release.workflow_planner"
	ReleaseNotificationSchedulerAgentKey = "release.notification_scheduler"

	ReleaseReadinessTeamName = "release.readiness.team"

	ReleaseKnowledgeAnalysisSkillID    = "powerx.release.knowledge_analysis"
	ReleaseWorkflowPlanningSkillID     = "powerx.release.workflow_planning"
	ReleaseNotificationScheduleSkillID = "powerx.release.notification_schedule"
	ReleaseReportSynthesisSkillID      = "powerx.release.report_synthesis"
)

type releaseAgentSeed struct {
	Key         string
	Name        string
	Description string
	Role        string
	SkillIDs    []string
}

type releaseSkillSeed struct {
	SkillID      string
	Name         string
	Description  string
	InputSchema  string
	OutputSchema string
	Examples     []string
}

func SeedA2AReleaseReadinessDemo(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("db is required")
	}
	ctx := seedCtx()
	env := envOrDefault("POWERX_ENV", "dev")

	tenantRepo := tenantrepo.NewTenantRepository(db)
	sysTenant, err := tenantRepo.EnsureByKey(ctx, tenantmodel.SystemTenantKey, "System", tenantmodel.TenantPlanFree, tenantmodel.TenantTypeSystem)
	if err != nil {
		return fmt.Errorf("ensure system tenant: %w", err)
	}
	tenantUUID := sysTenant.UUID.String()

	if err = seedReleaseReadinessSkills(ctx, db); err != nil {
		return err
	}

	agentIDs := map[string]uint64{}
	for _, item := range releaseReadinessAgentSeeds() {
		agentID, errAgent := seedReleaseReadinessAgent(ctx, db, env, tenantUUID, item)
		if errAgent != nil {
			return errAgent
		}
		agentIDs[item.Key] = agentID
		if errBind := agentrepo.NewAgentSkillBindingRepository(db).Replace(ctx, env, &tenantUUID, agentID, item.SkillIDs); errBind != nil {
			return fmt.Errorf("bind release readiness skills for %s failed: %w", item.Key, errBind)
		}
	}

	team, err := seedReleaseReadinessTeam(ctx, db, tenantUUID, agentIDs[ReleaseCoordinatorAgentKey])
	if err != nil {
		return err
	}
	members := []struct {
		Key      string
		Role     string
		Priority int
	}{
		{Key: ReleaseKnowledgeAnalystAgentKey, Role: "retriever", Priority: 10},
		{Key: ReleaseWorkflowPlannerAgentKey, Role: "executor", Priority: 20},
		{Key: ReleaseNotificationSchedulerAgentKey, Role: "reviewer", Priority: 30},
	}
	memberRepo := repoagent.NewAgentTeamMemberRepository(db)
	for _, member := range members {
		childID := agentIDs[member.Key]
		if childID == 0 {
			return fmt.Errorf("release readiness child agent missing: %s", member.Key)
		}
		if _, err = memberRepo.Upsert(ctx, &modelagent.AgentTeamMember{
			TeamID:       team.ID,
			TenantUUID:   tenantUUID,
			ChildAgentID: childID,
			Role:         member.Role,
			Priority:     member.Priority,
			Enabled:      true,
		}); err != nil {
			return fmt.Errorf("upsert release readiness team member %s failed: %w", member.Key, err)
		}
	}

	logger.InfoF(logger.WithLogFields(context.Background(), map[string]interface{}{"module": "legacy"}), "[seed] release readiness A2A demo ready: team=%s", ReleaseReadinessTeamName)
	return nil
}

func releaseReadinessAgentSeeds() []releaseAgentSeed {
	return []releaseAgentSeed{
		{
			Key:         ReleaseCoordinatorAgentKey,
			Name:        "Release Coordinator",
			Description: "Coordinates release readiness multi-agent jobs and synthesizes the final report.",
			Role:        "planner",
			SkillIDs:    []string{ReleaseReportSynthesisSkillID},
		},
		{
			Key:         ReleaseKnowledgeAnalystAgentKey,
			Name:        "Release Knowledge Analyst",
			Description: "Analyzes release knowledge, historical incidents, and risk evidence.",
			Role:        "retriever",
			SkillIDs:    []string{ReleaseKnowledgeAnalysisSkillID},
		},
		{
			Key:         ReleaseWorkflowPlannerAgentKey,
			Name:        "Release Workflow Planner",
			Description: "Builds release execution steps, validation checklist, and rollback plan.",
			Role:        "executor",
			SkillIDs:    []string{ReleaseWorkflowPlanningSkillID},
		},
		{
			Key:         ReleaseNotificationSchedulerAgentKey,
			Name:        "Release Notification Scheduler",
			Description: "Builds notification, reminder, and escalation schedules for releases.",
			Role:        "reviewer",
			SkillIDs:    []string{ReleaseNotificationScheduleSkillID},
		},
	}
}

func releaseReadinessSkillSeeds() []releaseSkillSeed {
	return []releaseSkillSeed{
		{
			SkillID:      ReleaseKnowledgeAnalysisSkillID,
			Name:         "Release Knowledge Analysis",
			Description:  "Analyze release notes, historical risks, and focus areas for release readiness.",
			InputSchema:  `{"type":"object","required":["release_name","focus_areas"],"properties":{"release_name":{"type":"string"},"release_date":{"type":"string"},"focus_areas":{"type":"array","items":{"type":"string"}}}}`,
			OutputSchema: `{"type":"object","properties":{"risk_summary":{"type":"string"},"evidence":{"type":"array","items":{"type":"string"}},"focus_modules":{"type":"array","items":{"type":"string"}}}}`,
			Examples:     []string{"分析 PowerX Core v0.9.2 的发布风险", "检查 Agent Skill Bridge 和插件安装风险"},
		},
		{
			SkillID:      ReleaseWorkflowPlanningSkillID,
			Name:         "Release Workflow Planning",
			Description:  "Create release steps, verification checklist, and rollback plan from risk analysis.",
			InputSchema:  `{"type":"object","required":["risk_summary"],"properties":{"risk_summary":{"type":"string"},"release_name":{"type":"string"},"release_date":{"type":"string"}}}`,
			OutputSchema: `{"type":"object","properties":{"release_steps":{"type":"array","items":{"type":"string"}},"validation_checklist":{"type":"array","items":{"type":"string"}},"rollback_plan":{"type":"array","items":{"type":"string"}}}}`,
			Examples:     []string{"基于风险分析生成发布流程", "生成回滚步骤和验证清单"},
		},
		{
			SkillID:      ReleaseNotificationScheduleSkillID,
			Name:         "Release Notification Schedule",
			Description:  "Create notification recipients, timing, reminders, and escalation path for release operations.",
			InputSchema:  `{"type":"object","required":["release_steps"],"properties":{"release_steps":{"type":"array","items":{"type":"string"}},"release_date":{"type":"string"}}}`,
			OutputSchema: `{"type":"object","properties":{"recipients":{"type":"array","items":{"type":"string"}},"schedule":{"type":"array","items":{"type":"string"}},"escalation_path":{"type":"array","items":{"type":"string"}}}}`,
			Examples:     []string{"生成发布通知计划", "生成值班提醒和升级路径"},
		},
		{
			SkillID:      ReleaseReportSynthesisSkillID,
			Name:         "Release Report Synthesis",
			Description:  "Synthesize child agent outputs into a release readiness report.",
			InputSchema:  `{"type":"object","required":["knowledge_analysis","workflow_planning","notification_schedule"],"properties":{"knowledge_analysis":{"type":"object"},"workflow_planning":{"type":"object"},"notification_schedule":{"type":"object"}}}`,
			OutputSchema: `{"type":"object","properties":{"report":{"type":"string"},"status":{"type":"string"},"sections":{"type":"array","items":{"type":"string"}}}}`,
			Examples:     []string{"汇总发布准备报告", "整合风险、流程、回滚和通知计划"},
		},
	}
}

func seedReleaseReadinessSkills(ctx context.Context, db *gorm.DB) error {
	now := time.Now().UTC()
	for _, item := range releaseReadinessSkillSeeds() {
		manifest := datatypes.JSON([]byte(fmt.Sprintf(`{
			"name":%q,
			"description":%q,
			"version":"1.0.0",
			"schema":"powerx.skill-manifest.v1",
			"source":"builtin",
			"entrypoints":["runbook.default"],
			"intent_examples":%s,
			"input_schema":%s,
			"output_schema":%s
		}`, item.Name, item.Description, jsonStringArray(item.Examples), item.InputSchema, item.OutputSchema)))
		record := &skillmodel.SkillRegistryRecord{
			SkillID:            item.SkillID,
			Version:            "1.0.0",
			Source:             skillmodel.SkillSourceBuiltin,
			Status:             skillmodel.SkillStatusPublished,
			IsLatestPublished:  true,
			BundleURI:          "builtin://skills/" + strings.ReplaceAll(item.SkillID, ".", "/") + "/1.0.0",
			Checksum:           "sha256:" + strings.ReplaceAll(item.SkillID, ".", "-") + "-1.0.0",
			ManifestJSON:       manifest,
			ImportType:         "a2a_demo_seed",
			UpdatedBy:          "seed",
			PublishedAt:        &now,
			LatestSwitchedAt:   &now,
			ApprovalNote:       "seed release readiness A2A demo skill",
			IntegrityPolicyRef: "builtin-default",
		}
		record.Normalize()
		if err := db.WithContext(ctx).Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "skill_id"}, {Name: "version"}},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"source":               record.Source,
				"status":               record.Status,
				"is_latest_published":  record.IsLatestPublished,
				"bundle_uri":           record.BundleURI,
				"checksum":             record.Checksum,
				"manifest_json":        record.ManifestJSON,
				"import_type":          record.ImportType,
				"updated_by":           record.UpdatedBy,
				"published_at":         record.PublishedAt,
				"latest_switched_at":   record.LatestSwitchedAt,
				"approval_note":        record.ApprovalNote,
				"integrity_policy_ref": record.IntegrityPolicyRef,
				"updated_at":           now,
			}),
		}).Create(record).Error; err != nil {
			return fmt.Errorf("upsert release readiness skill %s failed: %w", item.SkillID, err)
		}
	}
	return nil
}

func seedReleaseReadinessAgent(ctx context.Context, db *gorm.DB, env string, tenantUUID string, item releaseAgentSeed) (uint64, error) {
	agentRepo := agentrepo.NewAgentRepository(db)
	a := &agentmodel.Agent{
		Env:            env,
		TenantUUID:     &tenantUUID,
		Key:            item.Key,
		Name:           item.Name,
		Description:    item.Description,
		TypeID:         "release_readiness",
		Scene:          "ai_engineering.release",
		PromptSeed:     fmt.Sprintf("You are %s. Work only within the PowerX release readiness task assigned to you.", item.Name),
		Persona:        item.Role,
		Source:         "core",
		Scope:          agentmodel.AgentScopeTenant,
		Visibility:     agentmodel.AgentVisibilityTenant,
		Status:         agentmodel.AgentStatusActive,
		BlueprintRefs:  datatypes.JSON([]byte(`[]`)),
		IntentCardsRef: datatypes.JSON([]byte(`[]`)),
		ToolAllowlist:  datatypes.JSON([]byte(`[]`)),
		KBStrategy:     agentmodel.KBStrategyUnion,
		Meta: datatypes.JSONMap{
			"builtin_demo": true,
			"a2a_demo":     "release_readiness",
			"role":         item.Role,
			"managed_by":   "powerx_core_seed",
		},
	}
	if err := agentRepo.UpsertByScopeKey(ctx, env, &tenantUUID, a); err != nil {
		return 0, fmt.Errorf("upsert release readiness agent %s failed: %w", item.Key, err)
	}
	found, err := agentRepo.FindByScopeKey(ctx, env, &tenantUUID, item.Key)
	if err != nil {
		return 0, fmt.Errorf("find release readiness agent %s failed: %w", item.Key, err)
	}
	return found.ID, nil
}

func seedReleaseReadinessTeam(ctx context.Context, db *gorm.DB, tenantUUID string, parentAgentID uint64) (*modelagent.AgentTeam, error) {
	if parentAgentID == 0 {
		return nil, fmt.Errorf("release readiness parent agent id is required")
	}
	var team modelagent.AgentTeam
	err := db.WithContext(ctx).
		Where("tenant_uuid = ? AND parent_agent_id = ? AND team_name = ?", strings.ToLower(strings.TrimSpace(tenantUUID)), parentAgentID, ReleaseReadinessTeamName).
		First(&team).Error
	switch err {
	case nil:
		updates := map[string]any{
			"dispatch_mode":          modelagent.DispatchModeMixed,
			"default_failure_policy": modelagent.FailurePolicyContinue,
			"status":                 modelagent.TeamStatusActive,
			"created_by":             "seed",
			"updated_at":             time.Now().UTC(),
		}
		if err = db.WithContext(ctx).Model(&modelagent.AgentTeam{}).Where("id = ?", team.ID).Updates(updates).Error; err != nil {
			return nil, fmt.Errorf("update release readiness team failed: %w", err)
		}
		if err = db.WithContext(ctx).First(&team, team.ID).Error; err != nil {
			return nil, err
		}
		return &team, nil
	case gorm.ErrRecordNotFound:
		team = modelagent.AgentTeam{
			TenantUUID:           tenantUUID,
			ParentAgentID:        parentAgentID,
			TeamName:             ReleaseReadinessTeamName,
			DispatchMode:         modelagent.DispatchModeMixed,
			DefaultFailurePolicy: modelagent.FailurePolicyContinue,
			Status:               modelagent.TeamStatusActive,
			CreatedBy:            "seed",
		}
		team.Normalize()
		if err = db.WithContext(ctx).Create(&team).Error; err != nil {
			return nil, fmt.Errorf("create release readiness team failed: %w", err)
		}
		return &team, nil
	default:
		return nil, err
	}
}

func jsonStringArray(values []string) string {
	if len(values) == 0 {
		return "[]"
	}
	quoted := make([]string, 0, len(values))
	for _, value := range values {
		quoted = append(quoted, fmt.Sprintf("%q", value))
	}
	return "[" + strings.Join(quoted, ",") + "]"
}
