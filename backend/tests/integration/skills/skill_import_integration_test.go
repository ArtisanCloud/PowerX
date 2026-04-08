package skillsintegration

import (
	"context"
	"testing"

	skillservice "github.com/ArtisanCloud/PowerX/internal/service/skills"
	skillmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/skills"
	skillrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/skills"
	"github.com/stretchr/testify/require"
)

func TestSkillImportDraftWithMetadata(t *testing.T) {
	db := setupSkillsDB(t)
	registryRepo := skillrepo.NewSkillRegistryRepository(db)
	traceRepo := skillrepo.NewSkillExecutionTraceRepository(db)
	auditRepo := skillrepo.NewSkillLifecycleAuditRepository(db)
	auditSvc := skillservice.NewAuditTraceService(traceRepo, auditRepo)
	importSvc := skillservice.NewImportService(registryRepo, auditSvc)

	record, err := importSvc.ImportDraft(context.Background(), skillservice.ImportRequest{
		SkillID:    "skill.import.integration",
		Version:    "1.0.0",
		Source:     skillmodel.SkillSourceThirdParty,
		BundleURI:  "s3://skills/skill.import.integration-1.0.0.tgz",
		Checksum:   "sha256:7f0f4e8f7f0f4e8f7f0f4e8f7f0f4e8f7f0f4e8f7f0f4e8f7f0f4e8f7f0f4e8f",
		SourceURL:  "https://github.com/example/skills",
		SourceRef:  "refs/tags/v1.0.0",
		Operator:   "integration-user",
		ImportType: skillservice.ImportTypeUpload,
	})
	require.NoError(t, err)
	require.Equal(t, skillmodel.SkillStatusDraft, record.Status)
	require.Equal(t, "https://github.com/example/skills", record.SourceURL)
	require.Equal(t, "refs/tags/v1.0.0", record.SourceRef)
	require.Equal(t, skillservice.ImportTypeUpload, record.ImportType)

	saved, err := registryRepo.GetBySkillVersion(context.Background(), "skill.import.integration", "1.0.0")
	require.NoError(t, err)
	require.Equal(t, skillmodel.SkillStatusDraft, saved.Status)

	audits, err := auditRepo.ListBySkill(context.Background(), "skill.import.integration", 10)
	require.NoError(t, err)
	require.NotEmpty(t, audits)
	require.Equal(t, "import", audits[0].Action)
}
