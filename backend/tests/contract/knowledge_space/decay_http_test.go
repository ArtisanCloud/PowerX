package knowledge_space_contract

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/ArtisanCloud/PowerX/tests/knowledge_space/testenv"
	"github.com/stretchr/testify/require"
)

func TestDecayHTTPFlow(t *testing.T) {
	env := testenv.New(t)
	t.Cleanup(env.Close)

	tpl := env.SeedPolicyTemplate("decay-http", "v1")
	space := env.CreateSpaceFixture("decay-primary", tpl)
	otherSpace := env.CreateSpaceFixture("decay-secondary", tpl)
	engine := env.Engine()

	var createdTasks []string

	t.Run("create decay tasks", func(t *testing.T) {
		payload := map[string]any{
			"spaceId":  space.UUID.String(),
			"detected": 2,
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/api/knowledge/decay/tasks", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp := serveKnowledgeRequest(t, engine, req, env.TenantUUID().String())
		require.Equal(t, http.StatusCreated, resp.Code)

		var apiResp struct {
			Data struct {
				Tasks []struct {
					UUID      string `json:"uuid"`
					SpaceUUID string `json:"space_uuid"`
					Status    string `json:"status"`
				} `json:"tasks"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &apiResp))
		tasksPayload := apiResp.Data.Tasks
		require.Len(t, tasksPayload, 2)
		for _, task := range tasksPayload {
			require.Equal(t, space.UUID.String(), task.SpaceUUID)
			require.Equal(t, "open", task.Status)
			createdTasks = append(createdTasks, task.UUID)
		}
	})

	t.Run("list decay tasks", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/knowledge/decay/status?spaceId=%s", space.UUID), nil)
		resp := serveKnowledgeRequest(t, engine, req, env.TenantUUID().String())
		require.Equal(t, http.StatusOK, resp.Code)

		var apiResp struct {
			Data struct {
				Tasks []struct {
					UUID   string `json:"uuid"`
					Status string `json:"status"`
				} `json:"tasks"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &apiResp))
		require.Len(t, apiResp.Data.Tasks, 2)
	})

	t.Run("restore task", func(t *testing.T) {
		payload := map[string]any{
			"taskId":        createdTasks[0],
			"notes":         "false alert",
			"falsePositive": true,
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/api/knowledge/decay/restore", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp := serveKnowledgeRequest(t, engine, req, env.TenantUUID().String())
		require.Equal(t, http.StatusOK, resp.Code)

		var apiResp struct {
			Data struct {
				UUID          string `json:"uuid"`
				Status        string `json:"status"`
				FalsePositive bool   `json:"false_positive"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &apiResp))
		require.Equal(t, createdTasks[0], apiResp.Data.UUID)
		require.Equal(t, "closed", apiResp.Data.Status)
		require.True(t, apiResp.Data.FalsePositive)
	})

	t.Run("status reflects open backlog", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/knowledge/decay/status?spaceId=%s", space.UUID), nil)
		resp := serveKnowledgeRequest(t, engine, req, env.TenantUUID().String())
		require.Equal(t, http.StatusOK, resp.Code)

		var apiResp struct {
			Data struct {
				Tasks []struct {
					UUID   string `json:"uuid"`
					Status string `json:"status"`
				} `json:"tasks"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &apiResp))
		require.Len(t, apiResp.Data.Tasks, 1)
		require.Equal(t, createdTasks[1], apiResp.Data.Tasks[0].UUID)
		require.Equal(t, "open", apiResp.Data.Tasks[0].Status)
	})

	t.Run("tenant isolation", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/knowledge/decay/status?spaceId=%s", otherSpace.UUID), nil)
		resp := serveKnowledgeRequest(t, engine, req, env.TenantUUID().String())
		require.Equal(t, http.StatusOK, resp.Code)

		var apiResp struct {
			Data struct {
				Tasks []struct {
					UUID string `json:"uuid"`
				} `json:"tasks"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &apiResp))
		require.Len(t, apiResp.Data.Tasks, 0)
	})

	t.Run("missing tenant header rejected", func(t *testing.T) {
		payload := map[string]any{
			"spaceId":  space.UUID.String(),
			"detected": 1,
		}
		body, _ := json.Marshal(payload)
		missingReq := httptest.NewRequest(http.MethodPost, "/api/knowledge/decay/tasks", bytes.NewReader(body))
		missingReq.Header.Set("Content-Type", "application/json")
		missingReq.Header.Set("Authorization", "Bearer token")
		missingResp := httptest.NewRecorder()
		engine.ServeHTTP(missingResp, missingReq)
		require.Equal(t, http.StatusUnauthorized, missingResp.Code)
	})

	t.Run("metrics snapshot recorded", func(t *testing.T) {
		data, err := os.ReadFile(env.DecayReportPath)
		require.NoError(t, err)

		var snapshot struct {
			Detected         int            `json:"detected"`
			FalsePositive    int            `json:"falsePositive"`
			AverageFillHours float64        `json:"avgFillHours"`
			Metrics          map[string]any `json:"metrics"`
		}
		require.NoError(t, json.Unmarshal(data, &snapshot))
		require.Equal(t, 1, snapshot.FalsePositive)
		require.InDelta(t, 0, snapshot.AverageFillHours, 0.001)
		require.Contains(t, snapshot.Metrics, "knowledge.decay.detected")
		require.Contains(t, snapshot.Metrics, "knowledge.decay.false_positive")
		require.Contains(t, snapshot.Metrics, "knowledge.gap.backlog")
		require.Contains(t, snapshot.Metrics, "knowledge.decay.fill_time_hours")
	})
}
