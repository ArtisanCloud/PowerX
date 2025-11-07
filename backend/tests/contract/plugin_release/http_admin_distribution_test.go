package plugin_release

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	adminhandler "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/plugin_release"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/plugin_release"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestAdminDistributionEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)
	deps, db := setupPluginReleaseDeps(t)
	ctx := context.Background()

	candidate, err := deps.PluginReleaseService.CreateCandidate(ctx, &models.PluginReleaseCandidate{
		TenantID:         "tenant-admin-dist",
		PluginID:         "px.demo",
		Version:          "v3.1.0",
		BuildArtifactURI: "s3://bucket/releases/v3.1.0.zip",
		CommitHash:       "commit-dist-1",
		ReleaseNotes:     "Admin distribution contract test",
		GateStatus:       models.PluginReleaseGateStatusPassed,
		ApprovalStatus:   models.PluginReleaseApprovalApproved,
	})
	require.NoError(t, err)
	require.False(t, candidate.UUID == uuid.Nil)

	engine := gin.New()
	protected := engine.Group("/api/admin")
	protected.Use(func(c *gin.Context) {
		if c.GetHeader("Authorization") == "" {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		c.Next()
	})
	adminhandler.RegisterAPIRoutes(nil, protected, deps)

	pkgPayload := map[string]any{
		"releaseCandidateId":   candidate.UUID.String(),
		"packageUri":           "s3://offline/packages/px-demo-v3.1.0.pxp",
		"checksum":             "sha256:test-checksum",
		"signatureFingerprint": "fingerprint-demo",
		"dependencies":         []string{"core", "payments"},
		"licenseReport": map[string]any{
			"apache": 3,
		},
	}
	body, _ := json.Marshal(pkgPayload)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/plugin-release/offline-packages", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer admin")
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)
	require.Equal(t, http.StatusCreated, resp.Code)

	var pkgResp struct {
		Code int `json:"code"`
		Data struct {
			ID                 uint64 `json:"id"`
			ReleaseCandidateID string `json:"releaseCandidateId"`
			PackageURI         string `json:"packageUri"`
			Status             string `json:"status"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &pkgResp))
	require.NotZero(t, pkgResp.Data.ID)
	require.Equal(t, candidate.UUID.String(), pkgResp.Data.ReleaseCandidateID)
	require.Equal(t, "submitted", pkgResp.Data.Status)

	listingPayload := map[string]any{
		"offlinePackageId": pkgResp.Data.ID,
		"channel":          "online",
		"pricing": map[string]any{
			"tier": "premium",
		},
		"supportPolicy": map[string]any{
			"sla": "24x7",
		},
		"submissionForm": map[string]any{
			"notes": "Needs quick approval",
		},
	}
	listingBody, _ := json.Marshal(listingPayload)
	listingReq := httptest.NewRequest(http.MethodPost, "/api/admin/plugin-release/marketplace/listings", bytes.NewReader(listingBody))
	listingReq.Header.Set("Authorization", "Bearer admin")
	listingResp := httptest.NewRecorder()
	engine.ServeHTTP(listingResp, listingReq)
	require.Equal(t, http.StatusCreated, listingResp.Code)

	var listingData struct {
		Code int `json:"code"`
		Data struct {
			ID           uint64  `json:"id"`
			ReviewStatus string  `json:"reviewStatus"`
			EscalatedAt  *string `json:"escalatedAt"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(listingResp.Body.Bytes(), &listingData))
	require.NotZero(t, listingData.Data.ID)
	require.Equal(t, "pending", listingData.Data.ReviewStatus)

	review := func(decision string) *httptest.ResponseRecorder {
		payload := map[string]any{"decision": decision}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/api/admin/plugin-release/marketplace/listings/"+strconv.FormatUint(listingData.Data.ID, 10)+"/reviews", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer admin")
		rec := httptest.NewRecorder()
		engine.ServeHTTP(rec, req)
		return rec
	}

	needFixResp := review("need_fix")
	require.Equal(t, http.StatusOK, needFixResp.Code)

	secondNeedFix := review("need_fix")
	require.Equal(t, http.StatusOK, secondNeedFix.Code)

	var escalated struct {
		Code int `json:"code"`
		Data struct {
			ReviewStatus string  `json:"reviewStatus"`
			ReviewCount  int     `json:"reviewCount"`
			EscalatedAt  *string `json:"escalatedAt"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(secondNeedFix.Body.Bytes(), &escalated))
	require.Equal(t, "need_fix", escalated.Data.ReviewStatus)
	require.Equal(t, 2, escalated.Data.ReviewCount)
	require.NotNil(t, escalated.Data.EscalatedAt)

	approvedResp := review("approved")
	require.Equal(t, http.StatusOK, approvedResp.Code)

	var approved struct {
		Code int `json:"code"`
		Data struct {
			ReviewStatus string `json:"reviewStatus"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(approvedResp.Body.Bytes(), &approved))
	require.Equal(t, "approved", approved.Data.ReviewStatus)

	var listing models.MarketplaceListing
	require.NoError(t, db.WithContext(ctx).
		Where("id = ?", listingData.Data.ID).
		Take(&listing).Error)
	require.Equal(t, 3, listing.ReviewCount)
	require.NotNil(t, listing.EscalatedAt)
	require.WithinDuration(t, time.Now(), *listing.EscalatedAt, time.Minute)
}
