package bootstrap

import (
	"context"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/service/setting"
	dbsetting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/stretchr/testify/require"
)

func TestDrainedRuntimeCredentialRecoveryDoesNotAuthorizeTenant(t *testing.T) {
	deps, cfg := newRuntimeCredentialTestDeps(t)
	ctx := context.Background()
	const pluginID = "com.powerx.plugins.runtime-recovery"
	first, err := ensurePluginRuntimeCredential(ctx, deps, cfg, pluginID)
	require.NoError(t, err)
	svc := setting.NewPluginInstanceConfigService(deps)
	require.NoError(t, svc.SetEnabled(ctx, first.TenantUUID, pluginID, false))
	recovered, err := ensurePluginRuntimeCredential(ctx, deps, cfg, pluginID)
	require.NoError(t, err)
	require.Equal(t, first.ClientID, recovered.ClientID)
	require.Equal(t, first.ClientSecret, recovered.ClientSecret)
	require.Error(t, svc.VerifyClient(ctx, first.TenantUUID, pluginID, first.ClientID, first.ClientSecret, "powerx:api", "access", ""))
	require.Error(t, svc.ValidateStoredClientCredential(ctx, first.TenantUUID, pluginID, first.ClientID, "wrong-secret", "powerx:api", "access", ""))
}

func TestExplicitRuntimeEnableRestoresBindingAndRollsBackFailure(t *testing.T) {
	deps, cfg := newRuntimeCredentialTestDeps(t)
	require.NoError(t, deps.DB.AutoMigrate(&dbsetting.PluginDrainJob{}))
	ctx := reqctx.WithClaims(context.Background(), &reqctx.CoreXClaims{IsRoot: true, UserID: 7})
	const pluginID = "com.powerx.plugins.runtime-recovery"
	cred, err := ensurePluginRuntimeCredential(ctx, deps, cfg, pluginID)
	require.NoError(t, err)
	require.NoError(t, deps.DB.Model(&dbsetting.PluginInstanceConfig{}).Where("plugin_id = ?", pluginID).Updates(map[string]any{
		"enabled": false, "status": dbsetting.PluginInstanceStatusDrained, "drain_job_id": "upgrade-job",
	}).Error)
	job := &dbsetting.PluginDrainJob{JobID: "upgrade-job", PluginID: pluginID, Status: dbsetting.PluginDrainJobStatusReadyToUninstall}
	require.NoError(t, deps.DB.Create(job).Error)
	var before dbsetting.PluginInstanceConfig
	require.NoError(t, deps.DB.Where("plugin_id = ?", pluginID).First(&before).Error)

	rollback, err := preparePluginRuntimeEnable(ctx, deps, pluginID)
	require.NoError(t, err)
	require.NotNil(t, rollback)
	svc := setting.NewPluginInstanceConfigService(deps)
	require.NoError(t, svc.VerifyClient(ctx, cred.TenantUUID, pluginID, cred.ClientID, cred.ClientSecret, "powerx:api", "access", ""))
	var enabled dbsetting.PluginInstanceConfig
	require.NoError(t, deps.DB.First(&enabled, before.ID).Error)
	require.JSONEq(t, string(before.ValueJSON), string(enabled.ValueJSON), "activation must not change credentials or grants")
	require.NoError(t, deps.DB.First(job, job.ID).Error)
	require.Equal(t, dbsetting.PluginDrainJobStatusCompleted, job.Status)

	require.NoError(t, rollback(ctx))
	require.Error(t, svc.VerifyClient(ctx, cred.TenantUUID, pluginID, cred.ClientID, cred.ClientSecret, "powerx:api", "access", ""))
	require.NoError(t, deps.DB.First(&enabled, before.ID).Error)
	require.False(t, enabled.Enabled)
	require.Equal(t, before.Status, enabled.Status)
	require.Equal(t, before.DrainJobID, enabled.DrainJobID)
	require.NoError(t, deps.DB.First(job, job.ID).Error)
	require.Equal(t, dbsetting.PluginDrainJobStatusReadyToUninstall, job.Status)
}

func TestRuntimeEnableCannotResumeUnreadyDrainOrStartup(t *testing.T) {
	deps, cfg := newRuntimeCredentialTestDeps(t)
	require.NoError(t, deps.DB.AutoMigrate(&dbsetting.PluginDrainJob{}))
	const pluginID = "com.powerx.plugins.runtime-recovery"
	_, err := ensurePluginRuntimeCredential(context.Background(), deps, cfg, pluginID)
	require.NoError(t, err)
	require.NoError(t, deps.DB.Model(&dbsetting.PluginInstanceConfig{}).Where("plugin_id = ?", pluginID).Updates(map[string]any{"enabled": false, "status": dbsetting.PluginInstanceStatusDrained}).Error)
	require.NoError(t, deps.DB.Create(&dbsetting.PluginDrainJob{JobID: "blocked-upgrade", PluginID: pluginID, Status: dbsetting.PluginDrainJobStatusDraining}).Error)
	rollback, err := preparePluginRuntimeEnable(context.Background(), deps, pluginID)
	require.NoError(t, err)
	require.Nil(t, rollback)
	ctx := reqctx.WithClaims(context.Background(), &reqctx.CoreXClaims{IsRoot: true, UserID: 7})
	_, err = preparePluginRuntimeEnable(ctx, deps, pluginID)
	require.ErrorContains(t, err, "PLUGIN_DRAIN_NOT_READY")
	var binding dbsetting.PluginInstanceConfig
	require.NoError(t, deps.DB.Where("plugin_id = ?", pluginID).First(&binding).Error)
	require.False(t, binding.Enabled)
}
