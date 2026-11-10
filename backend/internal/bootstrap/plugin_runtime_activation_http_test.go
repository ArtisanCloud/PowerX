package bootstrap

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	pluginservice "github.com/ArtisanCloud/PowerX/internal/service/plugin"
	adminplugin "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/plugin"
	dbsetting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// The real HTTP handler must leave the lifecycle checkpoint intact, including
// failures before preparation and cancellation after preparation has committed.
type runtimeActivationHTTPManager struct {
	plugin_mgr.Manager
	enable func(context.Context, string) error
}

func (m *runtimeActivationHTTPManager) Enable(ctx context.Context, id string) error {
	return m.enable(ctx, id)
}

func (m *runtimeActivationHTTPManager) Get(_ context.Context, id string) (plugin_mgr.Plugin, error) {
	return plugin_mgr.Plugin{ID: id, Version: "0.3.1", State: plugin_mgr.StateEnabled}, nil
}

func TestHTTPEnableOwnsNoDrainCheckpoint(t *testing.T) {
	for _, tc := range []struct {
		name         string
		preflightErr bool
		startupErr   bool
		cancel       bool
		unready      bool
	}{
		{name: "credential_preflight_failure", preflightErr: true},
		{name: "startup_failure", startupErr: true},
		{name: "cancelled_startup_failure", startupErr: true, cancel: true},
		{name: "unready_drain", unready: true},
		{name: "successful_enable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			deps, cfg := newRuntimeCredentialTestDeps(t)
			require.NoError(t, deps.DB.AutoMigrate(&dbsetting.PluginDrainJob{}, &dbsetting.PluginCapabilityApproval{}))
			const pluginID = "com.powerx.plugins.http-runtime-recovery"
			cred, err := ensurePluginRuntimeCredential(context.Background(), deps, cfg, pluginID)
			require.NoError(t, err)
			require.NoError(t, deps.DB.Model(&dbsetting.PluginInstanceConfig{}).Where("plugin_id = ?", pluginID).Updates(map[string]any{
				"enabled": false, "status": dbsetting.PluginInstanceStatusDrained, "drain_job_id": "http-ready-job",
			}).Error)
			checkpoint := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
			job := dbsetting.PluginDrainJob{JobID: "http-ready-job", PluginID: pluginID, Status: dbsetting.PluginDrainJobStatusReadyToUninstall, CompletedAt: &checkpoint}
			if tc.unready {
				job.Status, job.CompletedAt = dbsetting.PluginDrainJobStatusDraining, nil
			}
			require.NoError(t, deps.DB.Create(&job).Error)
			var before dbsetting.PluginInstanceConfig
			require.NoError(t, deps.DB.Where("plugin_id = ?", pluginID).First(&before).Error)
			ctx := reqctx.WithTenantUUID(reqctx.WithClaims(context.Background(), &reqctx.CoreXClaims{IsRoot: true, UserID: 7, TenantUUID: cred.TenantUUID}), cred.TenantUUID)
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()
			enableCalls, prepared := 0, false
			previous := plugin_mgr.CurrentPluginManager()
			t.Cleanup(func() { plugin_mgr.ResetGlobalForTest(previous) })
			plugin_mgr.ResetGlobalForTest(&runtimeActivationHTTPManager{enable: func(ctx context.Context, id string) error {
				enableCalls++
				require.True(t, plugin_mgr.EventFabricProvisioning(ctx), "only explicit admin enable selects provisioning")
				var entering dbsetting.PluginDrainJob
				require.NoError(t, deps.DB.First(&entering, job.ID).Error)
				require.Equal(t, job.Status, entering.Status, "HTTP must not finalize the checkpoint before the lifecycle starts")
				if tc.preflightErr {
					return errors.New("TEST_RUNTIME_CREDENTIAL_PREFLIGHT_FAILED")
				}
				rollback, err := preparePluginRuntimeEnable(ctx, deps, id)
				if err != nil {
					return err
				}
				prepared = true
				if tc.startupErr {
					if tc.cancel {
						cancel()
					}
					require.NotNil(t, rollback)
					require.NoError(t, rollback(context.WithoutCancel(ctx)))
					return errors.New("TEST_RUNTIME_PROCESS_START_FAILED")
				}
				return nil
			}})
			router := gin.New()
			router.POST("/:id/enable", adminplugin.PluginEnableHandler(deps))
			request := httptest.NewRequest(http.MethodPost, "/"+pluginID+"/enable", nil).WithContext(ctx)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			require.Equal(t, 1, enableCalls)
			require.Equal(t, !tc.preflightErr && !tc.unready, prepared)
			var stored dbsetting.PluginDrainJob
			require.NoError(t, deps.DB.First(&stored, job.ID).Error)
			var binding dbsetting.PluginInstanceConfig
			require.NoError(t, deps.DB.First(&binding, before.ID).Error)
			guardErr := pluginservice.NewPluginDrainJobService(deps.DB).EnsurePluginAcceptsNewUsage(context.Background(), pluginID)
			if tc.preflightErr || tc.startupErr || tc.unready {
				require.GreaterOrEqual(t, response.Code, http.StatusBadRequest)
				require.False(t, binding.Enabled)
				require.Equal(t, before.Status, binding.Status)
				require.Equal(t, before.DrainJobID, binding.DrainJobID)
				require.JSONEq(t, string(before.ValueJSON), string(binding.ValueJSON))
				require.Equal(t, job.Status, stored.Status)
				if job.CompletedAt == nil {
					require.Nil(t, stored.CompletedAt)
				} else {
					require.Equal(t, checkpoint, stored.CompletedAt.UTC())
				}
				require.Error(t, guardErr, "failed enable must keep new usage blocked")
			} else {
				require.Equal(t, http.StatusOK, response.Code, response.Body.String())
				require.True(t, binding.Enabled)
				require.Equal(t, dbsetting.PluginDrainJobStatusCompleted, stored.Status)
				require.NoError(t, guardErr)
			}
		})
	}
}
