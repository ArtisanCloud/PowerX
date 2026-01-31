//go:build ignore

package agentlifecycleintegration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	agentmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	"github.com/ArtisanCloud/PowerX/tests/agent_lifecycle/testenv"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestRetirementRetention(t *testing.T) {
	env := testenv.New(t)
	t.Cleanup(env.Close)

	engine := env.Engine()

	tenantUUID := "tenant-001"
	registerBody := map[string]any{
		"alias":                      "retire-agent",
		"telemetry_contract_version": "otel-agent-v1",
	}
	registerBytes, _ := json.Marshal(registerBody)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/agent/lifecycle/agents", bytes.NewReader(registerBytes))
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-PowerX-Tenant", tenantUUID)
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)
	require.Equal(t, http.StatusCreated, resp.Code)

	var registerResp struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &registerResp))
	agentID := registerResp.Data.ID

	retireBytes, _ := json.Marshal(map[string]any{})
	retireReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/admin/agent/lifecycle/agents/%s/retire", agentID), bytes.NewReader(retireBytes))
	retireReq.Header.Set("Authorization", "Bearer token")
	retireReq.Header.Set("Content-Type", "application/json")
	retireReq.Header.Set("X-PowerX-Tenant", tenantUUID)
	retireResp := httptest.NewRecorder()
	engine.ServeHTTP(retireResp, retireReq)
	require.Equal(t, http.StatusOK, retireResp.Code)

	agentUUID, err := uuid.Parse(agentID)
	require.NoError(t, err)

	var event agentmodel.AgentLifecycleEventRecord
	require.NoError(t, env.DB.Where("agent_uuid = ? AND to_status = ?", agentUUID, "retired").Last(&event).Error)

	// 假设 13 个月后仍需保留
	past := time.Now().AddDate(-1, -1, 0)
	require.NoError(t, env.DB.Model(&event).Update("occurred_at", past).Error)

	var fetched agentmodel.AgentLifecycleEventRecord
	require.NoError(t, env.DB.Where("uuid = ?", event.UUID).First(&fetched).Error)
	require.WithinDuration(t, past, fetched.OccurredAt, time.Second)
}
