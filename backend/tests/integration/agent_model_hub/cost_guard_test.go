package agentmodelhubintegration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	appshared "github.com/ArtisanCloud/PowerX/internal/app/shared"
	amhinst "github.com/ArtisanCloud/PowerX/internal/service/agent_model_hub/instrumentation"
	amhshared "github.com/ArtisanCloud/PowerX/internal/service/agent_model_hub/shared"
	costquota "github.com/ArtisanCloud/PowerX/internal/service/cost_quota"
	agentmodelhubhttp "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/agent_model_hub"
	"github.com/ArtisanCloud/PowerX/pkg/cache"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/agent_model_hub"
	ammatestenv "github.com/ArtisanCloud/PowerX/tests/agent_model_hub/testenv"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestCostGuardIntegration(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	env := ammatestenv.New(t)
	env.MustInsertTenant(4001, ammatestenv.AgentModelHubTenantUUID)
	deps := &appshared.Deps{DB: env.DB}

	engine := gin.New()
	public := engine.Group("/api")
	protected := engine.Group("/api")
	protected.Use(ammatestenv.RequireAgentModelHubAuth())
	agentmodelhubhttp.RegisterAPIRoutes(public, protected, deps)

	ctx := context.Background()
	costSvc := costquota.NewService(costquota.Options{
		Options: amhshared.Options{
			DB:              env.DB,
			Cache:           cache.NewMemoryCache(),
			Instrumentation: amhinst.NewInstrumentation(nil, nil),
		},
	})
	_, err := costSvc.EnsureLedger(ctx, "default", costquota.LedgerInput{
		TenantUUID:     ammatestenv.AgentModelHubTenantUUID,
		BudgetPeriod:   "monthly",
		QuotaLimit:     1000,
		DashboardScope: "tenant",
	})
	require.NoError(t, err)

	// Report initial usage (600 USD)
	reportUsage(t, engine, 600)

	resp := fetchQuotas(t, engine)
	require.Len(t, resp.Data.Quotas, 1)
	require.InDelta(t, 600, resp.Data.Quotas[0].Usage, 0.1)
	require.Equal(t, "healthy", resp.Data.Quotas[0].Status)

	// Report another spike to breach the quota.
	reportUsage(t, engine, 550)

	resp = fetchQuotas(t, engine)
	require.Equal(t, "breached", resp.Data.Quotas[0].Status)
	require.InDelta(t, 1150, resp.Data.Quotas[0].Usage, 0.1)

	// Enforce throttle action via HTTP.
	rr := authedRequest(t, engine, http.MethodPost, "/api/internal/provider-quotas/enforce", mustJSONBytes(t, map[string]any{
		"env":         "default",
		"action":      "throttle",
		"reason":      "integration-test",
		"ticketId":    "INC-4242",
		"requestedBy": "tests",
	}))
	require.Equal(t, http.StatusOK, rr.Code)

	var ledger model.CostQuotaLedger
	require.NoError(t, env.DB.Where("tenant_uuid = ?", ammatestenv.AgentModelHubTenantUUID).First(&ledger).Error)
	require.Equal(t, "throttle", ledger.EnforcementState["action"])
	require.Equal(t, "integration-test", ledger.EnforcementState["reason"])
}

func reportUsage(t *testing.T, engine *gin.Engine, delta float64) {
	t.Helper()
	rr := authedRequest(t, engine, http.MethodPost, "/api/internal/provider-usage/report", mustJSONBytes(t, map[string]any{
		"env":          "default",
		"events":       []map[string]any{{"costUsd": delta}},
		"budgetPeriod": "monthly",
	}))
	require.Equal(t, http.StatusAccepted, rr.Code)
}

func fetchQuotas(t *testing.T, engine *gin.Engine) quotaResponse {
	t.Helper()
	rr := authedRequest(t, engine, http.MethodGet, "/api/internal/provider-quotas", nil)
	require.Equal(t, http.StatusOK, rr.Code)
	var resp quotaResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	require.Equal(t, http.StatusOK, resp.Code)
	return resp
}

func authedRequest(t *testing.T, engine *gin.Engine, method, path string, body []byte) *httptest.ResponseRecorder {
	var reader *bytes.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Authorization", "Bearer test")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return serveAgentModelHubRequest(t, engine, req)
}

func mustJSONBytes(t *testing.T, payload any) []byte {
	t.Helper()
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return b
}

type quotaResponse struct {
	Code int `json:"code"`
	Data struct {
		TenantUUID string           `json:"tenant_uuid"`
		Quotas     []quotaEntryResp `json:"quotas"`
	} `json:"data"`
}

type quotaEntryResp struct {
	ProviderID string  `json:"providerId"`
	Limit      float64 `json:"limit"`
	Usage      float64 `json:"usage"`
	Status     string  `json:"status"`
}
