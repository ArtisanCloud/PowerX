package skills

import (
	"context"
	skillmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/skills"
	skillrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/skills"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestMissingDependencyAllowsDraftButBlocksPublishAndBinding(t *testing.T) {
	db := setupDefinitionServiceTestDB(t)
	svc := NewDefinitionService(skillrepo.NewSkillDefinitionRepository(db))
	tenant, member := uuid.NewString(), uuid.NewString()
	source := createAgentAuthoringSource(t, svc, tenant, member)
	definition := evidenceDefinition()
	delete(definition, "tool_dependencies")
	draft, _, err := svc.CreateDraft(context.Background(), CreateDefinitionDraftInput{TenantUUID: tenant, SkillID: "tenant.custom_report", DisplayNameI18n: map[string]string{"en-US": "custom_report"}, DescriptionI18n: map[string]string{"en-US": "custom_report"}, SourceKind: skillmodel.SkillPackageSourceAgentAuthoring, PackageSourceUUID: source.UUID.String(), Definition: definition, AuthorMemberUUID: member})
	require.NoError(t, err)
	require.Error(t, svc.CheckRunnable(context.Background(), tenant, draft.SkillID))
	_, _, err = svc.PublishCurrentRevision(context.Background(), PublishDefinitionInput{TenantUUID: tenant, DraftUUID: draft.UUID.String(), ArtifactURI: "local://skill-sources/custom.tgz", Checksum: "sha256:test", UpdatedByMemberUUID: member})
	require.ErrorContains(t, err, "skill.tool_dependencies_required")
	require.Error(t, svc.CheckRunnable(context.Background(), uuid.NewString(), draft.SkillID))
}
