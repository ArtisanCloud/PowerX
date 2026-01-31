//go:build ignore

package plugin_release

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	plugsvc "github.com/ArtisanCloud/PowerX/internal/service/plugin_release"
	"github.com/ArtisanCloud/PowerX/internal/transport/http/openapi/plugin_release"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/plugin_release"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/plugin_release"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const (
	contractTenantUUID             = "a71a31ef-832c-4c99-8cf8-3a6ece8baaf7"
	canaryContractTenantUUID       = "d2df2c4c-3b7e-4e60-94fa-185141e58c4d"
	canaryScopeTenantUUID          = "e9f11823-53ce-4273-bb4e-065701e6a95c"
	guardrailContractTenantUUID    = "5a934c35-5ccc-4d59-ae59-bf938c74fceb"
	guardrailScopeTenantUUID       = "6ed77e58-5521-4ceb-bbdd-e3835fcb342c"
	distributionContractTenantUUID = "8a13ad6d-4ec5-4bf5-9241-4e8c7f5da6d0"
	distributionImportTenantUUID   = "3bf7c541-c4fb-48c9-9cec-71c8dc41b704"
	publishCLITenantUUID           = "b4bf55f1-5acd-4f0f-b918-5f0b37690698"
	adminGuardrailTenantUUID       = "c8f2410e-1349-4d1b-b3f0-4c3b89d3a7d2"
	adminGuardrailScopeTenantUUID  = "e2c2f817-1bbf-4247-9f24-972f0d559c56"
)

func setupPluginReleaseDeps(t *testing.T) (*shared.Deps, *gorm.DB) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&parseTime=true&_loc=UTC"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec("ATTACH DATABASE ':memory:' AS public").Error)
	prevSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = ""
	t.Cleanup(func() { coremodel.PowerXSchema = prevSchema })
	require.NoError(t, db.AutoMigrate(
		&models.LocalInstallSession{},
		&models.PluginReleaseCandidate{},
		&models.ReleasePlan{},
		&models.CanaryDeploymentRecord{},
		&models.OfflineDistributionPackage{},
		&models.MarketplaceListing{},
	))
	require.NoError(t, db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_plugin_release_candidate_tenant_plugin_version ON plugin_release_candidates(tenant_uuid, plugin_id, version)").Error)

	candidateRepo := repo.NewReleaseCandidateRepository(db)
	planRepo := repo.NewReleasePlanRepository(db)
	distributionRepo := repo.NewDistributionRepository(db)
	sessionRepo := repo.NewLocalInstallSessionRepository(db)

	service := plugsvc.NewService(
		candidateRepo,
		planRepo,
		distributionRepo,
		sessionRepo,
		"test.plugin.release",
		plugsvc.Options{
			FeatureFlags: plugsvc.FeatureFlagOptions{
				EnableLocalInstall:        true,
				EnableOfflineDistribution: true,
			},
			LocalInstall: plugsvc.LocalInstallOptions{
				SessionTTL:        10 * time.Minute,
				MaxArtifactSizeMB: 50,
			},
			Runtime: plugsvc.RuntimeOptions{
				RollbackTimeout: 5 * time.Minute,
			},
			Distribution: plugsvc.DistributionOptions{
				OfflineBucket:       "test-offline",
				OfflinePrefix:       "packages",
				EscalationThreshold: 2,
				ArtifactRetention:   30 * 24 * time.Hour,
				ReviewSLA:           48 * time.Hour,
			},
		},
	)

	return &shared.Deps{
		PluginReleaseService: service,
		PluginReleaseOptions: shared.PluginReleaseOptions{
			FeatureFlags: shared.PluginReleaseFeatureFlagsOptions{
				EnableLocalInstall:        true,
				EnableOfflineDistribution: true,
			},
			LocalInstall: shared.PluginReleaseLocalInstallOptions{
				SessionTTL:        10 * time.Minute,
				MaxArtifactSizeMB: 50,
			},
			Distribution: shared.PluginReleaseDistributionOptions{
				OfflineBucket:       "test-offline",
				OfflinePrefix:       "packages",
				EscalationThreshold: 2,
				ArtifactRetention:   30 * 24 * time.Hour,
			},
		},
	}, db
}

func TestTenantLocalInstallSessionLifecycle(t *testing.T) {
	gin.SetMode(gin.TestMode)

	deps, db := setupPluginReleaseDeps(t)

	engine := gin.New()
	protected := engine.Group("/api")
	protected.Use(func(c *gin.Context) {
		if c.GetHeader("Authorization") == "" {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		if tenantUUID := strings.TrimSpace(c.GetHeader("X-PowerX-Tenant")); tenantUUID != "" {
			ctx := reqctx.WithTenantUUID(c.Request.Context(), tenantUUID)
			c.Request = c.Request.WithContext(ctx)
			reqctx.CopyCtxToGin(c)
		}
		c.Next()
	})
	plugin_release.RegisterTenantRoutes(protected, deps)

	startPayload := map[string]any{
		"tenant_uuid":  contractTenantUUID,
		"developerId":  2025,
		"artifactUri":  "s3://bucket/plugins/dev-build.zip",
		"featureFlags": []string{"beta_ui"},
	}
	body, err := json.Marshal(startPayload)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/tenant/plugin-release/local/sessions", bytes.NewReader(body))
	resp := servePluginTenantRequest(t, engine, req, contractTenantUUID)
	require.Equal(t, http.StatusCreated, resp.Code)

	var createResponse struct {
		Code int `json:"code"`
		Data struct {
			SessionID    string   `json:"sessionId"`
			TenantUUID   string   `json:"tenant_uuid"`
			DeveloperID  uint64   `json:"developerId"`
			ArtifactURI  string   `json:"artifactUri"`
			FeatureFlags []string `json:"featureFlags"`
			Status       string   `json:"status"`
			CreatedAt    string   `json:"createdAt"`
			ExpiresAt    string   `json:"expiresAt"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &createResponse))
	require.Equal(t, http.StatusCreated, createResponse.Code)
	require.NotEmpty(t, createResponse.Data.SessionID)
	require.Equal(t, contractTenantUUID, createResponse.Data.TenantUUID)
	require.Equal(t, uint64(2025), createResponse.Data.DeveloperID)
	require.Equal(t, "s3://bucket/plugins/dev-build.zip", createResponse.Data.ArtifactURI)
	require.Equal(t, []string{"beta_ui"}, createResponse.Data.FeatureFlags)
	require.Equal(t, models.LocalInstallStatusInProgress, createResponse.Data.Status)
	require.NotEmpty(t, createResponse.Data.CreatedAt)
	require.NotEmpty(t, createResponse.Data.ExpiresAt)

	getReq := httptest.NewRequest(http.MethodGet, "/api/tenant/plugin-release/local/sessions/"+createResponse.Data.SessionID, nil)
	getResp := servePluginTenantRequest(t, engine, getReq, contractTenantUUID)
	require.Equal(t, http.StatusOK, getResp.Code)

	var getPayload struct {
		Code int `json:"code"`
		Data struct {
			SessionID   string   `json:"sessionId"`
			Status      string   `json:"status"`
			FeatureFlag []string `json:"featureFlags"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(getResp.Body.Bytes(), &getPayload))
	require.Equal(t, http.StatusOK, getPayload.Code)
	require.Equal(t, models.LocalInstallStatusInProgress, getPayload.Data.Status)

	sessionUUID, err := uuid.Parse(createResponse.Data.SessionID)
	require.NoError(t, err)

	stopReq := httptest.NewRequest(http.MethodDelete, "/api/tenant/plugin-release/local/sessions/"+sessionUUID.String(), nil)
	stopResp := servePluginTenantRequest(t, engine, stopReq, contractTenantUUID)
	require.Equal(t, http.StatusAccepted, stopResp.Code)

	var stored struct {
		Status    string
		ExpiredAt string
	}
	require.NoError(t, db.WithContext(context.Background()).
		Table("plugin_release_local_install_sessions").
		Select("status, expired_at").
		Where("uuid = ?", sessionUUID).
		Scan(&stored).Error)
	require.Equal(t, models.LocalInstallStatusSuccess, stored.Status)
	require.NotEmpty(t, stored.ExpiredAt)

	// session should now report success
	getAfterStop := httptest.NewRequest(http.MethodGet, "/api/tenant/plugin-release/local/sessions/"+sessionUUID.String(), nil)
	afterStopResp := servePluginTenantRequest(t, engine, getAfterStop, contractTenantUUID)
	require.Equal(t, http.StatusOK, afterStopResp.Code)

	var afterStopPayload struct {
		Code int `json:"code"`
		Data struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(afterStopResp.Body.Bytes(), &afterStopPayload))
	require.Equal(t, http.StatusOK, afterStopPayload.Code)
	require.Equal(t, models.LocalInstallStatusSuccess, afterStopPayload.Data.Status)
}
