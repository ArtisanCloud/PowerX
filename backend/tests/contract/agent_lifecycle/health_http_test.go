//go:build ignore

package agentlifecyclecontract

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/agent_lifecycle"
	"github.com/ArtisanCloud/PowerX/tests/agent_lifecycle/testenv"
	"github.com/stretchr/testify/require"
)

func TestHealthHTTP(t *testing.T) {
	env := testenv.New(t)
	t.Cleanup(env.Close)

	svc := env.Deps.AgentLifecycle.Service
	ctx := context.Background()

	result, err := svc.Register(ctx, agent_lifecycle.RegisterInput{
		TenantID:                 "tenant-001",
		Alias:                    "health-http",
		TelemetryContractVersion: "otel-agent-v1",
	})
	require.NoError(t, err)
	agentID := result.Agent.ID

	// 预先写入多段健康快照，模拟近期退化
	base := time.Now().Add(-2 * time.Hour).UTC()
	record := func(offset time.Duration, status string, metrics agent_lifecycle.HealthMetricsInput) {
		err := svc.RecordHealthSnapshot(ctx, agent_lifecycle.HealthInput{
			AgentID:         agentID,
			TenantID:        "tenant-001",
			WindowStartedAt: base.Add(offset),
			WindowDuration:  time.Minute,
			Status:          status,
			Metrics:         metrics,
		})
		require.NoError(t, err)
	}

	record(0, "healthy", agent_lifecycle.HealthMetricsInput{
		ThroughputPerMin: 120,
		SuccessRate:      0.99,
		P95LatencyMs:     120,
		ResourceUtilPct:  0.45,
		ErrorRate:        0.01,
	})

	record(time.Hour, "degraded", agent_lifecycle.HealthMetricsInput{
		ThroughputPerMin: 70,
		SuccessRate:      0.78,
		P95LatencyMs:     1800,
		ResourceUtilPct:  0.88,
		ErrorRate:        0.32,
		AnomalyTraceIDs:  []string{"trace-a"},
	})

	record(90*time.Minute, "unavailable", agent_lifecycle.HealthMetricsInput{
		ThroughputPerMin: 20,
		SuccessRate:      0.4,
		P95LatencyMs:     2500,
		ResourceUtilPct:  0.92,
		ErrorRate:        0.65,
		AnomalyTraceIDs:  []string{"trace-b"},
	})

	engine := env.Engine()

	// 拉取健康摘要
	summaryReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/openapi/agents/%s/health/summary", agentID.String()), nil)
	summaryReq.Header.Set("Authorization", "Bearer token")
	summaryResp := httptest.NewRecorder()
	engine.ServeHTTP(summaryResp, summaryReq)
	require.Equal(t, http.StatusOK, summaryResp.Code)

	var summaryBody struct {
		Data struct {
			Status          string   `json:"status"`
			HealthScore     int      `json:"health_score"`
			Recommendations []string `json:"recommendations"`
			Metrics         struct {
				ErrorRate        float64 `json:"error_rate"`
				P95LatencyMs     int     `json:"p95_latency_ms"`
				ThroughputPerMin float64 `json:"throughput_per_min"`
			} `json:"metrics"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(summaryResp.Body.Bytes(), &summaryBody))
	require.Equal(t, "unavailable", summaryBody.Data.Status)
	require.Less(t, summaryBody.Data.HealthScore, 60)
	require.NotEmpty(t, summaryBody.Data.Recommendations)
	require.InDelta(t, 0.65, summaryBody.Data.Metrics.ErrorRate, 0.001)

	// 拉取健康历史
	historyReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/openapi/agents/%s/health/history?range_hours=3&limit=5", agentID.String()), nil)
	historyReq.Header.Set("Authorization", "Bearer token")
	historyResp := httptest.NewRecorder()
	engine.ServeHTTP(historyResp, historyReq)
	require.Equal(t, http.StatusOK, historyResp.Code)

	var historyBody struct {
		Data struct {
			Snapshots []struct {
				Status      string   `json:"status"`
				HealthScore int      `json:"health_score"`
				TraceIDs    []string `json:"anomaly_trace_ids"`
			} `json:"snapshots"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(historyResp.Body.Bytes(), &historyBody))
	require.GreaterOrEqual(t, len(historyBody.Data.Snapshots), 3)
	require.Equal(t, "unavailable", historyBody.Data.Snapshots[0].Status)
	require.Contains(t, historyBody.Data.Snapshots[0].TraceIDs, "trace-b")
	require.Equal(t, "healthy", historyBody.Data.Snapshots[len(historyBody.Data.Snapshots)-1].Status)
}
