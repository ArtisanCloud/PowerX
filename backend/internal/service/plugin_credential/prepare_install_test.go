package plugincredential

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/infra/plugin/runtimecredential"
	"github.com/ArtisanCloud/PowerX/internal/service/setting"
	settingmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	settingrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/setting"
	"github.com/stretchr/testify/require"
)

func TestPrepareInstallPreviewsMissingRootWithoutWriting(t *testing.T) {
	f := newRepairFixture(t)
	require.NoError(t, os.RemoveAll(f.cfg.Plugin.InstalledDir))
	before, err := os.ReadFile(f.cfg.Plugin.RegistryFile)
	require.NoError(t, err)
	result, err := f.svc.PrepareInstall(context.Background(), RuntimeCredentialRepairOptions{PluginID: testRepairPluginID})
	require.NoError(t, err)
	require.Equal(t, "rotation_required", result.Action)
	require.False(t, result.CurrentConfigAvailable)
	require.Equal(t, "install_plugin_package", result.NextStep)
	_, err = os.Stat(f.cfg.Plugin.InstalledDir)
	require.True(t, os.IsNotExist(err))
	after, err := os.ReadFile(f.cfg.Plugin.RegistryFile)
	require.NoError(t, err)
	require.Equal(t, before, after)
	_, err = f.svc.PrepareInstall(context.Background(), RuntimeCredentialRepairOptions{PluginID: testRepairPluginID, Confirm: true})
	require.ErrorContains(t, err, "explicit -rotate")
	_, err = os.Stat(f.cfg.Plugin.InstalledDir)
	require.True(t, os.IsNotExist(err))
}

func TestPrepareInstallRotatesWithoutInventingInstalledArtifacts(t *testing.T) {
	for _, keepRegistry := range []bool{true, false} {
		t.Run(map[bool]string{true: "existing_registry", false: "missing_registry"}[keepRegistry], func(t *testing.T) {
			f := newRepairFixture(t)
			require.NoError(t, os.RemoveAll(f.cfg.Plugin.InstalledDir))
			if !keepRegistry {
				require.NoError(t, os.Remove(f.cfg.Plugin.RegistryFile))
			}
			ctx := context.Background()
			opts := RuntimeCredentialRepairOptions{PluginID: testRepairPluginID, Confirm: true, Rotate: true}
			result, err := f.svc.PrepareInstall(ctx, opts)
			require.NoError(t, err)
			require.True(t, result.Applied)
			require.Equal(t, "rotate", result.Action)
			require.False(t, result.CurrentConfigAvailable)
			require.Equal(t, "install_plugin_package", result.NextStep)
			path, err := runtimecredential.Path(f.cfg.Plugin.InstalledDir, testRepairPluginID)
			require.NoError(t, err)
			cred, err := runtimecredential.Read(path)
			require.NoError(t, err)
			verifier := &setting.PluginInstanceConfigService{PluginInstanceRepo: settingrepo.NewPluginInstanceConfigRepository(f.db)}
			require.NoError(t, verifier.VerifyClient(ctx, testRepairTenantUUID, testRepairPluginID, cred.ClientID, cred.ClientSecret, "powerx:api", "access", ""))
			_, err = os.Stat(f.hostPath)
			require.True(t, os.IsNotExist(err))
			_, err = os.Stat(filepath.Join(f.cfg.Plugin.InstalledDir, testRepairPluginID, "0.1.75"))
			require.True(t, os.IsNotExist(err))
			if !keepRegistry {
				_, err = os.Stat(f.cfg.Plugin.RegistryFile)
				require.True(t, os.IsNotExist(err), "must not create an empty installed registry")
			}
			second, err := f.svc.PrepareInstall(ctx, opts)
			require.NoError(t, err)
			require.Equal(t, "healthy", second.Action)
			require.False(t, second.Applied)
			require.Equal(t, result.SecretVersion, second.SecretVersion)
			require.Equal(t, "install_plugin_package", second.NextStep)
		})
	}
}

func TestPrepareInstallRollsBackRotationAndNewDirectoriesOnWriteFailure(t *testing.T) {
	f := newRepairFixture(t)
	require.NoError(t, os.RemoveAll(f.cfg.Plugin.InstalledDir))
	f.svc.write = func(string, []byte, os.FileMode) error { return errors.New("simulated preparation write failure") }
	result, err := f.svc.PrepareInstall(context.Background(), RuntimeCredentialRepairOptions{PluginID: testRepairPluginID, Confirm: true, Rotate: true})
	require.ErrorContains(t, err, "simulated preparation write failure")
	require.False(t, result.Applied)
	require.False(t, result.RecoveryNeeded)
	_, err = os.Stat(f.cfg.Plugin.InstalledDir)
	require.True(t, os.IsNotExist(err))
	var stored settingmodel.PluginInstanceConfig
	require.NoError(t, f.db.First(&stored, f.record.ID).Error)
	require.Equal(t, f.record.ValueJSON, stored.ValueJSON)
}

func TestPrepareInstallWithoutCredentialRecordRequiresNormalInstall(t *testing.T) {
	f := newRepairFixture(t)
	require.NoError(t, os.RemoveAll(f.cfg.Plugin.InstalledDir))
	require.NoError(t, f.db.Unscoped().Delete(f.record).Error)
	result, err := f.svc.PrepareInstall(context.Background(), RuntimeCredentialRepairOptions{PluginID: testRepairPluginID, Confirm: true, Rotate: true})
	require.NoError(t, err)
	require.Equal(t, "install_required", result.Action)
	require.False(t, result.Applied)
	_, err = os.Stat(f.cfg.Plugin.InstalledDir)
	require.True(t, os.IsNotExist(err))
	var count int64
	require.NoError(t, f.db.Model(&settingmodel.PluginInstanceConfig{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestPrepareInstallRefusesExistingHostValuesAndDisabledCredential(t *testing.T) {
	t.Run("existing_artifacts", func(t *testing.T) {
		f := newRepairFixture(t)
		_, err := f.svc.PrepareInstall(context.Background(), RuntimeCredentialRepairOptions{PluginID: testRepairPluginID, Confirm: true, Rotate: true})
		require.ErrorContains(t, err, "use repair-plugin-runtime-credentials")
	})
	t.Run("disabled_credential", func(t *testing.T) {
		f := newRepairFixture(t)
		require.NoError(t, os.RemoveAll(f.cfg.Plugin.InstalledDir))
		require.NoError(t, f.db.Model(f.record).Updates(map[string]any{"enabled": false, "status": "disabled"}).Error)
		_, err := f.svc.PrepareInstall(context.Background(), RuntimeCredentialRepairOptions{PluginID: testRepairPluginID, Confirm: true, Rotate: true})
		require.ErrorContains(t, err, "disabled or draining")
		_, err = os.Stat(f.cfg.Plugin.InstalledDir)
		require.True(t, os.IsNotExist(err))
	})
}

func TestPrepareInstallRestoresRegistryCredentialWhenArtifactsAreMissing(t *testing.T) {
	f := newRepairFixture(t)
	require.NoError(t, os.RemoveAll(f.cfg.Plugin.InstalledDir))
	raw, err := os.ReadFile(f.cfg.Plugin.RegistryFile)
	require.NoError(t, err)
	var registry map[string]any
	require.NoError(t, json.Unmarshal(raw, &registry))
	p := registry["plugins"].(map[string]any)[testRepairPluginID].(map[string]any)
	hc := p["versions"].(map[string]any)["0.1.75"].(map[string]any)["host_config"].(map[string]any)
	values := hc["values"].(map[string]any)
	values["POWERX_STS_CLIENT_ID"] = testRepairPluginID + "." + testRepairTenantUUID
	values["POWERX_STS_CLIENT_SECRET"] = f.secret
	raw, err = json.Marshal(registry)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(f.cfg.Plugin.RegistryFile, raw, 0o600))
	result, err := f.svc.PrepareInstall(context.Background(), RuntimeCredentialRepairOptions{PluginID: testRepairPluginID, Confirm: true})
	require.NoError(t, err)
	require.Equal(t, "restore", result.Action)
	require.True(t, result.Applied)
	var stored settingmodel.PluginInstanceConfig
	require.NoError(t, f.db.First(&stored, f.record.ID).Error)
	require.Equal(t, f.record.ValueJSON, stored.ValueJSON)
	path, err := runtimecredential.Path(f.cfg.Plugin.InstalledDir, testRepairPluginID)
	require.NoError(t, err)
	cred, err := runtimecredential.Read(path)
	require.NoError(t, err)
	require.Equal(t, f.secret, cred.ClientSecret)
	_, err = os.Stat(f.hostPath)
	require.True(t, os.IsNotExist(err))
}
