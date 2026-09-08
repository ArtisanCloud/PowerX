package plugin

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"

	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	capmodels "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	setting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestPostgresConcurrentApprovalAndManifestRemoval(t *testing.T) {
	dsn := os.Getenv("POWERX_SESSION_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("postgres_acceptance_not_configured")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(8)
	schema := "px_grants_contract_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	require.NoError(t, db.Exec(`CREATE SCHEMA "`+schema+`"`).Error)
	previous := model.PowerXSchema
	model.PowerXSchema = schema
	t.Cleanup(func() {
		require.NoError(t, db.Exec(`DROP SCHEMA "`+schema+`" CASCADE`).Error)
		model.PowerXSchema = previous
		require.NoError(t, sqlDB.Close())
	})
	for i := 0; i < 2; i++ {
		require.NoError(t, db.AutoMigrate(&capmodels.CapabilityRecord{}, &capmodels.CapabilityRegistration{}, &setting.PluginInstanceConfig{}, &setting.PluginCapabilityApproval{}))
	}
	tenant := uuid.NewString()
	const capability = "com.corex.agent.session.manage"
	const pluginID = "com.powerx.acceptance.grants"
	seedPublishedTenantCapability(t, db, tenant, capability)
	raw, err := json.Marshal(map[string]any{"client_id": pluginID + "." + tenant, "manifest_capabilities": []string{capability}, "allowed_capabilities": []string{capability}})
	require.NoError(t, err)
	require.NoError(t, db.Create(&setting.PluginInstanceConfig{TenantUUID: tenant, PluginID: pluginID, Key: "auth.credentials", Enabled: true, ValueJSON: datatypes.JSON(raw)}).Error)
	svc := NewTenantPluginInstanceService(db)
	admin := reqctx.WithClaims(context.Background(), &reqctx.CoreXClaims{IsRoot: true, UserUUID: uuid.NewString()})
	manifest := plugin_mgr.Manifest{ID: pluginID}
	var wg sync.WaitGroup
	errs := make([]error, 12)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if i%2 == 0 {
				errs[i] = svc.SetIndependentCapabilityApproval(admin, tenant, pluginID, capability, true)
			} else {
				errs[i] = svc.SyncManifestRequiredCapabilities(context.Background(), manifest)
			}
		}(i)
	}
	wg.Wait()
	for _, err := range errs {
		require.NoError(t, err)
	}
	var approvals int64
	require.NoError(t, db.Model(&setting.PluginCapabilityApproval{}).Where("status = ?", "active").Count(&approvals).Error)
	require.EqualValues(t, 1, approvals)
	readAllowed := func() []string {
		var cfg setting.PluginInstanceConfig
		require.NoError(t, db.Where("tenant_uuid = ? AND plugin_id = ?", tenant, pluginID).First(&cfg).Error)
		var doc struct {
			Allowed  []string `json:"allowed_capabilities"`
			Manifest []string `json:"manifest_capabilities"`
		}
		require.NoError(t, json.Unmarshal(cfg.ValueJSON, &doc))
		require.Empty(t, doc.Manifest)
		return doc.Allowed
	}
	require.Equal(t, []string{capability}, readAllowed())
	for i := 0; i < 2; i++ {
		require.NoError(t, svc.SetIndependentCapabilityApproval(admin, tenant, pluginID, capability, false))
		require.NoError(t, svc.SyncManifestRequiredCapabilities(context.Background(), manifest))
	}
	require.Empty(t, readAllowed())
	var approval setting.PluginCapabilityApproval
	require.NoError(t, db.First(&approval).Error)
	require.Equal(t, "revoked", approval.Status)
	require.NotNil(t, approval.RevokedAt)
}
