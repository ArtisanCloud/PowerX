//go:build ignore

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

	"github.com/ArtisanCloud/PowerX/internal/service/plugin_release/pipeline"
	"github.com/ArtisanCloud/PowerX/internal/service/plugin_release/runtime"
	adminhandler "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/plugin_release"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const (
	adminCanaryTenantUUID  = "4d2b07b5-6a5d-4a86-8ce6-78b22a6e4c9c"
	adminCanaryScopeTenant = "d1aa2dda-7e30-4dcc-8c56-7b97b8d357c8"
)

func TestAdminCanaryDeployEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)
	deps, _ := setupPluginReleaseDeps(t)
	pipelineSvc := deps.PluginReleaseService.Pipeline()

	ctx := context.Background()
	candidate, err := pipelineSvc.SubmitCandidate(ctx, pipeline.SubmitCandidateInput{
		TenantUUID:    adminCanaryTenantUUID,
		PluginID:      "px.demo",
		Version:       "v3.2.1",
		BuildArtifact: "s3://releases/v3.2.1.zip",
		CommitHash:    "commit123456",
		ReleaseNotes:  "Admin HTTP deployment test release.",
		Labels: map[string]string{
			"coverage": "95",
		},
	})
	require.NoError(t, err)
	_, err = pipelineSvc.RunQualityGates(ctx, pipeline.RunQualityGatesInput{CandidateID: candidate.UUID})
	require.NoError(t, err)

	plan, _, err := pipelineSvc.GenerateReleasePlan(ctx, pipeline.GeneratePlanInput{
		CandidateID: candidate.UUID,
		WindowStart: time.Now().Add(15 * time.Minute),
		WindowEnd:   time.Now().Add(45 * time.Minute),
		CanaryBatches: []pipeline.CanaryBatchInput{
			{
				Name:        "batch-http",
				TenantScope: []string{adminCanaryScopeTenant},
				MetricThresholds: map[string]float64{
					"error_rate": 0.05,
				},
				RollbackTimeoutMins: 10,
			},
		},
		RollbackScripts:     []string{"rollback.sh"},
		NotificationTargets: []string{"release@powerx.dev"},
	})
	require.NoError(t, err)

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

	triggerBody, _ := json.Marshal(map[string]string{
		"batchName": "batch-http",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/admin/plugin-release/plans/"+strconv.FormatUint(plan.ID, 10)+"/deploy/canary", bytes.NewReader(triggerBody))
	req.Header.Set("Authorization", "Bearer admin")
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)
	require.Equal(t, http.StatusOK, resp.Code)

	var triggerPayload struct {
		Code int `json:"code"`
		Data struct {
			Events []runtime.ProgressEvent `json:"events"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &triggerPayload))
	require.GreaterOrEqual(t, len(triggerPayload.Data.Events), 2)

	finalBody, _ := json.Marshal(map[string]string{"action": "promote"})
	finalReq := httptest.NewRequest(http.MethodPost, "/api/admin/plugin-release/plans/"+strconv.FormatUint(plan.ID, 10)+"/deploy/finalize", bytes.NewReader(finalBody))
	finalReq.Header.Set("Authorization", "Bearer admin")
	finalResp := httptest.NewRecorder()
	engine.ServeHTTP(finalResp, finalReq)
	require.Equal(t, http.StatusOK, finalResp.Code)
}
