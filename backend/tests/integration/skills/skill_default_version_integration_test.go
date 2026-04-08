package skillsintegration

import (
	"context"
	"testing"

	skillservice "github.com/ArtisanCloud/PowerX/internal/service/skills"
	skillmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/skills"
	skillrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/skills"
	"github.com/stretchr/testify/require"
)

func TestSkillInvokeDefaultVersionUsesLatestPublished(t *testing.T) {
	db := setupSkillsDB(t)
	require.NoError(t, db.Create(&skillmodel.SkillRegistryRecord{
		SkillID:           "skill.default.version",
		Version:           "1.0.0",
		Source:            skillmodel.SkillSourcePlugin,
		Status:            skillmodel.SkillStatusPublished,
		IsLatestPublished: false,
		BundleURI:         "s3://skills/skill.default.version-1.0.0.tgz",
		Checksum:          "sha256:default-v1",
		ImportType:        "upload",
		UpdatedBy:         "seed",
	}).Error)
	require.NoError(t, db.Create(&skillmodel.SkillRegistryRecord{
		SkillID:           "skill.default.version",
		Version:           "2.0.0",
		Source:            skillmodel.SkillSourcePlugin,
		Status:            skillmodel.SkillStatusPublished,
		IsLatestPublished: true,
		BundleURI:         "s3://skills/skill.default.version-2.0.0.tgz",
		Checksum:          "sha256:default-v2",
		ImportType:        "upload",
		UpdatedBy:         "seed",
	}).Error)

	registryRepo := skillrepo.NewSkillRegistryRepository(db)
	traceRepo := skillrepo.NewSkillExecutionTraceRepository(db)
	auditRepo := skillrepo.NewSkillLifecycleAuditRepository(db)
	invokeSvc := skillservice.NewInvokeService(registryRepo, skillservice.NewAuditTraceService(traceRepo, auditRepo))

	resolved, err := invokeSvc.Resolve(context.Background(), skillservice.InvokeRequest{
		TenantUUID: "tenant-us3-default",
		SkillID:    "skill.default.version",
		Version:    "",
		InvokePath: "tenant.skills.invoke",
	})
	require.NoError(t, err)
	require.Equal(t, "2.0.0", resolved.Version)
	require.NotEmpty(t, resolved.TraceID)
}
