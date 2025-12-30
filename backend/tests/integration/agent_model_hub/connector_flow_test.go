package agentmodelhubintegration

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	appshared "github.com/ArtisanCloud/PowerX/internal/app/shared"
	amhinst "github.com/ArtisanCloud/PowerX/internal/service/agent_model_hub/instrumentation"
	"github.com/ArtisanCloud/PowerX/internal/service/agent_model_hub/shared"
	connectorguard "github.com/ArtisanCloud/PowerX/internal/service/connector_guard"
	agentmodelhubhttp "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/agent_model_hub"
	"github.com/ArtisanCloud/PowerX/pkg/cache"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/agent_model_hub"
	"github.com/ArtisanCloud/PowerX/pkg/corex/tenantkeys"
	ammatestenv "github.com/ArtisanCloud/PowerX/tests/agent_model_hub/testenv"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestConnectorFlowIntegration(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	env := ammatestenv.New(t)
	env.MustInsertTenant(3001, ammatestenv.AgentModelHubTenantUUID)

	engine := gin.New()
	public := engine.Group("/api")
	protected := engine.Group("/api")
	protected.Use(ammatestenv.RequireAgentModelHubAuth())

	deps := &appshared.Deps{DB: env.DB}
	agentmodelhubhttp.RegisterAPIRoutes(public, protected, deps)

	payload := map[string]any{
		"env":    "default",
		"region": "us-east-1",
		"mappingTemplate": map[string]any{
			"workflow": "sync_leads",
			"fields": map[string]string{
				"crm": "hubspot",
			},
		},
		"rateLimitPerMinute": 90,
		"secrets": map[string]string{
			"oauth_token":         "coze-refresh-token-int",
			"webhook_signing_key": "coze-webhook-secret-int",
		},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/internal/connector-platforms/coze/instances", bytes.NewReader(mustJSON(t, payload)))
	req.Header.Set("Content-Type", "application/json")
	rr := serveAgentModelHubRequest(t, engine, req)
	require.Equal(t, http.StatusOK, rr.Code)

	var resp struct {
		Code int                    `json:"code"`
		Data map[string]interface{} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	instance := resp.Data["instance"].(map[string]interface{})
	instanceID := instance["instance_id"].(string)
	require.NotEmpty(t, instanceID)

	connectorSvc := connectorguard.NewService(connectorguard.Options{
		Options: shared.Options{
			DB:              env.DB,
			Cache:           cache.NewMemoryCache(),
			TenantKeySvc:    tenantkeys.NewTenantKeyService(env.DB),
			Instrumentation: amhinst.NewInstrumentation(nil, nil),
		},
		Instances: repo.NewConnectorInstanceRepository(env.DB),
	})

	payloadBody := []byte(`{"ok":true}`)
	timestamp := time.Now().UTC().Format(time.RFC3339)
	signature := "sha256=" + computeTestSignature("coze-webhook-secret-int", timestamp, payloadBody)

	err := connectorSvc.VerifyWebhookSignature(context.Background(), connectorguard.WebhookVerificationInput{
		InstanceID: uuid.MustParse(instanceID),
		Signature:  signature,
		Timestamp:  timestamp,
		Payload:    payloadBody,
	})
	require.NoError(t, err)

	// Track a failure metric to trigger auto-pause
	newRate, triggered, err := connectorSvc.TrackCallbackMetric(context.Background(), connectorguard.CallbackMetricInput{
		InstanceID: uuid.MustParse(instanceID),
		Success:    false,
		Threshold:  0.05,
		Reason:     "integration test failure",
	})
	require.NoError(t, err)
	require.True(t, triggered)
	require.GreaterOrEqual(t, newRate, 0.05)

	// Manual pause endpoint should still succeed (idempotent)
	pauseReq := httptest.NewRequest(http.MethodPost, "/api/internal/connector-platforms/coze/instances/"+instanceID+"/pause", bytes.NewReader([]byte(`{"reason":"manual confirm"}`)))
	pauseReq.Header.Set("Content-Type", "application/json")
	pauseRR := serveAgentModelHubRequest(t, engine, pauseReq)
	require.Equal(t, http.StatusOK, pauseRR.Code)

	record, err := repo.NewConnectorInstanceRepository(env.DB).FindByUUID(context.Background(), uuid.MustParse(instanceID))
	require.NoError(t, err)
	require.Equal(t, "paused", record.Status)
	require.NotEmpty(t, record.LastPauseReason)
}

func computeTestSignature(secret, timestamp string, payload []byte) string {
	body := append([]byte(timestamp), '.')
	body = append(body, payload...)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}
