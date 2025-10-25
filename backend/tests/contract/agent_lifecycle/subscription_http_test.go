package agentlifecyclecontract

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/service/agent_lifecycle"
	"github.com/ArtisanCloud/PowerX/tests/agent_lifecycle/testenv"
	"github.com/stretchr/testify/require"
)

type subscriptionResponse struct {
	MetricsFilter  []string `json:"metrics_filter"`
	HealthStatuses []string `json:"health_statuses"`
	UpdatedAt      string   `json:"updated_at"`
}

func TestSubscriptionHTTP(t *testing.T) {
	env := testenv.New(t)
	t.Cleanup(env.Close)

	service := env.Deps.AgentLifecycle.Service
	ctx := context.Background()

	res, err := service.Register(ctx, agent_lifecycle.RegisterInput{
		TenantID:                 "tenant-002",
		Alias:                    "subscription-http",
		TelemetryContractVersion: "otel-agent-v1",
	})
	require.NoError(t, err)
	agentID := res.Agent.ID.String()

	httpEngine := env.Engine()

	update := func(body map[string]any, expectedStatusCode int) *httptest.ResponseRecorder {
		data, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/admin/agent/lifecycle/agents/%s/subscription", agentID), bytes.NewReader(data))
		req.Header.Set("Authorization", "Bearer token")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		httpEngine.ServeHTTP(rec, req)
		require.Equal(t, expectedStatusCode, rec.Code)
		return rec
	}

	fetch := func() subscriptionResponse {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/admin/agent/lifecycle/agents/%s/subscription", agentID), nil)
		req.Header.Set("Authorization", "Bearer token")
		rec := httptest.NewRecorder()
		httpEngine.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
		var body struct {
			Data subscriptionResponse `json:"data"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		return body.Data
	}

	reqBody := map[string]any{
		"tenant_id":       "tenant-002",
		"metrics_filter":  []string{"error_rate", "p95_latency_ms"},
		"health_statuses": []string{"degraded", "unavailable"},
		"requested_by":    "ops-user",
		"trace_id":        "trace-subscription-1",
	}
	response := update(reqBody, http.StatusOK)

	var updateBody struct {
		Data subscriptionResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &updateBody))
	require.Contains(t, updateBody.Data.HealthStatuses, "degraded")
	require.NotEmpty(t, updateBody.Data.UpdatedAt)

	// 新配置应立即可读
	cfg := fetch()
	require.Equal(t, []string{"error_rate", "p95_latency_ms"}, cfg.MetricsFilter)
	require.Contains(t, cfg.HealthStatuses, "unavailable")

	// 更新为仅在不可用时告警
	update(map[string]any{
		"tenant_id":       "tenant-002",
		"metrics_filter":  []string{"success_rate"},
		"health_statuses": []string{"unavailable"},
		"requested_by":    "ops-review",
		"trace_id":        "trace-subscription-2",
	}, http.StatusOK)

	cfg = fetch()
	require.Equal(t, []string{"success_rate"}, cfg.MetricsFilter)
	require.Equal(t, []string{"unavailable"}, cfg.HealthStatuses)

	// 构造非法请求，预期触发回滚（保持旧配置）
	badResp := update(map[string]any{
		"tenant_id":       "tenant-002",
		"metrics_filter":  []string{},
		"health_statuses": []string{},
	}, http.StatusBadRequest)
	require.NotEmpty(t, badResp.Body.String())

	cfg = fetch()
	require.Equal(t, []string{"success_rate"}, cfg.MetricsFilter)
	require.Equal(t, []string{"unavailable"}, cfg.HealthStatuses)
}
