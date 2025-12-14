package version

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	plugincompat "github.com/ArtisanCloud/PowerX/internal/service/plugin_compat"
	plugingovernance "github.com/ArtisanCloud/PowerX/internal/service/plugin_governance"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	compatmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/plugin_compat"
	govmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/plugin_governance"
	pluginrelease "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/plugin_release"
	compatrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/plugin_compat"
	govrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/plugin_governance"
	pluginreleaserepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/plugin_release"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const versionTenantUUID = "3f71dff7-89a1-4eb7-82d9-818f2b6ec3b8"

func TestHandlerScanAndBoard(t *testing.T) {
	t.Parallel()

	db := openVersionDB(t)
	releaseRepo := pluginreleaserepo.NewReleaseCandidateRepository(db)
	govRepo := govrepo.NewReportRepository(db)

	require.NoError(t, db.Create(&pluginrelease.PluginReleaseCandidate{
		TenantUUID:     versionTenantUUID,
		PluginID:       "plugin.demo",
		Version:        "1.2.3",
		GateStatus:     pluginrelease.PluginReleaseGateStatusPassed,
		ApprovalStatus: pluginrelease.PluginReleaseApprovalApproved,
	}).Error)

	h := &handler{
		gov: plugingovernance.NewService(govRepo, releaseRepo, func() time.Time { return time.Unix(0, 0).UTC() }),
	}

	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	body := map[string]any{
		"tenantId": versionTenantUUID,
		"pluginId": "plugin.demo",
	}
	payload, err := json.Marshal(body)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, "/internal/version/governance/scan", bytes.NewReader(payload))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	ctx.Request = withTenant(req)

	h.scan(ctx)
	require.Equal(t, http.StatusCreated, rec.Code)
	require.Contains(t, rec.Body.String(), `"risk_level"`)

	// board should return summary with the generated report
	rec = httptest.NewRecorder()
	ctx, _ = gin.CreateTestContext(rec)
	req, err = http.NewRequest(http.MethodGet, "/internal/version/governance/board?limit=5", nil)
	require.NoError(t, err)
	ctx.Request = withTenant(req)
	h.board(ctx)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"total":1`)
}

func TestHandlerCompatFlow(t *testing.T) {
	t.Parallel()

	db := openVersionDB(t)
	compatRepo := compatrepo.NewExceptionRepository(db)
	h := &handler{
		compat: plugincompat.NewService(compatRepo, func() time.Time { return time.Unix(0, 0).UTC() }),
	}

	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	payload := map[string]any{
		"tenantId":       versionTenantUUID,
		"pluginId":       "plugin.demo",
		"currentVersion": "1.0.0",
		"targetVersion":  "2.0.0",
		"reason":         "legacy dataset",
	}
	body, err := json.Marshal(payload)
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPost, "/internal/version/compat/exception", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	ctx.Request = withTenant(req)

	h.createException(ctx)
	require.Equal(t, http.StatusCreated, rec.Code)

	var response struct {
		Data compatmodel.CompatException `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	require.Equal(t, "plugin.demo", response.Data.PluginID)

	rec = httptest.NewRecorder()
	ctx, _ = gin.CreateTestContext(rec)
	approvalPayload := map[string]any{
		"id":       response.Data.UUID.String(),
		"status":   "approved",
		"reviewer": "admin@example.com",
	}
	body, err = json.Marshal(approvalPayload)
	require.NoError(t, err)
	req, err = http.NewRequest(http.MethodPost, "/internal/version/compat/approve", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	ctx.Request = withTenant(req)
	h.approveException(ctx)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"status":"approved"`)
}

func openVersionDB(t *testing.T) *gorm.DB {
	t.Helper()
	prevSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = ""
	t.Cleanup(func() { coremodel.PowerXSchema = prevSchema })

	dsn := fmt.Sprintf("file:version-tests-%d?mode=memory&cache=shared&_fk=1&_loc=UTC", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&pluginrelease.PluginReleaseCandidate{},
		&govmodel.VersionGovernanceReport{},
		&compatmodel.CompatException{},
	))
	return db
}

func withTenant(req *http.Request) *http.Request {
	ctx := req.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	ctx = reqctx.WithTenantUUID(ctx, versionTenantUUID)
	return req.WithContext(ctx)
}
