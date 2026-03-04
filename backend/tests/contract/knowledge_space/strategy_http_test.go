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

type strategyValidateResponse struct {
	OK              bool                   `json:"ok"`
	SceneKey        string                 `json:"sceneKey"`
	BundleKey       string                 `json:"bundleKey"`
	EnabledChannels []string               `json:"enabledChannels"`
	Missing         []map[string]any       `json:"missing"`
	Capabilities    map[string]bool        `json:"capabilities"`
	CheckedAt       string                 `json:"checkedAt"`
}

type errorResponse struct {
	Code      int                    `json:"code"`
	Message   string                 `json:"message"`
	Error     string                 `json:"error"`
	ErrorCode string                 `json:"error_code"`
	Details   map[string]interface{} `json:"details"`
}

func TestStrategyCatalogAndPrereqHTTP(t *testing.T) {
	env := testenv.New(t)
	t.Cleanup(env.Close)

	engine := env.Engine()

	policyID := env.SeedPolicyTemplate("default", "v1")

	t.Run("contract scene defaults to P2", func(t *testing.T) {
		payload := map[string]any{
			"sceneKey":  "contract_quote",
			"bundleKey": "",
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/api/admin/knowledge-spaces/strategy/validate", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp := serveKnowledgeRequest(t, engine, req, env.TenantUUID().String())
		require.Equalf(t, http.StatusOK, resp.Code, "body=%s", resp.Body.String())

		var out apiResponse[strategyValidateResponse]
		require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &out))
		require.Equal(t, "contract_quote", out.Data.SceneKey)
		require.Equal(t, "p2_high_accuracy", out.Data.BundleKey)
	})

	t.Run("kg scene defaults to P3", func(t *testing.T) {
		payload := map[string]any{
			"sceneKey":  "sql_kg",
			"bundleKey": "",
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/api/admin/knowledge-spaces/strategy/validate", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp := serveKnowledgeRequest(t, engine, req, env.TenantUUID().String())
		require.Equalf(t, http.StatusOK, resp.Code, "body=%s", resp.Body.String())

		var out apiResponse[strategyValidateResponse]
		require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &out))
		require.Equal(t, "sql_kg", out.Data.SceneKey)
		require.Equal(t, "p3_kg_strong", out.Data.BundleKey)
	})

	t.Run("contract scene forbids P0", func(t *testing.T) {
		payload := map[string]any{
			"sceneKey":  "contract_quote",
			"bundleKey": "p0_basic",
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/api/admin/knowledge-spaces/strategy/validate", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp := serveKnowledgeRequest(t, engine, req, env.TenantUUID().String())
		require.Equalf(t, http.StatusBadRequest, resp.Code, "body=%s", resp.Body.String())

		var out errorResponse
		require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &out))
		require.Equal(t, http.StatusBadRequest, out.Code)
		require.Contains(t, out.Message, "不允许")
	})

	t.Run("kg scene missing index blocks activate", func(t *testing.T) {
		// 默认测试环境不提供 KG 能力（index.kg=false），激活应被阻止。
		createBody := map[string]any{
			"spaceName":      "kg-space",
			"departmentCode": "IT",
			"quotas": map[string]any{
				"cpuCores":             2,
				"storageGb":            50,
				"ingestionConcurrency": 1,
			},
			"policyTemplateVersionId": fmt.Sprint(policyID),
			"ingestionProfileKey":     "p3_kg_strong",
			"indexProfileKey":         "p3_kg_strong",
			"ragProfileKey":           "p3_kg_strong",
			"featureFlags":            []string{"rag.scene:sql_kg", "rag.bundle:p3_kg_strong"},
		}
		createReq := newJSONRequest(http.MethodPost, "/api/admin/knowledge-spaces", createBody, env.TenantUUID().String())
		createResp := httptest.NewRecorder()
		engine.ServeHTTP(createResp, createReq)
		require.Equalf(t, http.StatusCreated, createResp.Code, "body=%s", createResp.Body.String())

		var created apiResponse[knowledgeSpaceView]
		require.NoError(t, json.Unmarshal(createResp.Body.Bytes(), &created))
		require.NotEmpty(t, created.Data.SpaceID)

		activateReq := newJSONRequest(
			http.MethodPatch,
			fmt.Sprintf("/api/admin/knowledge-spaces/%s", created.Data.SpaceID),
			map[string]any{"status": "active"},
			env.TenantUUID().String(),
		)
		activateResp := httptest.NewRecorder()
		engine.ServeHTTP(activateResp, activateReq)
		require.Equalf(t, http.StatusBadRequest, activateResp.Code, "body=%s", activateResp.Body.String())

		var out errorResponse
		require.NoError(t, json.Unmarshal(activateResp.Body.Bytes(), &out))
		require.Equal(t, http.StatusBadRequest, out.Code)
		require.NotEmpty(t, out.ErrorCode)
		require.Equal(t, "kg_required", out.ErrorCode)
		require.NotNil(t, out.Details)
	})
}
