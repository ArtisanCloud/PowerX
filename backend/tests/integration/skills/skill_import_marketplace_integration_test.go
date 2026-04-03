package skillsintegration

import (
	"context"
	"testing"

	skillservice "github.com/ArtisanCloud/PowerX/internal/service/skills"
	skillmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/skills"
	skillrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/skills"
	"github.com/stretchr/testify/require"
)

type fakeMarketplaceResolver struct{}

func (r *fakeMarketplaceResolver) Resolve(ctx context.Context, req skillservice.ImportRequest) (*skillservice.SkillSourceResolved, error) {
	return &skillservice.SkillSourceResolved{
		SkillMarkdown: `---
name: Prompt Template Renderer
description: Render template with variables
version: 1.0.0
entrypoints:
  - runbook.default
---
`,
		BundleURI: "https://github.com/anthropics/skills/tree/main/prompt-template",
		SourceRef: "main",
	}, nil
}

func TestSkillImportMarketplaceFromResolver(t *testing.T) {
	db := setupSkillsDB(t)
	registryRepo := skillrepo.NewSkillRegistryRepository(db)
	traceRepo := skillrepo.NewSkillExecutionTraceRepository(db)
	auditRepo := skillrepo.NewSkillLifecycleAuditRepository(db)
	auditSvc := skillservice.NewAuditTraceService(traceRepo, auditRepo)
	importSvc := skillservice.NewImportService(registryRepo, auditSvc).
		WithSourceResolver(&fakeMarketplaceResolver{})

	record, err := importSvc.ImportDraft(context.Background(), skillservice.ImportRequest{
		SkillID:    "skill.thirdparty.prompt-template",
		Version:    "1.0.0",
		Source:     skillmodel.SkillSourceThirdParty,
		Checksum:   "sha256:7f0f4e8f7f0f4e8f7f0f4e8f7f0f4e8f7f0f4e8f7f0f4e8f7f0f4e8f7f0f4e8f",
		SourceURL:  "https://github.com/anthropics/skills",
		Operator:   "integration-user",
		ImportType: skillservice.ImportTypeMarketplace,
	})
	require.NoError(t, err)
	require.Equal(t, skillmodel.SkillStatusDraft, record.Status)
	require.Equal(t, "https://github.com/anthropics/skills/tree/main/prompt-template", record.BundleURI)
	require.Equal(t, "main", record.SourceRef)
	require.NotEmpty(t, record.ManifestJSON)
}
