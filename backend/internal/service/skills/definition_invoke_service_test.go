package skills

import (
	"context"
	"testing"

	skillmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/skills"
	skillrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/skills"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestDefinitionInvokeService_UsesPublishedRevisionOnly(t *testing.T) {
	db := setupDefinitionServiceTestDB(t)
	tenantUUID := uuid.NewString()
	memberUUID := uuid.NewString()
	definitions := NewDefinitionService(skillrepo.NewSkillDefinitionRepository(db))
	source := createAgentAuthoringSource(t, definitions, tenantUUID, memberUUID)
	draft, _, err := definitions.CreateDraft(context.Background(), CreateDefinitionDraftInput{
		TenantUUID: tenantUUID, SkillID: "tenant.generic-review", DisplayNameI18n: map[string]string{"zh-CN": "通用复盘"},
		DescriptionI18n: map[string]string{"zh-CN": "描述"}, SourceKind: skillmodel.SkillPackageSourceAgentAuthoring,
		PackageSourceUUID: source.UUID.String(),
		Definition:        llmPromptDefinition("请生成复盘"), AuthorMemberUUID: memberUUID,
	})
	require.NoError(t, err)

	runtime := NewDefinitionInvokeService(skillrepo.NewSkillDefinitionRepository(db), NewManifestExecutor(ManifestExecutorOptions{
		LLM: func(_ context.Context, _ ManifestLLMInvocation) (string, error) {
			return "# 复盘\n\n- 已完成", nil
		},
	}), nil)
	_, err = runtime.Execute(context.Background(), InvokeRequest{TenantUUID: tenantUUID, SkillID: draft.SkillID}, nil, nil)
	require.ErrorContains(t, err, "skill.definition_not_published")

	_, _, err = definitions.PublishCurrentRevision(context.Background(), PublishDefinitionInput{
		TenantUUID: tenantUUID, DraftUUID: draft.UUID.String(), ArtifactURI: "s3://powerx-skills/review.tgz",
		Checksum: "sha256:review", UpdatedByMemberUUID: memberUUID,
	})
	require.NoError(t, err)

	result, err := runtime.Execute(context.Background(), InvokeRequest{TenantUUID: tenantUUID, SkillID: draft.SkillID, TraceID: uuid.NewString()}, map[string]any{"campaign": "summer"}, map[string]any{"locale": "zh-CN"})
	require.NoError(t, err)
	require.Equal(t, "completed", result.Status)
	require.Equal(t, "# 复盘\n\n- 已完成", result.Result["content"])

	_, err = runtime.Execute(context.Background(), InvokeRequest{TenantUUID: tenantUUID, SkillID: draft.SkillID, Version: uuid.NewString()}, nil, map[string]any{"locale": "zh-CN"})
	require.ErrorContains(t, err, "skill.definition_revision_not_current")
}
