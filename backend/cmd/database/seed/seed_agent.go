// pkg/cmd/database/seed/agent.go
package seed

import (
	agentm "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	agentr "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/repository"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// SeedSystemDefaultAgent 在“系统租户(tenant_id=1)”下创建/更新默认Agent（不可删除）
func SeedSystemDefaultAgent(db *gorm.DB) error {
	ctx := seedCtx()
	env := envOrDefault("POWERX_ENV", "dev")

	tenantID := uint64(1) // ← 系统租户
	agentRepo := agentr.NewAgentRepository(db)
	settingRepo := agentr.NewAgentSettingRepository(db)

	const (
		agentKey  = "core.system.default"
		agentName = "System Default Agent"
	)

	// 已存在直接返回（幂等）
	if _, err := agentRepo.FindByScopeKey(ctx, env, &tenantID, agentKey); err == nil {
		return nil
	} else if err != nil && err != gorm.ErrRecordNotFound {
		return err
	}

	// 组装 Agent（挂在系统租户 tenant_id=1）
	a := &agentm.Agent{
		Env:      env,
		TenantID: &tenantID,

		Key:         agentKey,
		Name:        agentName,
		Description: "Built-in system agent (seed). Used as a safe default when tenant has not created its own agents.",

		Source:     "core",
		Scope:      agentm.AgentScopeSystem, // 语义上仍是“系统级”，但实体挂在租户1
		Visibility: agentm.AgentVisibilityPublic,
		Status:     agentm.AgentStatusActive,

		DefaultPersonaID: nil,
		BlueprintRefs:    datatypes.JSON{}, // e.g. [{"id":"core.demo.baseline","version":"1.0.0","entry":"main"}]
		IntentCardsRef:   datatypes.JSON{},
		ToolAllowlist:    datatypes.JSON{},

		KBStrategy: agentm.KBStrategyUnion,
		Meta: datatypes.JSONMap{
			"builtin":             true,
			"protect_from_delete": true,
			"icon":                "i-heroicons-cog-6-tooth",
			"tags":                []string{"system", "default"},
		},
	}

	// 用仓库 Upsert（租户级唯一：env + tenant_id + key）
	if err := agentRepo.UpsertByScopeKey(ctx, env, &tenantID, a); err != nil {
		return err
	}

	// 重新查询拿到 ID（避免 Upsert 未回填主键）
	dbAgent, err := agentRepo.FindByScopeKey(ctx, env, &tenantID, agentKey)
	if err != nil {
		return err
	}

	// 写入一条 Setting（不强制覆盖上游设置；健康状态占位）
	as := &agentm.AgentSetting{
		AgentID:       dbAgent.ID,
		Provider:      "", // 留空：按租户/系统默认解析
		Model:         "",
		Params:        datatypes.JSONMap{},
		OverrideFlags: datatypes.JSONMap{},
		QuotaPolicy:   datatypes.JSONMap{},
		HealthStatus:  "unknown",
		HealthInfo:    datatypes.JSONMap{},
	}
	if err := settingRepo.UpsertByAgent(ctx, env, &tenantID, as); err != nil {
		return err
	}

	return nil
}
