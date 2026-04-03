package skillsintegration

import (
	"context"
	"testing"

	skillservice "github.com/ArtisanCloud/PowerX/internal/service/skills"
	skillmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/skills"
	skillrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/skills"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

func TestSkillToolCallingUnauthorizedSkillHardFiltered(t *testing.T) {
	db := setupSkillsDB(t)

	require.NoError(t, db.Create(&skillmodel.SkillRegistryRecord{
		SkillID:           "skill.tool.authz.denied",
		Version:           "1.0.0",
		Source:            skillmodel.SkillSourcePlugin,
		Status:            skillmodel.SkillStatusPublished,
		IsLatestPublished: true,
		BundleURI:         "s3://skills/skill.tool.authz.denied-1.0.0.tgz",
		Checksum:          "sha256:tool-authz-denied",
		ImportType:        "upload",
		UpdatedBy:         "seed",
	}).Error)
	require.NoError(t, db.Create(&skillmodel.SkillCapabilityBinding{
		SkillID:         "skill.tool.authz.denied",
		Version:         "1.0.0",
		CapabilityID:    "cap.skill.tool.authz",
		ToolGrants:      datatypes.JSON([]byte(`["orders.write"]`)),
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
	adapterSvc := skillservice.NewAdapterService(invokeSvc, bindingRepo)

	result, err := adapterSvc.InvokeUnified(context.Background(), skillservice.UnifiedInvokeRequest{
		TenantUUID:        "tenant-tool-authz",
		CapabilityID:      "cap.skill.tool.authz",
		PreferredProtocol: "skill",
		ToolGrantIDs:      []string{"orders.read"},
		Payload:           map[string]interface{}{"query": "请执行未授权技能"},
	})
	require.Error(t, err)
	require.Nil(t, result)
	require.ErrorIs(t, err, skillrepo.ErrSkillNotFound)
}
