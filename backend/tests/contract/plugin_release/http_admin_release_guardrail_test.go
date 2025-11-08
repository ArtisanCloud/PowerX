//go:build ignore

package plugin_release

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	adminhandler "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/plugin_release"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestAdminReleaseGuardrailLifecycle(t *testing.T) {
	gin.SetMode(gin.TestMode)

	deps, _ := setupPluginReleaseDeps(t)

	engine := gin.New()
	protected := engine.Group("/api/admin")
	protected.Use(func(c *gin.Context) {
		if c.GetHeader("Authorization") == "" {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		ctx := reqctx.WithClaims(c.Request.Context(), &reqctx.CoreXClaims{
			IsRoot: true,
			Roles:  []string{"system_admin"},
		})
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	adminhandler.RegisterAPIRoutes(nil, protected, deps)

	candidatePayload := map[string]any{
		"tenantId":         "tenant-admin",
		"pluginId":         "px.demo",
		"version":          "v2.0.0",
		"buildArtifactUri": "s3://bucket/releases/v2.0.0.zip",
		"commitHash":       "123456789abcdef",
		"releaseNotes":     "Release with automated QA, metrics and rollback documentation.",
		"labels": map[string]string{
			"channel":  "stable",
			"coverage": "95",
		},
	}
	body, _ := json.Marshal(candidatePayload)
	createReq := httptest.NewRequest(http.MethodPost, "/api/admin/plugin-release/candidates", bytes.NewReader(body))
	createReq.Header.Set("Authorization", "Bearer admin")
	createResp := httptest.NewRecorder()
	engine.ServeHTTP(createResp, createReq)
	require.Equal(t, http.StatusCreated, createResp.Code)

	var createData struct {
		Code int `json:"code"`
		Data struct {
			CandidateID string `json:"candidateId"`
			GateStatus  string `json:"gateStatus"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(createResp.Body.Bytes(), &createData))
	require.NotEmpty(t, createData.Data.CandidateID)

	gatesReq := httptest.NewRequest(http.MethodPost, "/api/admin/plugin-release/candidates/"+createData.Data.CandidateID+"/gates", nil)
	gatesReq.Header.Set("Authorization", "Bearer admin")
	gatesResp := httptest.NewRecorder()
	engine.ServeHTTP(gatesResp, gatesReq)
	require.Equal(t, http.StatusAccepted, gatesResp.Code)

	var gatesData struct {
		Code int `json:"code"`
		Data struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(gatesResp.Body.Bytes(), &gatesData))
	require.Equal(t, "passed", gatesData.Data.Status)

	planPayload := map[string]any{
		"releaseCandidateId": createData.Data.CandidateID,
		"windowStart":        time.Now().UTC().Add(2 * time.Hour).Format(time.RFC3339),
		"windowEnd":          time.Now().UTC().Add(4 * time.Hour).Format(time.RFC3339),
		"canaryBatches": []map[string]any{
			{
				"name":        "batch-a",
				"tenantScope": []string{"tenant-x"},
				"metricThresholds": map[string]float64{
					"error_rate": 0.01,
				},
				"rollbackTimeoutMinutes": 20,
			},
		},
		"rollbackScripts":     []string{"scripts/rollback.sh"},
		"notificationTargets": []string{"devops@powerx.dev"},
	}
	planBody, _ := json.Marshal(planPayload)
	planReq := httptest.NewRequest(http.MethodPost, "/api/admin/plugin-release/plans", bytes.NewReader(planBody))
	planReq.Header.Set("Authorization", "Bearer admin")
	planResp := httptest.NewRecorder()
	engine.ServeHTTP(planResp, planReq)
	require.Equal(t, http.StatusCreated, planResp.Code)

	var planData struct {
		Code int `json:"code"`
		Data struct {
			PlanID             uint64           `json:"planId"`
			ReleaseCandidateID string           `json:"releaseCandidateId"`
			CanaryBatches      []map[string]any `json:"canaryBatches"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(planResp.Body.Bytes(), &planData))
	require.NotZero(t, planData.Data.PlanID)
	require.Equal(t, createData.Data.CandidateID, planData.Data.ReleaseCandidateID)
	require.Len(t, planData.Data.CanaryBatches, 1)

	getReq := httptest.NewRequest(http.MethodGet, "/api/admin/plugin-release/candidates/"+createData.Data.CandidateID, nil)
	getReq.Header.Set("Authorization", "Bearer admin")
	getResp := httptest.NewRecorder()
	engine.ServeHTTP(getResp, getReq)
	require.Equal(t, http.StatusOK, getResp.Code)

	var getData struct {
		Code int `json:"code"`
		Data struct {
			CandidateID    string `json:"candidateId"`
			ApprovalStatus string `json:"approvalStatus"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(getResp.Body.Bytes(), &getData))
	_, err := uuid.Parse(getData.Data.CandidateID)
	require.NoError(t, err)
	require.Equal(t, "approved", getData.Data.ApprovalStatus)
}
