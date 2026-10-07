// Package runtime_host 仅为合同测试提供隔离数据库和真实授权记录。
package runtime_host

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	core "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	capm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/runtime_host"
	setting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	tenantmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var Capabilities = []string{"com.corex.runtime.cache.read", "com.corex.runtime.cache.manage", "com.corex.runtime.taskcenter.read", "com.corex.runtime.taskcenter.manage"}

func Database(t *testing.T) *gorm.DB {
	t.Helper()
	old := core.PowerXSchema
	t.Cleanup(func() { core.PowerXSchema = old })
	cfg := &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)}
	var db *gorm.DB
	var e error
	if dsn := os.Getenv("POWERX_RUNTIME_HOST_TEST_POSTGRES_DSN"); dsn != "" {
		core.PowerXSchema = "contract_runtime_host_" + uuid.New().String()[:8]
		db, e = gorm.Open(postgres.Open(dsn), cfg)
		require.NoError(t, e)
		sqlDB, e := db.DB()
		require.NoError(t, e)
		t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
		schema := core.PowerXSchema
		require.NoError(t, db.Exec(`CREATE SCHEMA "`+schema+`"`).Error)
		t.Cleanup(func() { require.NoError(t, db.Exec(`DROP SCHEMA "`+schema+`" CASCADE`).Error) })
	} else {
		core.PowerXSchema = "main"
		db, e = gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), cfg)
		require.NoError(t, e)
		sqlDB, e := db.DB()
		require.NoError(t, e)
		sqlDB.SetMaxOpenConns(1)
		t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	}
	require.NoError(t, db.AutoMigrate(&m.Subject{}, &m.Task{}, &m.Operation{}, &capm.CapabilityRecord{}, &capm.CapabilityRegistration{}, &setting.PluginInstanceConfig{}, &tenantmodel.Tenant{}))
	require.NoError(t, db.AutoMigrate(&m.Subject{}, &m.Task{}, &m.Operation{}))
	for _, id := range Capabilities {
		require.NoError(t, db.Create(&capm.CapabilityRecord{CapabilityID: id, PluginID: "core", PluginVersion: "1", Title: id, CapabilitiesHash: id, ProtocolHash: id, Status: "published"}).Error)
	}
	return db
}
func Actor(t *testing.T, db *gorm.DB, tenant, plugin string) (context.Context, *setting.PluginInstanceConfig) {
	t.Helper()
	var tenantCount int64
	require.NoError(t, db.Model(&tenantmodel.Tenant{}).Where("uuid = ?", tenant).Count(&tenantCount).Error)
	if tenantCount == 0 {
		require.NoError(t, db.Create(&tenantmodel.Tenant{PowerUUIDModel: core.PowerUUIDModel{UUID: uuid.MustParse(tenant)}, Key: tenant, Name: tenant, Status: 1}).Error)
	}
	for _, id := range Capabilities {
		var n int64
		require.NoError(t, db.Model(&capm.CapabilityRegistration{}).Where("tenant_uuid = ? AND capability_id = ?", tenant, id).Count(&n).Error)
		if n == 0 {
			require.NoError(t, db.Create(&capm.CapabilityRegistration{CapabilityID: id, TenantUUID: tenant, ContractRef: "v1", Status: "published", Version: 1, RoutingPolicyID: uuid.New()}).Error)
		}
	}
	raw, _ := json.Marshal(map[string]any{"client_id": plugin, "allowed_capabilities": Capabilities})
	row := &setting.PluginInstanceConfig{TenantUUID: tenant, PluginID: plugin, Key: "auth.credentials", Enabled: true, ValueJSON: datatypes.JSON(raw)}
	require.NoError(t, db.Create(row).Error)
	claims := &reqctx.CoreXClaims{TenantUUID: tenant, PluginID: plugin, Scope: "access", RegisteredClaims: jwt.RegisteredClaims{Issuer: "powerx-sts", Subject: "client:" + plugin, Audience: jwt.ClaimStrings{"powerx:api"}, ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}}
	return reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), tenant), claims), row
}
func Grant(t *testing.T, db *gorm.DB, row *setting.PluginInstanceConfig, ids ...string) {
	t.Helper()
	raw, _ := json.Marshal(map[string]any{"client_id": row.PluginID, "allowed_capabilities": ids})
	require.NoError(t, db.Model(row).Update("value_json", datatypes.JSON(raw)).Error)
}
