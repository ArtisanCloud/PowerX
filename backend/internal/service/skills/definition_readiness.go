package skills

import (
	"context"
	"fmt"
	"github.com/ArtisanCloud/PowerX/pkg/utils"

	skillmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/skills"
)

// CheckRunnable 在绑定时验证本租户已发布定义及工具依赖，避免错误配置进入运行入口。
func (s *DefinitionService) CheckRunnable(ctx context.Context, tenantUUID, skillKey string) error {
	return s.CheckRunnableForAgent(ctx, tenantUUID, skillKey, "", 0)
}

// CheckRunnableForAgent 绑定时必须保留实际目标 Agent，不能以发布者授权代替 Agent grant。
func (s *DefinitionService) CheckRunnableForAgent(ctx context.Context, tenantUUID, skillKey, env string, agentID uint64) error {
	if err := requireUUID(tenantUUID, "tenant_uuid"); err != nil {
		return err
	}
	draft, err := s.repo.GetDraftBySkillID(ctx, tenantUUID, skillKey)
	if err != nil {
		return err
	}
	if draft.Status != skillmodel.SkillDefinitionDraftStatusPublished {
		return fmt.Errorf("skill.definition_not_published")
	}
	revision, err := s.repo.GetCurrentRevision(ctx, tenantUUID, draft.UUID.String())
	if err != nil {
		return err
	}
	if revision.Status != skillmodel.SkillDefinitionRevisionStatusPublished || revision.PublishedArtifactURI == "" || revision.PublishedChecksum == "" {
		return fmt.Errorf("skill.definition_revision_not_published")
	}
	var definition map[string]any
	if err := utils.DecodeStrictJSON(revision.DefinitionJSON, &definition); err != nil {
		return err
	}
	if err := validatePowerXDefinition(definition); err != nil {
		return err
	}
	if asStringInterface(nestedManifestMap(definition, "executor")["type"]) == "instruction_only" {
		return fmt.Errorf("skill.executor_instruction_only_not_runnable")
	}
	return s.dependencies.Check(ctx, DependencyCheckInput{TenantUUID: tenantUUID, Stage: DependencyBind, Env: env, AgentID: agentID, Definition: definition})
}
