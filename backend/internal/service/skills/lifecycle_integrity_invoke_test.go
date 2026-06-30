package skills

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	skillmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/skills"
	skillrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/skills"
)

func setupSkillsServiceTestDB(t *testing.T) *gorm.DB {
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
		&skillmodel.SkillCapabilityBinding{},
		&skillmodel.OfficialSkillCatalogEntry{},
	))
	return db
}

func TestLifecycleService_PublishRollbackStateMachine(t *testing.T) {
	db := setupSkillsServiceTestDB(t)
	require.NoError(t, db.Create(&skillmodel.SkillRegistryRecord{
		SkillID:           "skill.lifecycle",
		Version:           "1.0.0",
		Source:            skillmodel.SkillSourcePlugin,
		Status:            skillmodel.SkillStatusPublished,
		IsLatestPublished: true,
		BundleURI:         "s3://skills/skill.lifecycle-1.0.0.tgz",
		Checksum:          "sha256:lifecycle-v1",
		ImportType:        ImportTypeUpload,
		UpdatedBy:         "seed",
	}).Error)
	require.NoError(t, db.Create(&skillmodel.SkillRegistryRecord{
		SkillID:           "skill.lifecycle",
		Version:           "2.0.0",
		Source:            skillmodel.SkillSourcePlugin,
		Status:            skillmodel.SkillStatusDraft,
		IsLatestPublished: false,
		BundleURI:         "s3://skills/skill.lifecycle-2.0.0.tgz",
		Checksum:          "sha256:lifecycle-v2",
		ImportType:        ImportTypeUpload,
		UpdatedBy:         "seed",
	}).Error)
	require.NoError(t, db.Create(&skillmodel.SkillRegistryRecord{
		SkillID:           "skill.lifecycle",
		Version:           "3.0.0",
		Source:            skillmodel.SkillSourcePlugin,
		Status:            skillmodel.SkillStatusDisabled,
		IsLatestPublished: false,
		BundleURI:         "s3://skills/skill.lifecycle-3.0.0.tgz",
		Checksum:          "sha256:lifecycle-v3",
		ImportType:        ImportTypeUpload,
		UpdatedBy:         "seed",
	}).Error)

	registryRepo := skillrepo.NewSkillRegistryRepository(db)
	lifecycleSvc := NewLifecycleService(registryRepo, nil)

	err := lifecycleSvc.Publish(context.Background(), "skill.lifecycle", "2.0.0", "operator-a", "approve")
	require.NoError(t, err)
	v1AfterPublish, err := registryRepo.GetBySkillVersion(context.Background(), "skill.lifecycle", "1.0.0")
	require.NoError(t, err)
	v2AfterPublish, err := registryRepo.GetBySkillVersion(context.Background(), "skill.lifecycle", "2.0.0")
	require.NoError(t, err)
	require.False(t, v1AfterPublish.IsLatestPublished)
	require.True(t, v2AfterPublish.IsLatestPublished)
	require.Equal(t, skillmodel.SkillStatusPublished, v2AfterPublish.Status)

	err = lifecycleSvc.Rollback(context.Background(), "skill.lifecycle", "1.0.0", "operator-b", "rollback")
	require.NoError(t, err)
	v1AfterRollback, err := registryRepo.GetBySkillVersion(context.Background(), "skill.lifecycle", "1.0.0")
	require.NoError(t, err)
	v2AfterRollback, err := registryRepo.GetBySkillVersion(context.Background(), "skill.lifecycle", "2.0.0")
	require.NoError(t, err)
	require.True(t, v1AfterRollback.IsLatestPublished)
	require.False(t, v2AfterRollback.IsLatestPublished)

	err = lifecycleSvc.Publish(context.Background(), "skill.lifecycle", "3.0.0", "operator-c", "should-fail")
	require.ErrorContains(t, err, "disabled skill version cannot be published")
}

func TestImportServicePublishLatestMarksPluginSkillPublished(t *testing.T) {
	db := setupSkillsServiceTestDB(t)
	registryRepo := skillrepo.NewSkillRegistryRepository(db)
	importSvc := NewImportService(registryRepo, nil)

	_, err := importSvc.ImportDraft(context.Background(), ImportRequest{
		SkillID:    "powerxplugin.template.basic",
		Version:    "1.0.0",
		Source:     skillmodel.SkillSourcePlugin,
		BundleURI:  "plugin-registry://com.powerx.plugins.base/powerxplugin.template.basic/1.0.0",
		Checksum:   "sha256:plugin-template-basic",
		Manifest:   []byte(`{"skill_id":"powerxplugin.template.basic"}`),
		Operator:   "plugin-sync",
		ImportType: ImportTypeUpload,
	})
	require.NoError(t, err)

	require.NoError(t, importSvc.PublishLatest(context.Background(), "powerxplugin.template.basic", "1.0.0", "plugin-sync", "plugin registry sync"))

	rec, err := registryRepo.GetLatestPublished(context.Background(), "powerxplugin.template.basic")
	require.NoError(t, err)
	require.Equal(t, skillmodel.SkillStatusPublished, rec.Status)
	require.True(t, rec.IsLatestPublished)
}

func TestImportDraftDoesNotDemotePublishedPluginSkill(t *testing.T) {
	db := setupSkillsServiceTestDB(t)
	registryRepo := skillrepo.NewSkillRegistryRepository(db)
	importSvc := NewImportService(registryRepo, nil)

	req := ImportRequest{
		SkillID:    "powerxplugin.template.basic",
		Version:    "1.0.0",
		Source:     skillmodel.SkillSourcePlugin,
		BundleURI:  "plugin-registry://com.powerx.plugins.base/powerxplugin.template.basic/1.0.0",
		Checksum:   "sha256:plugin-template-basic",
		Manifest:   []byte(`{"skill_id":"powerxplugin.template.basic","title":"模板对象基础能力"}`),
		Operator:   "plugin-sync",
		ImportType: ImportTypeUpload,
	}
	_, err := importSvc.ImportDraft(context.Background(), req)
	require.NoError(t, err)
	require.NoError(t, importSvc.PublishLatest(context.Background(), req.SkillID, req.Version, "plugin-sync", "plugin registry sync"))

	req.Manifest = []byte(`{"skill_id":"powerxplugin.template.basic","title":"模板对象基础能力 updated"}`)
	_, err = importSvc.ImportDraft(context.Background(), req)
	require.NoError(t, err)

	rec, err := registryRepo.GetLatestPublished(context.Background(), req.SkillID)
	require.NoError(t, err)
	require.Equal(t, skillmodel.SkillStatusPublished, rec.Status)
	require.True(t, rec.IsLatestPublished)
	require.Contains(t, string(rec.ManifestJSON), "updated")
}

func TestImportPluginPublishedCreatesPublishedRecordDirectly(t *testing.T) {
	db := setupSkillsServiceTestDB(t)
	registryRepo := skillrepo.NewSkillRegistryRepository(db)
	importSvc := NewImportService(registryRepo, nil)

	rec, err := importSvc.ImportPluginPublished(context.Background(), ImportRequest{
		SkillID:    "powerxplugin.template.basic",
		Version:    "1.0.0",
		Source:     skillmodel.SkillSourcePlugin,
		BundleURI:  "plugin-registry://com.powerx.plugins.base/powerxplugin.template.basic/1.0.0",
		Checksum:   "sha256:plugin-template-basic",
		Manifest:   []byte(`{"skill_id":"powerxplugin.template.basic","title":"模板对象基础能力"}`),
		Operator:   "plugin-sync",
		ImportType: ImportTypeUpload,
	}, "plugin registry sync")
	require.NoError(t, err)
	require.Equal(t, skillmodel.SkillStatusPublished, rec.Status)
	require.True(t, rec.IsLatestPublished)
	require.NotNil(t, rec.PublishedAt)

	latest, err := registryRepo.GetLatestPublished(context.Background(), "powerxplugin.template.basic")
	require.NoError(t, err)
	require.Equal(t, rec.ID, latest.ID)
	require.Equal(t, skillmodel.SkillStatusPublished, latest.Status)
}

func TestImportPluginPublishedCreatesCapabilityBindings(t *testing.T) {
	db := setupSkillsServiceTestDB(t)
	registryRepo := skillrepo.NewSkillRegistryRepository(db)
	bindingRepo := skillrepo.NewSkillCapabilityBindingRepository(db)
	importSvc := NewImportService(registryRepo, nil).
		WithCapabilityBindingRepository(bindingRepo)

	_, err := importSvc.ImportPluginPublished(context.Background(), ImportRequest{
		SkillID:   "court_mate.knowledge.basketball_extraction.local",
		Version:   "1.0.0",
		Source:    skillmodel.SkillSourcePlugin,
		BundleURI: "plugin-registry://com.powerx.plugins.court-mate.local/court_mate.knowledge.basketball_extraction.local/1.0.0",
		Checksum:  "sha256:plugin-court-mate-knowledge",
		Manifest: []byte(`{
			"skill_id":"court_mate.knowledge.basketball_extraction.local",
			"title":"篮球训练知识抽取",
			"description":"从文档抽取篮球训练知识候选",
			"intent_examples":["抽取训练动作"],
			"executor":{
				"type":"capability",
				"prepare_capability":"com.powerx.plugins.court-mate.local.knowledge.extract.prepare",
				"action_map":{
					"extract":"com.powerx.plugins.court-mate.local.knowledge.extract_basketball_training"
				}
			}
		}`),
		Operator:   "plugin-sync",
		ImportType: ImportTypeUpload,
	}, "plugin registry sync")
	require.NoError(t, err)

	bindings, err := bindingRepo.ListBySkillVersion(context.Background(), "court_mate.knowledge.basketball_extraction.local", "1.0.0")
	require.NoError(t, err)
	require.Len(t, bindings, 2)
	capabilities := map[string]bool{}
	for _, binding := range bindings {
		capabilities[binding.CapabilityID] = true
		require.Equal(t, "active", binding.BindingStatus)
		require.Equal(t, "tenant", binding.VisibilityScope)
		require.Equal(t, skillmodel.SkillSourcePlugin, binding.SourceConstraint)
	}
	require.True(t, capabilities["com.powerx.plugins.court-mate.local.knowledge.extract.prepare"])
	require.True(t, capabilities["com.powerx.plugins.court-mate.local.knowledge.extract_basketball_training"])
}

func TestIntegrityPolicy_ValidateImportAndPublish(t *testing.T) {
	policy := &IntegrityPolicy{RequireChecksum: true, RequireSignature: true}

	err := policy.ValidateImport(ImportRequest{
		SkillID:   "skill.integrity",
		Version:   "1.0.0",
		BundleURI: "s3://skills/skill.integrity-1.0.0.tgz",
		Checksum:  "sha256:integrity",
		Signature: "sig-v1",
	})
	require.NoError(t, err)

	err = policy.ValidateImport(ImportRequest{
		SkillID:   "skill.integrity",
		Version:   "1.0.0",
		BundleURI: "s3://skills/skill.integrity-1.0.0.tgz",
		Checksum:  "md5:bad",
		Signature: "sig-v1",
	})
	require.ErrorContains(t, err, "checksum mismatch")

	err = policy.ValidatePublish(&skillmodel.SkillRegistryRecord{
		SkillID:   "skill.integrity",
		Version:   "1.0.0",
		Checksum:  "sha256:integrity",
		Signature: "",
	})
	require.ErrorContains(t, err, "signature is required before publish")
}

func TestInvokeService_ResolveDefaultVersion(t *testing.T) {
	db := setupSkillsServiceTestDB(t)
	require.NoError(t, db.Create(&skillmodel.SkillRegistryRecord{
		SkillID:           "skill.resolve",
		Version:           "1.0.0",
		Source:            skillmodel.SkillSourcePlugin,
		Status:            skillmodel.SkillStatusPublished,
		IsLatestPublished: false,
		BundleURI:         "s3://skills/skill.resolve-1.0.0.tgz",
		Checksum:          "sha256:resolve-v1",
		ImportType:        ImportTypeUpload,
		UpdatedBy:         "seed",
	}).Error)
	require.NoError(t, db.Create(&skillmodel.SkillRegistryRecord{
		SkillID:           "skill.resolve",
		Version:           "2.0.0",
		Source:            skillmodel.SkillSourcePlugin,
		Status:            skillmodel.SkillStatusPublished,
		IsLatestPublished: true,
		BundleURI:         "s3://skills/skill.resolve-2.0.0.tgz",
		Checksum:          "sha256:resolve-v2",
		ImportType:        ImportTypeUpload,
		UpdatedBy:         "seed",
	}).Error)
	require.NoError(t, db.Create(&skillmodel.SkillRegistryRecord{
		SkillID:           "skill.resolve",
		Version:           "3.0.0",
		Source:            skillmodel.SkillSourcePlugin,
		Status:            skillmodel.SkillStatusDraft,
		IsLatestPublished: false,
		BundleURI:         "s3://skills/skill.resolve-3.0.0.tgz",
		Checksum:          "sha256:resolve-v3",
		ImportType:        ImportTypeUpload,
		UpdatedBy:         "seed",
	}).Error)

	registryRepo := skillrepo.NewSkillRegistryRepository(db)
	invokeSvc := NewInvokeService(registryRepo, nil)

	resolved, err := invokeSvc.Resolve(context.Background(), InvokeRequest{
		TenantUUID: "tenant-unit",
		SkillID:    "skill.resolve",
		InvokePath: "tenant.skills.invoke",
	})
	require.NoError(t, err)
	require.Equal(t, "2.0.0", resolved.Version)
	require.NotEmpty(t, resolved.TraceID)

	_, err = invokeSvc.Resolve(context.Background(), InvokeRequest{
		TenantUUID: "tenant-unit",
		SkillID:    "skill.resolve",
		Version:    "3.0.0",
		InvokePath: "tenant.skills.invoke",
	})
	require.ErrorContains(t, err, "skill version is not published")
}
