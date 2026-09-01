package skills

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	skillmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/skills"
	skillrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/skills"
)

func TestDefinitionService_CreatePublishAndAppendStrictly(t *testing.T) {
	db := setupDefinitionServiceTestDB(t)
	svc := NewDefinitionService(skillrepo.NewSkillDefinitionRepository(db))
	tenantUUID := uuid.NewString()
	memberUUID := uuid.NewString()
	source := createAgentAuthoringSource(t, svc, tenantUUID, memberUUID)

	draft, revision, err := svc.CreateDraft(context.Background(), CreateDefinitionDraftInput{
		TenantUUID:        tenantUUID,
		SkillID:           "tenant.activity-review",
		DisplayNameI18n:   map[string]string{"zh-CN": "活动复盘"},
		DescriptionI18n:   map[string]string{"zh-CN": "输出结构化复盘"},
		SourceKind:        skillmodel.SkillPackageSourceAgentAuthoring,
		PackageSourceUUID: source.UUID.String(),
		Definition:        llmPromptDefinition("请根据输入生成 JSON。"),
		ChangeSummary:     "initial",
		AuthorMemberUUID:  memberUUID,
	})
	require.NoError(t, err)
	require.Equal(t, skillmodel.SkillDefinitionDraftStatusDraft, draft.Status)
	require.Equal(t, revision.UUID.String(), draft.CurrentRevisionUUID)

	publishedDraft, publishedRevision, err := svc.PublishCurrentRevision(context.Background(), PublishDefinitionInput{
		TenantUUID:          tenantUUID,
		DraftUUID:           draft.UUID.String(),
		ArtifactURI:         "s3://powerx-skills/" + tenantUUID + "/tenant.activity-review/1.0.0.tgz",
		Checksum:            "sha256:published-snapshot",
		UpdatedByMemberUUID: memberUUID,
	})
	require.NoError(t, err)
	require.Equal(t, skillmodel.SkillDefinitionDraftStatusPublished, publishedDraft.Status)
	require.Equal(t, skillmodel.SkillDefinitionRevisionStatusPublished, publishedRevision.Status)

	_, successor, err := svc.AppendRevision(context.Background(), tenantUUID, draft.UUID.String(), memberUUID, llmPromptDefinition("new"), "successor", "")
	require.NoError(t, err)
	require.Equal(t, skillmodel.SkillDefinitionRevisionStatusDraft, successor.Status)
	require.Greater(t, successor.RevisionNumber, publishedRevision.RevisionNumber)
	_, successor, err = svc.PublishCurrentRevision(context.Background(), PublishDefinitionInput{
		TenantUUID: tenantUUID, DraftUUID: draft.UUID.String(),
		ArtifactURI: "s3://powerx-skills/" + tenantUUID + "/tenant.activity-review/2.0.0.tgz",
		Checksum:    "sha256:successor-snapshot", UpdatedByMemberUUID: memberUUID,
	})
	require.NoError(t, err)
	require.Equal(t, skillmodel.SkillDefinitionRevisionStatusPublished, successor.Status)
}

func TestDefinitionService_ExternalPackageRequiresFrozenObjectAndPackageSource(t *testing.T) {
	db := setupDefinitionServiceTestDB(t)
	svc := NewDefinitionService(skillrepo.NewSkillDefinitionRepository(db))
	tenantUUID := uuid.NewString()
	memberUUID := uuid.NewString()

	_, err := svc.CreatePackageSource(context.Background(), CreatePackageSourceInput{
		TenantUUID:          tenantUUID,
		SourceKind:          skillmodel.SkillPackageSourceExternalImport,
		ArtifactURI:         "file:///tmp/skill.tgz",
		Checksum:            "sha256:source",
		ContentType:         "application/gzip",
		StandardManifest:    map[string]any{"name": "activity-review", "description": "review"},
		CreatedByMemberUUID: memberUUID,
	})
	require.ErrorContains(t, err, "skill.package_object_uri_required")

	source, err := svc.CreatePackageSource(context.Background(), CreatePackageSourceInput{
		TenantUUID:          tenantUUID,
		SourceKind:          skillmodel.SkillPackageSourceExternalImport,
		ArtifactURI:         "local://skill-sources/imports/activity-review.tgz",
		Checksum:            "sha256:source",
		ContentType:         "application/gzip",
		StandardManifest:    map[string]any{"name": "activity-review", "description": "review"},
		CreatedByMemberUUID: memberUUID,
	})
	require.NoError(t, err)

	_, _, err = svc.CreateDraft(context.Background(), CreateDefinitionDraftInput{
		TenantUUID:         tenantUUID,
		SkillID:            "tenant.activity-review-import",
		DisplayNameI18n:    map[string]string{"zh-CN": "活动复盘导入"},
		DescriptionI18n:    map[string]string{"zh-CN": "导入"},
		SourceKind:         skillmodel.SkillPackageSourceExternalImport,
		PackageSourceUUID:  source.UUID.String(),
		Definition:         map[string]any{"schema": SkillDefinitionSchemaV2, "executor": map[string]any{"type": "instruction_only"}},
		AuthorMemberUUID:   memberUUID,
		InitialDraftStatus: skillmodel.SkillDefinitionDraftStatusInstructionOnly,
	})
	require.NoError(t, err)
}

func TestDefinitionService_RejectsFreeFormOrIncompleteDefinition(t *testing.T) {
	db := setupDefinitionServiceTestDB(t)
	svc := NewDefinitionService(skillrepo.NewSkillDefinitionRepository(db))
	tenantUUID := uuid.NewString()
	memberUUID := uuid.NewString()
	source := createAgentAuthoringSource(t, svc, tenantUUID, memberUUID)

	_, _, err := svc.CreateDraft(context.Background(), CreateDefinitionDraftInput{
		TenantUUID:        tenantUUID,
		SkillID:           "tenant.invalid-definition",
		DisplayNameI18n:   map[string]string{"zh-CN": "无效"},
		DescriptionI18n:   map[string]string{"zh-CN": "无效"},
		SourceKind:        skillmodel.SkillPackageSourceAgentAuthoring,
		PackageSourceUUID: source.UUID.String(),
		Definition:        map[string]any{"text": "请帮我写一个技能"},
		AuthorMemberUUID:  memberUUID,
	})
	require.ErrorContains(t, err, "skill.definition_schema_invalid")
}

func createAgentAuthoringSource(t *testing.T, svc *DefinitionService, tenantUUID, memberUUID string) *skillmodel.SkillPackageSource {
	t.Helper()
	source, err := svc.CreatePackageSource(context.Background(), CreatePackageSourceInput{
		TenantUUID: tenantUUID, SourceKind: skillmodel.SkillPackageSourceAgentAuthoring,
		ArtifactURI: "s3://powerx-skills/drafts/" + uuid.NewString() + ".tgz", Checksum: "sha256:agent-source",
		ContentType: "application/gzip", StandardManifest: map[string]any{"name": "agent-authored", "description": "structured draft"},
		CreatedByMemberUUID: memberUUID,
	})
	require.NoError(t, err)
	return source
}

func setupDefinitionServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	previousSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = previousSchema })
	require.NoError(t, db.AutoMigrate(
		&skillmodel.SkillPackageSource{},
		&skillmodel.SkillDefinitionDraft{},
		&skillmodel.SkillDefinitionRevision{},
	))
	return db
}

func llmPromptDefinition(prompt string) map[string]any {
	return map[string]any{
		"schema": SkillDefinitionSchemaV2,
		"executor": map[string]any{
			"type":                 "llm_prompt",
			"prompt_template_i18n": map[string]any{"zh-CN": prompt},
		},
	}
}
