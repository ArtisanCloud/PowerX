package plugincredential

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/infra/plugin/runtimecredential"
	"github.com/ArtisanCloud/PowerX/internal/service/setting"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	auditmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	settingmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	tenantmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	settingrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/setting"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
	"gopkg.in/yaml.v3"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const testRepairPluginID = "com.powerx.plugins.ai-craft"
const testRepairTenantUUID = "8421e1d2-e571-432a-8431-273396f3e8a3"

type repairFixture struct {
	db       *gorm.DB
	cfg      *config.Config
	svc      *RuntimeCredentialRepairService
	record   *settingmodel.PluginInstanceConfig
	hostPath string
	secret   string
}

func newRepairFixture(t *testing.T) repairFixture {
	t.Helper()
	previous := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = previous })
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&tenantmodel.Tenant{}, &settingmodel.PluginInstanceConfig{}, &auditmodel.AuditEvent{}))
	tenant := tenantmodel.Tenant{Key: "system", Name: "System", Type: tenantmodel.TenantTypeSystem, Status: 1}
	tenant.UUID = uuid.MustParse(testRepairTenantUUID)
	require.NoError(t, db.Create(&tenant).Error)
	secret := "original-runtime-secret-for-test"
	hash, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.MinCost)
	require.NoError(t, err)
	cc := setting.ClientCredential{ClientID: testRepairPluginID + "." + testRepairTenantUUID, ClientSecretHash: string(hash), SecretVersion: 3, AllowedAudiences: []string{"powerx:api"}, AllowedScopes: []string{"access"}}
	raw, err := json.Marshal(cc)
	require.NoError(t, err)
	record := &settingmodel.PluginInstanceConfig{TenantUUID: testRepairTenantUUID, PluginID: testRepairPluginID, Key: setting.KeyClientCredentials, ValueJSON: datatypes.JSON(raw), Enabled: true, Status: "enabled"}
	require.NoError(t, db.Create(record).Error)
	cfg := &config.Config{Deployment: config.DeploymentConfig{Env: "prod"}}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	cfg.Server.Port = listener.Addr().(*net.TCPAddr).Port
	require.NoError(t, listener.Close())
	root := t.TempDir()
	cfg.Plugin.InstalledDir = filepath.Join(root, "installed")
	cfg.Plugin.RegistryFile = filepath.Join(root, "registry.json")
	hostPath := filepath.Join(cfg.Plugin.InstalledDir, testRepairPluginID, "0.1.75/config/host-values.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(hostPath), 0o750))
	require.NoError(t, os.WriteFile(hostPath, []byte("env:\n  POWERX_DB_DATABASE: plugin_prod\ndatabase:\n  schema: preserved_schema\n"), 0o600))
	registry := map[string]any{"format_marker": "preserved", "plugins": map[string]any{testRepairPluginID: map[string]any{
		"current": "0.1.75", "versions": map[string]any{"0.1.75": map[string]any{
			"state": "installed", "manifest": map[string]any{"id": testRepairPluginID, "version": "0.1.75"},
			"paths":       map[string]any{"root": filepath.Dir(filepath.Dir(hostPath)), "host_values_file": hostPath},
			"host_config": map[string]any{"values": map[string]any{"UNRELATED": "keep"}},
		}},
	}}}
	raw, err = json.Marshal(registry)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(cfg.Plugin.RegistryFile, raw, 0o600))
	return repairFixture{db: db, cfg: cfg, svc: NewRuntimeCredentialRepairService(db, cfg), record: record, hostPath: hostPath, secret: secret}
}

func TestRepairPreviewAndConfirmationDoNotSilentlyRotate(t *testing.T) {
	f := newRepairFixture(t)
	before, err := os.ReadFile(f.cfg.Plugin.RegistryFile)
	require.NoError(t, err)
	result, err := f.svc.Repair(context.Background(), RuntimeCredentialRepairOptions{PluginID: testRepairPluginID, Rotate: true})
	require.NoError(t, err)
	require.Equal(t, "rotation_required", result.Action)
	require.False(t, result.Applied)
	require.Empty(t, result.BackupDir)
	_, err = f.svc.Repair(context.Background(), RuntimeCredentialRepairOptions{PluginID: testRepairPluginID, Confirm: true})
	require.ErrorContains(t, err, "explicit -rotate")
	after, err := os.ReadFile(f.cfg.Plugin.RegistryFile)
	require.NoError(t, err)
	require.Equal(t, before, after)
	var stored settingmodel.PluginInstanceConfig
	require.NoError(t, f.db.First(&stored, f.record.ID).Error)
	require.Equal(t, f.record.ValueJSON, stored.ValueJSON)
}

func TestRepairRotationUpdatesAllStoresAndIsIdempotent(t *testing.T) {
	f := newRepairFixture(t)
	// 同一插件的其他租户凭证不得被轮换。
	other := &settingmodel.PluginInstanceConfig{TenantUUID: "1edd4132-1644-412d-abb4-d5f1e9487052", PluginID: testRepairPluginID, Key: setting.KeyClientCredentials, ValueJSON: f.record.ValueJSON, Enabled: true}
	require.NoError(t, f.db.Create(other).Error)
	ctx := context.Background()
	opts := RuntimeCredentialRepairOptions{PluginID: testRepairPluginID, Confirm: true, Rotate: true}
	result, err := f.svc.Repair(ctx, opts)
	require.NoError(t, err)
	require.Equal(t, "rotate", result.Action)
	require.True(t, result.Applied)
	require.Equal(t, 4, result.SecretVersion)
	path, err := runtimecredential.Path(f.cfg.Plugin.InstalledDir, testRepairPluginID)
	require.NoError(t, err)
	cred, err := runtimecredential.Read(path)
	require.NoError(t, err)
	require.NotEqual(t, f.secret, cred.ClientSecret)
	verifier := &setting.PluginInstanceConfigService{PluginInstanceRepo: settingrepo.NewPluginInstanceConfigRepository(f.db)}
	require.NoError(t, verifier.VerifyClient(ctx, testRepairTenantUUID, testRepairPluginID, cred.ClientID, cred.ClientSecret, "powerx:api", "access", ""))
	require.Error(t, verifier.VerifyClient(ctx, testRepairTenantUUID, testRepairPluginID, cred.ClientID, f.secret, "powerx:api", "access", ""))
	var storedOther settingmodel.PluginInstanceConfig
	require.NoError(t, f.db.First(&storedOther, other.ID).Error)
	require.Equal(t, other.ValueJSON, storedOther.ValueJSON)
	for _, path := range []string{path, f.hostPath, f.cfg.Plugin.RegistryFile, filepath.Join(result.BackupDir, "backup.json")} {
		info, err := os.Stat(path)
		require.NoError(t, err)
		require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}
	info, err := os.Stat(result.BackupDir)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o700), info.Mode().Perm())
	hostRaw, err := os.ReadFile(f.hostPath)
	require.NoError(t, err)
	var host map[string]any
	require.NoError(t, yaml.Unmarshal(hostRaw, &host))
	require.Equal(t, cred.ClientSecret, host["env"].(map[string]any)["POWERX_STS_CLIENT_SECRET"])
	require.Equal(t, "plugin_prod", host["env"].(map[string]any)["POWERX_DB_DATABASE"])
	require.Equal(t, "preserved_schema", host["database"].(map[string]any)["schema"])
	registryRaw, err := os.ReadFile(f.cfg.Plugin.RegistryFile)
	require.NoError(t, err)
	var registry map[string]any
	require.NoError(t, json.Unmarshal(registryRaw, &registry))
	require.Equal(t, "preserved", registry["format_marker"])
	var audit auditmodel.AuditEvent
	require.NoError(t, f.db.First(&audit).Error)
	require.Equal(t, "PLUGIN_RUNTIME_CREDENTIAL_REPAIR", audit.Operation)
	resultRaw, err := json.Marshal(result)
	require.NoError(t, err)
	require.NotContains(t, string(resultRaw), cred.ClientSecret)
	require.NotContains(t, string(audit.Meta), cred.ClientSecret)
	second, err := f.svc.Repair(ctx, opts)
	require.NoError(t, err)
	require.Equal(t, "healthy", second.Action)
	require.False(t, second.Applied)
	require.Equal(t, 4, second.SecretVersion)
	after, err := os.ReadFile(f.cfg.Plugin.RegistryFile)
	require.NoError(t, err)
	require.Equal(t, registryRaw, after)
}

func TestRepairRestoresVerifiedRegistryCredentialWithoutRotation(t *testing.T) {
	f := newRepairFixture(t)
	raw, err := os.ReadFile(f.cfg.Plugin.RegistryFile)
	require.NoError(t, err)
	var registry map[string]any
	require.NoError(t, json.Unmarshal(raw, &registry))
	p := registry["plugins"].(map[string]any)[testRepairPluginID].(map[string]any)
	v := p["versions"].(map[string]any)["0.1.75"].(map[string]any)
	values := v["host_config"].(map[string]any)["values"].(map[string]any)
	values["POWERX_STS_CLIENT_ID"] = testRepairPluginID + "." + testRepairTenantUUID
	values["POWERX_STS_CLIENT_SECRET"] = f.secret
	raw, err = json.Marshal(registry)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(f.cfg.Plugin.RegistryFile, raw, 0o600))
	preview, err := f.svc.Repair(context.Background(), RuntimeCredentialRepairOptions{PluginID: testRepairPluginID})
	require.NoError(t, err)
	require.Equal(t, "restore", preview.Action)
	result, err := f.svc.Repair(context.Background(), RuntimeCredentialRepairOptions{PluginID: testRepairPluginID, Confirm: true})
	require.NoError(t, err)
	require.Equal(t, "restore", result.Action)
	var stored settingmodel.PluginInstanceConfig
	require.NoError(t, f.db.First(&stored, f.record.ID).Error)
	require.Equal(t, f.record.ValueJSON, stored.ValueJSON)
	path, err := runtimecredential.Path(f.cfg.Plugin.InstalledDir, testRepairPluginID)
	require.NoError(t, err)
	cred, err := runtimecredential.Read(path)
	require.NoError(t, err)
	require.Equal(t, f.secret, cred.ClientSecret)
}

func TestRepairFileFailureRollsBackDatabaseAndRuntimeFiles(t *testing.T) {
	f := newRepairFixture(t)
	registryBefore, err := os.ReadFile(f.cfg.Plugin.RegistryFile)
	require.NoError(t, err)
	hostBefore, err := os.ReadFile(f.hostPath)
	require.NoError(t, err)
	f.svc.write = func(path string, raw []byte, mode os.FileMode) error {
		if path == f.hostPath {
			return errors.New("simulated write failure")
		}
		return runtimecredential.AtomicWrite(path, raw, mode)
	}
	result, err := f.svc.Repair(context.Background(), RuntimeCredentialRepairOptions{PluginID: testRepairPluginID, Confirm: true, Rotate: true})
	require.ErrorContains(t, err, "simulated write failure")
	require.False(t, result.Applied)
	require.False(t, result.RecoveryNeeded)
	require.NotEmpty(t, result.BackupDir)
	after, err := os.ReadFile(f.cfg.Plugin.RegistryFile)
	require.NoError(t, err)
	require.Equal(t, registryBefore, after)
	after, err = os.ReadFile(f.hostPath)
	require.NoError(t, err)
	require.Equal(t, hostBefore, after)
	path, err := runtimecredential.Path(f.cfg.Plugin.InstalledDir, testRepairPluginID)
	require.NoError(t, err)
	_, err = os.Stat(path)
	require.True(t, os.IsNotExist(err))
	var stored settingmodel.PluginInstanceConfig
	require.NoError(t, f.db.First(&stored, f.record.ID).Error)
	require.Equal(t, f.record.ValueJSON, stored.ValueJSON)
	var auditCount int64
	require.NoError(t, f.db.Model(&auditmodel.AuditEvent{}).Count(&auditCount).Error)
	require.Zero(t, auditCount)
}

func TestRepairRefusesDisabledIdentityAndForeignRuntimePaths(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		f := newRepairFixture(t)
		require.NoError(t, f.db.Model(f.record).Updates(map[string]any{"enabled": false, "status": "disabled"}).Error)
		_, err := f.svc.Repair(context.Background(), RuntimeCredentialRepairOptions{PluginID: testRepairPluginID, Confirm: true, Rotate: true})
		require.ErrorContains(t, err, "disabled or draining")
	})
	t.Run("foreign_path", func(t *testing.T) {
		f := newRepairFixture(t)
		raw, err := os.ReadFile(f.cfg.Plugin.RegistryFile)
		require.NoError(t, err)
		var registry map[string]any
		require.NoError(t, json.Unmarshal(raw, &registry))
		p := registry["plugins"].(map[string]any)[testRepairPluginID].(map[string]any)
		v := p["versions"].(map[string]any)["0.1.75"].(map[string]any)
		v["paths"].(map[string]any)["host_values_file"] = "/opt/powerx-dev/another-plugin/config/host-values.yaml"
		raw, err = json.Marshal(registry)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(f.cfg.Plugin.RegistryFile, raw, 0o600))
		_, err = f.svc.Repair(context.Background(), RuntimeCredentialRepairOptions{PluginID: testRepairPluginID, Confirm: true, Rotate: true})
		require.ErrorContains(t, err, "path mismatch")
	})
}

func TestRepairRefusesApplyWhileCorePortIsListening(t *testing.T) {
	f := newRepairFixture(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	f.cfg.Server.Port = listener.Addr().(*net.TCPAddr).Port
	preview, err := f.svc.Repair(context.Background(), RuntimeCredentialRepairOptions{PluginID: testRepairPluginID})
	require.NoError(t, err)
	require.Equal(t, "rotation_required", preview.Action)
	result, err := f.svc.Repair(context.Background(), RuntimeCredentialRepairOptions{PluginID: testRepairPluginID, Confirm: true, Rotate: true})
	require.ErrorContains(t, err, "stop the matching Core service")
	require.False(t, result.Applied)
	require.Empty(t, result.BackupDir)
}

func TestRepairPreservesAuthorizationRestrictions(t *testing.T) {
	for _, name := range []string{"scope", "audience", "expiry", "draining"} {
		t.Run(name, func(t *testing.T) {
			f := newRepairFixture(t)
			var cc setting.ClientCredential
			require.NoError(t, json.Unmarshal(f.record.ValueJSON, &cc))
			switch name {
			case "scope":
				cc.AllowedScopes = []string{"other-scope"}
			case "audience":
				cc.AllowedAudiences = []string{"other-audience"}
			case "expiry":
				expires := time.Now().Unix() - 60
				cc.ExpiresAt = &expires
			case "draining":
				require.NoError(t, f.db.Model(f.record).Update("status", "draining_requested").Error)
			}
			raw, err := json.Marshal(cc)
			require.NoError(t, err)
			require.NoError(t, f.db.Model(f.record).Update("value_json", datatypes.JSON(raw)).Error)
			_, err = f.svc.Repair(context.Background(), RuntimeCredentialRepairOptions{PluginID: testRepairPluginID, Confirm: true, Rotate: true})
			require.Error(t, err)
			var after settingmodel.PluginInstanceConfig
			require.NoError(t, f.db.First(&after, f.record.ID).Error)
			require.JSONEq(t, string(raw), string(after.ValueJSON))
			if name == "draining" {
				require.Equal(t, "draining_requested", after.Status)
			}
		})
	}
}

func TestRepairDoesNotReuseForeignTenantCredential(t *testing.T) {
	f := newRepairFixture(t)
	raw, err := yaml.Marshal(map[string]any{"env": map[string]string{
		"POWERX_STS_CLIENT_ID":             testRepairPluginID + ".1edd4132-1644-412d-abb4-d5f1e9487052",
		"POWERX_STS_CLIENT_SECRET":         f.secret,
		"POWERX_GRPC_UPSTREAM_TENANT_UUID": "1edd4132-1644-412d-abb4-d5f1e9487052",
	}})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(f.hostPath, raw, 0o600))
	result, err := f.svc.Repair(context.Background(), RuntimeCredentialRepairOptions{PluginID: testRepairPluginID})
	require.NoError(t, err)
	require.Equal(t, "rotation_required", result.Action)
}

func TestRepairDirectoryErrorsIdentifyTheFailedPath(t *testing.T) {
	t.Run("installed_root_missing", func(t *testing.T) {
		f := newRepairFixture(t)
		f.cfg.Plugin.InstalledDir = filepath.Join(t.TempDir(), "missing-installed")
		_, err := f.svc.Repair(context.Background(), RuntimeCredentialRepairOptions{PluginID: testRepairPluginID})
		require.ErrorContains(t, err, "PLUGIN_RUNTIME_INSTALLED_ROOT_UNAVAILABLE")
		require.ErrorContains(t, err, f.cfg.Plugin.InstalledDir)
		require.ErrorIs(t, err, os.ErrNotExist)
	})
	t.Run("plugin_directory_missing", func(t *testing.T) {
		f := newRepairFixture(t)
		pluginRoot := filepath.Join(f.cfg.Plugin.InstalledDir, testRepairPluginID)
		require.NoError(t, os.RemoveAll(pluginRoot))
		_, err := f.svc.Repair(context.Background(), RuntimeCredentialRepairOptions{PluginID: testRepairPluginID})
		require.ErrorContains(t, err, "PLUGIN_RUNTIME_PLUGIN_DIR_UNAVAILABLE")
		require.ErrorContains(t, err, pluginRoot)
		require.ErrorIs(t, err, os.ErrNotExist)
		var after settingmodel.PluginInstanceConfig
		require.NoError(t, f.db.First(&after, f.record.ID).Error)
		require.Equal(t, f.record.ValueJSON, after.ValueJSON)
	})
	t.Run("plugin_symlink_outside_root", func(t *testing.T) {
		f := newRepairFixture(t)
		pluginRoot := filepath.Join(f.cfg.Plugin.InstalledDir, testRepairPluginID)
		require.NoError(t, os.RemoveAll(pluginRoot))
		target := t.TempDir()
		require.NoError(t, os.Symlink(target, pluginRoot))
		_, err := f.svc.Repair(context.Background(), RuntimeCredentialRepairOptions{PluginID: testRepairPluginID})
		require.ErrorContains(t, err, "PLUGIN_RUNTIME_PLUGIN_DIR_OUTSIDE_INSTALLED_ROOT")
		resolved, err := filepath.EvalSymlinks(target)
		require.NoError(t, err)
		_, _, err = resolveRepairDirectories(f.cfg.Plugin.InstalledDir, testRepairPluginID)
		require.ErrorContains(t, err, resolved)
	})
}

func TestRepairDirectoryContainsHandlesFilesystemRootAndSiblingPrefix(t *testing.T) {
	require.True(t, repairDirectoryContains(string(os.PathSeparator), filepath.Join(string(os.PathSeparator), "opt", "powerx")))
	require.False(t, repairDirectoryContains("/opt/powerx/plugins/installed", "/opt/powerx/plugins/installed-other/plugin"))
	require.False(t, repairDirectoryContains("/opt/powerx/plugins/installed", "/opt/powerx/plugins/installed"))
}
