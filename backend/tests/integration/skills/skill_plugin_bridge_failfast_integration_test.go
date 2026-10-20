package skillsintegration

import (
	"context"
	"encoding/json"
	"testing"

	skillservice "github.com/ArtisanCloud/PowerX/internal/service/skills"
	skillmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/skills"
	skillrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/skills"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

// 旧 Registry transport 必须显式要求迁移，不恢复已删除的插件专用 executor。
func TestPluginSkillBridgeLegacyTransportRequiresDefinitionUpgrade(t *testing.T) {
	db := setupSkillsDB(t)
	raw, err := json.Marshal(pluginManifestFixture("customer.parcel.review", "com.powerx.plugins.base.local"))
	require.NoError(t, err)
	require.NoError(t, db.Create(&skillmodel.SkillRegistryRecord{SkillID: "customer.parcel.review", Version: "1.0.0", Source: skillmodel.SkillSourcePlugin, Status: skillmodel.SkillStatusPublished, IsLatestPublished: true, BundleURI: "plugin://com.powerx.plugins.base.local/customer.parcel.review/1.0.0", Checksum: "sha256:fixture", ManifestJSON: datatypes.JSON(raw), ImportType: "upload", UpdatedBy: "fixture"}).Error)
	traces := skillrepo.NewSkillExecutionTraceRepository(db)
	service := skillservice.NewInvokeService(skillrepo.NewSkillRegistryRepository(db), skillservice.NewAuditTraceService(traces, skillrepo.NewSkillLifecycleAuditRepository(db)))
	_, err = service.Execute(context.Background(), skillservice.InvokeRequest{TenantUUID: uuid.NewString(), SkillID: "customer.parcel.review", TraceID: "legacy_requires_upgrade", InvokePath: "tenant.skills.invoke"}, map[string]any{"length_cm": 10}, nil)
	require.ErrorIs(t, err, skillservice.ErrDefinitionContractUpgradeRequired)
	trace, err := traces.GetByTraceID(context.Background(), "legacy_requires_upgrade")
	require.NoError(t, err)
	require.Equal(t, "failed", trace.Status)
	require.Equal(t, skillservice.ErrDefinitionContractUpgradeRequired.Error(), trace.ErrorCode)
	code, envelope := skillservice.MapInvokeError(skillservice.ErrDefinitionContractUpgradeRequired)
	require.Equal(t, 409, code)
	require.Equal(t, skillservice.ErrDefinitionContractUpgradeRequired.Error(), envelope.Code)
}
