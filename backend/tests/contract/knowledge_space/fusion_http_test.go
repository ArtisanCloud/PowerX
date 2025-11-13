package knowledge_space_contract

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ArtisanCloud/PowerX/tests/knowledge_space/testenv"
	"github.com/stretchr/testify/require"
)

type fusionStrategyPayload struct {
	StrategyID      string  `json:"strategyId"`
	Label           string  `json:"label"`
	DeploymentState string  `json:"deploymentState"`
	ConflictPolicy  string  `json:"conflictPolicy"`
	BM25Weight      float64 `json:"bm25Weight"`
	VectorWeight    float64 `json:"vectorWeight"`
}

func TestFusionHTTPHandlers(t *testing.T) {
	env := testenv.New(t)
	t.Cleanup(env.Close)

	policyID := env.SeedPolicyTemplate("http-fusion", "v1")
	space := env.CreateSpaceFixture("http-fusion-space", policyID)
	engine := env.Engine()

	postStrategy := func(payload map[string]any, expectedCode int) fusionStrategyPayload {
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/admin/knowledge-spaces/%s/fusion-strategies", space.UUID), bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer token")
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		engine.ServeHTTP(resp, req)
		require.Equal(t, expectedCode, resp.Code)

		var apiResp struct {
			Data fusionStrategyPayload `json:"data"`
		}
		require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &apiResp))
		return apiResp.Data
	}

	t.Run("publish strategy promotes active version", func(t *testing.T) {
		strategy := postStrategy(map[string]any{
			"label":           "baseline",
			"bm25Weight":      0.4,
			"vectorWeight":    0.6,
			"graphConstraint": "tenant:default",
			"rerankerModel":   "cross-encoder-v1",
			"conflictPolicy":  "allow_with_flag",
		}, http.StatusCreated)
		require.Equal(t, "active", strategy.DeploymentState)
		require.NotEmpty(t, strategy.StrategyID)
	})

	t.Run("queue strategy when conflict policy is queue", func(t *testing.T) {
		strategy := postStrategy(map[string]any{
			"label":           "queued-version",
			"bm25Weight":      0.5,
			"vectorWeight":    0.5,
			"graphConstraint": "tenant:default",
			"rerankerModel":   "cross-encoder-v1",
			"conflictPolicy":  "queue",
		}, http.StatusAccepted)
		require.Equal(t, "draft", strategy.DeploymentState)
	})

	var strategyList []fusionStrategyPayload
	{
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/admin/knowledge-spaces/%s/fusion-strategies", space.UUID), nil)
		req.Header.Set("Authorization", "Bearer token")
		resp := httptest.NewRecorder()
		engine.ServeHTTP(resp, req)
		require.Equal(t, http.StatusOK, resp.Code)
		var apiResp struct {
			Data []fusionStrategyPayload `json:"data"`
		}
		require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &apiResp))
		require.NotEmpty(t, apiResp.Data)
		strategyList = apiResp.Data
	}

	t.Run("rollback previous version", func(t *testing.T) {
		target := strategyList[len(strategyList)-1]
		url := fmt.Sprintf("/api/admin/knowledge-spaces/%s/fusion-strategies/%s/rollback", space.UUID, target.StrategyID)
		req := httptest.NewRequest(http.MethodPost, url, nil)
		req.Header.Set("Authorization", "Bearer token")
		resp := httptest.NewRecorder()
		engine.ServeHTTP(resp, req)
		require.Equal(t, http.StatusOK, resp.Code)
		var apiResp struct {
			Data fusionStrategyPayload `json:"data"`
		}
		require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &apiResp))
		require.Equal(t, target.StrategyID, apiResp.Data.StrategyID)
		require.Equal(t, "active", apiResp.Data.DeploymentState)
	})
}
