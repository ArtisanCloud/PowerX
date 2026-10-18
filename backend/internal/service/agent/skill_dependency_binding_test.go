package agent

import (
	"context"
	"testing"

	agentmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	skillsvc "github.com/ArtisanCloud/PowerX/internal/service/skills"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	skillmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/skills"
	skillrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/skills"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestReplaceSkillBindingsUsesInjectedReadinessBeforeWriting(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	previous := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = previous })
	require.NoError(t, db.AutoMigrate(&skillmodel.SkillPackageSource{}, &skillmodel.SkillDefinitionDraft{}, &skillmodel.SkillDefinitionRevision{}, &agentmodel.AgentSkillBinding{}))
	tenant, member := uuid.NewString(), uuid.NewString()
	repo := skillrepo.NewSkillDefinitionRepository(db)
	definitions := skillsvc.NewDefinitionService(repo, skillsvc.NewRuntimeDependencyService(db))
	source, err := definitions.CreatePackageSource(context.Background(), skillsvc.CreatePackageSourceInput{TenantUUID: tenant, SourceKind: skillmodel.SkillPackageSourceAgentAuthoring,
		ArtifactURI: "local://fixture/source.tgz", Checksum: "sha256:fixture", ContentType: "application/gzip", StandardManifest: map[string]any{"name": "custom", "description": "custom"}, CreatedByMemberUUID: member})
	require.NoError(t, err)
	draft, _, err := definitions.CreateDraft(context.Background(), skillsvc.CreateDefinitionDraftInput{TenantUUID: tenant, SkillID: "customer.inventory",
		DisplayNameI18n: map[string]string{"zh-CN": "库存"}, DescriptionI18n: map[string]string{"zh-CN": "库存"}, SourceKind: skillmodel.SkillPackageSourceAgentAuthoring,
		PackageSourceUUID: source.UUID.String(), AuthorMemberUUID: member,
		Definition: map[string]any{"schema": skillsvc.SkillDefinitionSchemaV2, "executor": map[string]any{"type": "llm_prompt", "prompt_template_i18n": map[string]any{"zh-CN": "test"}}}})
	require.NoError(t, err)
	_, _, err = definitions.PublishCurrentRevision(context.Background(), skillsvc.PublishDefinitionInput{TenantUUID: tenant, DraftUUID: draft.UUID.String(), ArtifactURI: "local://fixture/published.tgz", Checksum: "sha256:fixture", UpdatedByMemberUUID: member})
	require.NoError(t, err)
	blocked := NewAgentService(db, skillsvc.NewDependencyService(skillsvc.DependencyServiceOptions{}))
	require.ErrorContains(t, blocked.ReplaceSkillBindings(context.Background(), "dev", &tenant, 1, []string{draft.SkillID}), "skill.executor_unavailable")
	var count int64
	require.NoError(t, db.Model(&agentmodel.AgentSkillBinding{}).Count(&count).Error)
	require.Zero(t, count)
	ready := NewAgentService(db)
	require.NoError(t, ready.ReplaceSkillBindings(context.Background(), "dev", &tenant, 1, []string{draft.SkillID}))
	require.ErrorContains(t, blocked.ReplaceSkillBindings(context.Background(), "dev", &tenant, 1, []string{draft.SkillID}), "skill.executor_unavailable")
	var bindings []agentmodel.AgentSkillBinding
	require.NoError(t, db.Find(&bindings).Error)
	require.Len(t, bindings, 1)
	require.True(t, bindings[0].Enabled, "失败不能覆盖已有绑定")
}
