package skillsintegration

import (
	"context"
	"fmt"
	"testing"
	"time"

	skillservice "github.com/ArtisanCloud/PowerX/internal/service/skills"
	skillmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/skills"
	skillrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/skills"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

func TestSkillUnifiedInvokeHardFilterByToolGrants(t *testing.T) {
	db := setupSkillsDB(t)
	require.NoError(t, db.Create(&skillmodel.SkillRegistryRecord{
		SkillID:           "skill.filter.allowed.incident-triage",
		Version:           "1.0.0",
		Source:            skillmodel.SkillSourcePlugin,
		Status:            skillmodel.SkillStatusPublished,
		IsLatestPublished: true,
		BundleURI:         "s3://skills/skill.filter.allowed.incident-triage-1.0.0.tgz",
		Checksum:          "sha256:allowed",
		ImportType:        "upload",
		UpdatedBy:         "seed",
	}).Error)
	require.NoError(t, db.Create(&skillmodel.SkillRegistryRecord{
		SkillID:           "skill.filter.denied.incident-triage",
		Version:           "1.0.0",
		Source:            skillmodel.SkillSourcePlugin,
		Status:            skillmodel.SkillStatusPublished,
		IsLatestPublished: true,
		BundleURI:         "s3://skills/skill.filter.denied.incident-triage-1.0.0.tgz",
		Checksum:          "sha256:denied",
		ImportType:        "upload",
		UpdatedBy:         "seed",
	}).Error)
	require.NoError(t, db.Create(&skillmodel.SkillCapabilityBinding{
		SkillID:         "skill.filter.allowed.incident-triage",
		Version:         "1.0.0",
		CapabilityID:    "cap.skill.filter",
		ToolGrants:      datatypes.JSON([]byte(`["orders.read"]`)),
		BindingStatus:   "active",
		VisibilityScope: "tenant",
		CreatedBy:       "seed",
		UpdatedBy:       "seed",
	}).Error)
	require.NoError(t, db.Create(&skillmodel.SkillCapabilityBinding{
		SkillID:         "skill.filter.denied.incident-triage",
		Version:         "1.0.0",
		CapabilityID:    "cap.skill.filter",
		ToolGrants:      datatypes.JSON([]byte(`["inventory.write"]`)),
		BindingStatus:   "active",
		VisibilityScope: "tenant",
		CreatedBy:       "seed",
		UpdatedBy:       "seed",
	}).Error)

	registryRepo := skillrepo.NewSkillRegistryRepository(db)
	bindingRepo := skillrepo.NewSkillCapabilityBindingRepository(db)
	traceRepo := skillrepo.NewSkillExecutionTraceRepository(db)
	auditRepo := skillrepo.NewSkillLifecycleAuditRepository(db)
	invokeSvc := skillservice.NewInvokeService(registryRepo, skillservice.NewAuditTraceService(traceRepo, auditRepo))
	adapter := skillservice.NewAdapterService(invokeSvc, bindingRepo)
	result, err := adapter.InvokeUnified(context.Background(), skillservice.UnifiedInvokeRequest{
		TenantUUID:        "tenant-match",
		CapabilityID:      "cap.skill.filter",
		PreferredProtocol: "skill",
		ToolGrantIDs:      []string{"orders.read"},
		Payload: map[string]interface{}{
			"query": "allowed incident triage please",
		},
	})
	require.NoError(t, err)
	require.Equal(t, "skill.filter.allowed.incident-triage", result.SkillID)
}

func TestSkillMatchingScaleAvoidLinearMainPath(t *testing.T) {
	db := setupSkillsDB(t)
	now := time.Now().UTC()

	const candidateCount = 10000
	for i := 0; i < candidateCount; i++ {
		skillID := fmt.Sprintf("skill.scale.%05d.incident-triage", i)
		rec := &skillmodel.SkillRegistryRecord{
			SkillID:           skillID,
			Version:           "1.0.0",
			Source:            skillmodel.SkillSourcePlugin,
			Status:            skillmodel.SkillStatusPublished,
			IsLatestPublished: true,
			BundleURI:         "s3://skills/" + skillID + "-1.0.0.tgz",
			Checksum:          "sha256:" + skillID,
			ImportType:        "upload",
			UpdatedBy:         "seed",
		}
		require.NoError(t, db.Create(rec).Error)
		require.NoError(t, db.Model(&skillmodel.SkillRegistryRecord{}).
			Where("skill_id = ? AND version = ?", skillID, "1.0.0").
			Update("updated_at", now.Add(time.Duration(i)*time.Millisecond)).Error)
		require.NoError(t, db.Create(&skillmodel.SkillCapabilityBinding{
			SkillID:         skillID,
			Version:         "1.0.0",
			CapabilityID:    "cap.skill.scale",
			ToolGrants:      datatypes.JSON([]byte(`["scale.read"]`)),
			BindingStatus:   "active",
			VisibilityScope: "tenant",
			CreatedBy:       "seed",
			UpdatedBy:       "seed",
		}).Error)
	}

	registryRepo := skillrepo.NewSkillRegistryRepository(db)
	bindingRepo := skillrepo.NewSkillCapabilityBindingRepository(db)
	traceRepo := skillrepo.NewSkillExecutionTraceRepository(db)
	auditRepo := skillrepo.NewSkillLifecycleAuditRepository(db)
	invokeSvc := skillservice.NewInvokeService(registryRepo, skillservice.NewAuditTraceService(traceRepo, auditRepo))
	adapter := skillservice.NewAdapterService(invokeSvc, bindingRepo)
	start := time.Now()
	result, err := adapter.InvokeUnified(context.Background(), skillservice.UnifiedInvokeRequest{
		TenantUUID:        "tenant-scale",
		CapabilityID:      "cap.skill.scale",
		PreferredProtocol: "skill",
		ToolGrantIDs:      []string{"scale.read"},
		Payload: map[string]interface{}{
			"query": "skill scale incident triage",
		},
	})
	cost := time.Since(start)

	require.NoError(t, err)
	require.NotEmpty(t, result.SkillID)
	// The matching path must be bounded (DB limit + top-k rerank), not linear scan over all 10k rows.
	require.Less(t, cost, 2*time.Second)
}
