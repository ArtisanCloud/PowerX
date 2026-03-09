package skillsintegration

import (
	"context"
	"testing"

	skillservice "github.com/ArtisanCloud/PowerX/internal/service/skills"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	skillmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/skills"
	skillrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/skills"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestSkillLifecyclePublishRollbackLatestPointer(t *testing.T) {
	db := setupLifecycleDB(t)
	registryRepo := skillrepo.NewSkillRegistryRepository(db)
	auditRepo := skillrepo.NewSkillLifecycleAuditRepository(db)
	traceRepo := skillrepo.NewSkillExecutionTraceRepository(db)
	auditSvc := skillservice.NewAuditTraceService(traceRepo, auditRepo)
	lifecycleSvc := skillservice.NewLifecycleService(registryRepo, auditSvc)
	ctx := context.Background()

	require.NoError(t, db.Create(&skillmodel.SkillRegistryRecord{
		SkillID:           "skill.integration.lifecycle",
		Version:           "1.0.0",
		Source:            skillmodel.SkillSourcePlugin,
		Status:            skillmodel.SkillStatusPublished,
		IsLatestPublished: true,
		BundleURI:         "s3://skills/skill.integration.lifecycle-1.0.0.tgz",
		Checksum:          "sha256-integration-lifecycle-100",
		ImportType:        "upload",
		UpdatedBy:         "seed",
	}).Error)
	require.NoError(t, db.Create(&skillmodel.SkillRegistryRecord{
		SkillID:           "skill.integration.lifecycle",
		Version:           "2.0.0",
		Source:            skillmodel.SkillSourcePlugin,
		Status:            skillmodel.SkillStatusDraft,
		IsLatestPublished: false,
		BundleURI:         "s3://skills/skill.integration.lifecycle-2.0.0.tgz",
		Checksum:          "sha256-integration-lifecycle-200",
		ImportType:        "upload",
		UpdatedBy:         "seed",
	}).Error)

	require.NoError(t, lifecycleSvc.Publish(ctx, "skill.integration.lifecycle", "2.0.0", "integration", "publish v2"))
	latest, err := registryRepo.GetLatestPublished(ctx, "skill.integration.lifecycle")
	require.NoError(t, err)
	require.Equal(t, "2.0.0", latest.Version)

	require.NoError(t, lifecycleSvc.Rollback(ctx, "skill.integration.lifecycle", "1.0.0", "integration", "rollback to v1"))
	latest, err = registryRepo.GetLatestPublished(ctx, "skill.integration.lifecycle")
	require.NoError(t, err)
	require.Equal(t, "1.0.0", latest.Version)

	audits, err := auditRepo.ListBySkill(ctx, "skill.integration.lifecycle", 10)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(audits), 2)
}

func setupLifecycleDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&parseTime=true&_loc=UTC"), &gorm.Config{})
	require.NoError(t, err)

	prevSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = prevSchema })

	require.NoError(t, db.AutoMigrate(
		&skillmodel.SkillRegistryRecord{},
		&skillmodel.SkillLifecycleAudit{},
		&skillmodel.SkillExecutionTrace{},
	))
	return db
}
