package bootstrap

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/ArtisanCloud/PowerX/internal/infra/plugin/runtimecredential"
	"github.com/ArtisanCloud/PowerX/internal/service/setting"
	tenantsvc "github.com/ArtisanCloud/PowerX/internal/service/tenant"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	settingmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	tenantmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newRuntimeCredentialTestDeps(t *testing.T) (*shared.Deps, *config.Config) {
	t.Helper()
	previous := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = previous })
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&tenantmodel.Tenant{}, &settingmodel.PluginInstanceConfig{}))
	tenant := tenantmodel.Tenant{Key: "system", Name: "System", Type: tenantmodel.TenantTypeSystem, Status: 1}
	tenant.UUID = uuid.MustParse("8421e1d2-e571-432a-8431-273396f3e8a3")
	require.NoError(t, db.Create(&tenant).Error)
	cfg := &config.Config{}
	cfg.Plugin.InstalledDir = t.TempDir()
	return &shared.Deps{DB: db, TenantSvc: tenantsvc.NewTenantService(db, nil)}, cfg
}

func TestRuntimeCredentialSurvivesMissingVersionDirectory(t *testing.T) {
	deps, cfg := newRuntimeCredentialTestDeps(t)
	ctx := context.Background()
	id := "com.powerx.plugins.ai-craft"
	first, err := ensurePluginRuntimeCredential(ctx, deps, cfg, id)
	require.NoError(t, err)
	path, err := runtimecredential.Path(cfg.Plugin.InstalledDir, id)
	require.NoError(t, err)
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	// 没有任何版本目录时仍能从持久凭证恢复，不创建新 hash 或新 secret。
	second, err := ensurePluginRuntimeCredential(ctx, deps, cfg, id)
	require.NoError(t, err)
	require.Equal(t, first.ClientID, second.ClientID)
	require.Equal(t, first.ClientSecret, second.ClientSecret)
	svc := setting.NewPluginInstanceConfigService(deps)
	require.NoError(t, svc.VerifyClient(ctx, first.TenantUUID, id, first.ClientID, second.ClientSecret, "powerx:api", "access", ""))
}

func TestRuntimeCredentialImportsVerifiedOldHostValues(t *testing.T) {
	deps, cfg := newRuntimeCredentialTestDeps(t)
	ctx := context.Background()
	id := "com.powerx.plugins.ai-craft"
	tenantUUID := "8421e1d2-e571-432a-8431-273396f3e8a3"
	svc := setting.NewPluginInstanceConfigService(deps)
	clientID, secret, err := svc.EnsureCredentials(ctx, tenantUUID, id, nil)
	require.NoError(t, err)
	path := filepath.Join(cfg.Plugin.InstalledDir, id, "0.1.75/config/host-values.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	raw, err := yaml.Marshal(map[string]any{"env": map[string]string{"POWERX_STS_CLIENT_ID": clientID, "POWERX_STS_CLIENT_SECRET": secret}})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, raw, 0o600))
	cred, err := ensurePluginRuntimeCredential(ctx, deps, cfg, id)
	require.NoError(t, err)
	require.Equal(t, secret, cred.ClientSecret)
	durablePath, err := runtimecredential.Path(cfg.Plugin.InstalledDir, id)
	require.NoError(t, err)
	saved, err := runtimecredential.Read(durablePath)
	require.NoError(t, err)
	require.Equal(t, secret, saved.ClientSecret)
}

func TestRuntimeCredentialMissingIDReportsMissingFieldAndDoesNotRotate(t *testing.T) {
	deps, cfg := newRuntimeCredentialTestDeps(t)
	ctx := context.Background()
	id := "com.powerx.plugins.ai-craft"
	tenantUUID := "8421e1d2-e571-432a-8431-273396f3e8a3"
	svc := setting.NewPluginInstanceConfigService(deps)
	_, secret, err := svc.EnsureCredentials(ctx, tenantUUID, id, nil)
	require.NoError(t, err)
	before, err := svc.Get(ctx, tenantUUID, id, setting.KeyClientCredentials)
	require.NoError(t, err)
	path := filepath.Join(cfg.Plugin.InstalledDir, id, "0.1.75/config/host-values.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	require.NoError(t, os.WriteFile(path, []byte("env: {}\n"), 0o600))
	_, err = ensurePluginRuntimeCredential(ctx, deps, cfg, id)
	require.ErrorContains(t, err, "POWERX_STS_CLIENT_ID missing")
	require.NotContains(t, err.Error(), secret)
	after, err := svc.Get(ctx, tenantUUID, id, setting.KeyClientCredentials)
	require.NoError(t, err)
	require.Equal(t, before.ValueJSON, after.ValueJSON)
}

func TestRuntimeCredentialRejectsForeignTenantDurableCredential(t *testing.T) {
	deps, cfg := newRuntimeCredentialTestDeps(t)
	ctx := context.Background()
	id := "com.powerx.plugins.ai-craft"
	svc := setting.NewPluginInstanceConfigService(deps)
	_, _, err := svc.EnsureCredentials(ctx, "8421e1d2-e571-432a-8431-273396f3e8a3", id, nil)
	require.NoError(t, err)
	path, err := runtimecredential.Path(cfg.Plugin.InstalledDir, id)
	require.NoError(t, err)
	require.NoError(t, runtimecredential.Write(path, runtimecredential.Credential{
		TenantUUID: "1edd4132-1644-412d-abb4-d5f1e9487052", ClientID: id + ".1edd4132-1644-412d-abb4-d5f1e9487052", ClientSecret: "foreign-secret",
	}))
	_, err = ensurePluginRuntimeCredential(ctx, deps, cfg, id)
	require.ErrorContains(t, err, "identity mismatch")
	require.NotContains(t, err.Error(), "foreign-secret")
}

func TestRuntimeCredentialPersistenceFailureRollsBackFirstIssuance(t *testing.T) {
	deps, cfg := newRuntimeCredentialTestDeps(t)
	id := "com.powerx.plugins.ai-craft"
	// 插件目录被普通文件占据，稳定凭证无法写入。
	require.NoError(t, os.WriteFile(filepath.Join(cfg.Plugin.InstalledDir, id), []byte("blocked"), 0o600))
	_, err := ensurePluginRuntimeCredential(context.Background(), deps, cfg, id)
	require.ErrorContains(t, err, "persist plugin runtime credential")
	svc := setting.NewPluginInstanceConfigService(deps)
	record, err := svc.Get(context.Background(), "8421e1d2-e571-432a-8431-273396f3e8a3", id, setting.KeyClientCredentials)
	require.NoError(t, err)
	require.Nil(t, record)
}
