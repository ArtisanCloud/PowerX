package skillsintegration

import (
	"context"
	"testing"

	skillservice "github.com/ArtisanCloud/PowerX/internal/service/skills"
	skillmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/skills"
	skillrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/skills"
	"github.com/stretchr/testify/require"
)

func TestSkillPublishRejectedWhenChecksumMissingOrMismatch(t *testing.T) {
	db := setupSkillsDB(t)
	registryRepo := skillrepo.NewSkillRegistryRepository(db)
	traceRepo := skillrepo.NewSkillExecutionTraceRepository(db)
	auditRepo := skillrepo.NewSkillLifecycleAuditRepository(db)
	auditSvc := skillservice.NewAuditTraceService(traceRepo, auditRepo)
	lifecycleSvc := skillservice.NewLifecycleService(registryRepo, auditSvc)

	require.NoError(t, db.Create(&skillmodel.SkillRegistryRecord{
		SkillID:    "skill.integrity.missing",
		Version:    "1.0.0",
		Source:     skillmodel.SkillSourcePlugin,
		Status:     skillmodel.SkillStatusDraft,
		BundleURI:  "s3://skills/skill.integrity.missing-1.0.0.tgz",
		Checksum:   "",
		ImportType: "upload",
		UpdatedBy:  "seed",
	}).Error)
	require.NoError(t, db.Create(&skillmodel.SkillRegistryRecord{
		SkillID:    "skill.integrity.mismatch",
		Version:    "1.0.0",
		Source:     skillmodel.SkillSourcePlugin,
		Status:     skillmodel.SkillStatusDraft,
		BundleURI:  "s3://skills/skill.integrity.mismatch-1.0.0.tgz",
		Checksum:   "md5:abc123",
		ImportType: "upload",
		UpdatedBy:  "seed",
	}).Error)

	err := lifecycleSvc.Publish(context.Background(), "skill.integrity.missing", "1.0.0", "integration", "publish")
	require.Error(t, err)
	require.Contains(t, err.Error(), "checksum")

	err = lifecycleSvc.Publish(context.Background(), "skill.integrity.mismatch", "1.0.0", "integration", "publish")
	require.Error(t, err)
	require.Contains(t, err.Error(), "checksum mismatch")
}
