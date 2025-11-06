package plugin_release

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	plugsvc "github.com/ArtisanCloud/PowerX/internal/service/plugin_release"
	"github.com/ArtisanCloud/PowerX/internal/transport/http/openapi/plugin_release"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/plugin_release"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/plugin_release"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupPluginReleaseDeps(t *testing.T) (*shared.Deps, *gorm.DB) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&parseTime=true&_loc=UTC"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec("ATTACH DATABASE ':memory:' AS public").Error)
	prevSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = prevSchema })
	require.NoError(t, db.AutoMigrate(&models.LocalInstallSession{}))

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
			FeatureFlags: plugsvc.FeatureFlagOptions{EnableLocalInstall: true},
			LocalInstall: plugsvc.LocalInstallOptions{
				SessionTTL:        10 * time.Minute,
				MaxArtifactSizeMB: 50,
			},
		},
	)

	return &shared.Deps{
		PluginReleaseService: service,
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
		c.Next()
	})
	plugin_release.RegisterTenantRoutes(protected, deps)

	startPayload := map[string]any{
		"tenantId":     "101",
		"developerId":  2025,
		"artifactUri":  "s3://bucket/plugins/dev-build.zip",
		"featureFlags": []string{"beta_ui"},
	}
	body, err := json.Marshal(startPayload)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/tenant/plugin-release/local/sessions", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer tenant-token")
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)
	require.Equal(t, http.StatusCreated, resp.Code)

	var createResponse struct {
		Code int `json:"code"`
		Data struct {
			SessionID    string   `json:"sessionId"`
			TenantID     string   `json:"tenantId"`
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
	require.Equal(t, "101", createResponse.Data.TenantID)
	require.Equal(t, uint64(2025), createResponse.Data.DeveloperID)
	require.Equal(t, "s3://bucket/plugins/dev-build.zip", createResponse.Data.ArtifactURI)
	require.Equal(t, []string{"beta_ui"}, createResponse.Data.FeatureFlags)
	require.Equal(t, models.LocalInstallStatusInProgress, createResponse.Data.Status)
	require.NotEmpty(t, createResponse.Data.CreatedAt)
	require.NotEmpty(t, createResponse.Data.ExpiresAt)

	getReq := httptest.NewRequest(http.MethodGet, "/api/tenant/plugin-release/local/sessions/"+createResponse.Data.SessionID, nil)
	getReq.Header.Set("Authorization", "Bearer tenant-token")
	getResp := httptest.NewRecorder()
	engine.ServeHTTP(getResp, getReq)
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
	stopReq.Header.Set("Authorization", "Bearer tenant-token")
	stopResp := httptest.NewRecorder()
	engine.ServeHTTP(stopResp, stopReq)
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
	getAfterStop.Header.Set("Authorization", "Bearer tenant-token")
	afterStopResp := httptest.NewRecorder()
	engine.ServeHTTP(afterStopResp, getAfterStop)
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
