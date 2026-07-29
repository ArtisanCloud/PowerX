package workflowcontract

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	workflowhttp "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/workflow"
	"github.com/ArtisanCloud/PowerX/tests/workflow/testenv"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestWorkflowRuntimeHTTPContracts(t *testing.T) {
	chdirBackendRoot(t)
	gin.SetMode(gin.TestMode)
	env := testenv.New(t)

	engine := gin.New()
	public := engine.Group("/api")
	protected := engine.Group("/api")
	protected.Use(requireWorkflowAuth(testenv.ContractTenantUUID))
	workflowhttp.RegisterAPIRoutes(public, protected, &shared.Deps{
		Workflow: &shared.WorkflowDeps{Service: env.Service},
	})

	nodeCatalogResp := serveWorkflowRequest(t, engine, httptest.NewRequest(http.MethodGet, "/api/admin/workflows/node-catalog", nil), testenv.ContractTenantUUID)
	require.Equal(t, http.StatusOK, nodeCatalogResp.Code)
	requireResponseItems(t, nodeCatalogResp.Body.Bytes())

	nodeResp := serveWorkflowRequest(t, engine, httptest.NewRequest(http.MethodGet, "/api/admin/workflows/node-catalog/knowledge.publish", nil), testenv.ContractTenantUUID)
	require.Equal(t, http.StatusOK, nodeResp.Code)

	createBody := mustJSON(t, map[string]any{
		"name": "contract-runtime",
		"steps": []map[string]any{{
			"id":        "input",
			"type":      "system",
			"node_kind": "input.capture",
			"config":    map[string]any{},
		}},
	})
	createReq := httptest.NewRequest(http.MethodPost, "/api/admin/workflows/definitions", bytes.NewReader(createBody))
	createReq.Header.Set("Content-Type", "application/json")
	createResp := serveWorkflowRequest(t, engine, createReq, testenv.ContractTenantUUID)
	require.Equal(t, http.StatusCreated, createResp.Code)
	definitionUUID := responseDataString(t, createResp.Body.Bytes(), "uuid")
	require.NotEmpty(t, definitionUUID)

	validateReq := httptest.NewRequest(http.MethodPost, "/api/admin/workflows/definitions/"+definitionUUID+"/validate", bytes.NewReader(mustJSON(t, map[string]any{
		"steps": []map[string]any{{
			"id":        "input",
			"type":      "system",
			"node_kind": "input.capture",
			"config":    map[string]any{},
		}},
	})))
	validateReq.Header.Set("Content-Type", "application/json")
	validateResp := serveWorkflowRequest(t, engine, validateReq, testenv.ContractTenantUUID)
	require.Equal(t, http.StatusOK, validateResp.Code)

	publishReq := httptest.NewRequest(http.MethodPost, "/api/admin/workflows/definitions/"+definitionUUID+"/publish", bytes.NewReader([]byte(`{}`)))
	publishReq.Header.Set("Content-Type", "application/json")
	publishResp := serveWorkflowRequest(t, engine, publishReq, testenv.ContractTenantUUID)
	require.Equal(t, http.StatusOK, publishResp.Code)

	listResp := serveWorkflowRequest(t, engine, httptest.NewRequest(http.MethodGet, "/api/admin/workflows/definitions?page_size=10&offset=0", nil), testenv.ContractTenantUUID)
	require.Equal(t, http.StatusOK, listResp.Code)
	requireResponseItems(t, listResp.Body.Bytes())

	startBody := mustJSON(t, map[string]any{"definition_uuid": definitionUUID, "input": map[string]any{"source": "contract"}})
	startReq := httptest.NewRequest(http.MethodPost, "/api/admin/workflows/instances", bytes.NewReader(startBody))
	startReq.Header.Set("Content-Type", "application/json")
	startResp := serveWorkflowRequest(t, engine, startReq, testenv.ContractTenantUUID)
	require.Equal(t, http.StatusAccepted, startResp.Code)
	instanceUUID := responseDataString(t, startResp.Body.Bytes(), "uuid")
	require.NotEmpty(t, instanceUUID)

	instanceResp := serveWorkflowRequest(t, engine, httptest.NewRequest(http.MethodGet, "/api/admin/workflows/instances/"+instanceUUID+"?include_steps=true", nil), testenv.ContractTenantUUID)
	require.Equal(t, http.StatusOK, instanceResp.Code)

	reviewResp := serveWorkflowRequest(t, engine, httptest.NewRequest(http.MethodGet, "/api/admin/workflows/review-tasks?page=1&page_size=10", nil), testenv.ContractTenantUUID)
	require.Equal(t, http.StatusOK, reviewResp.Code)

	seedReq := httptest.NewRequest(http.MethodPost, "/api/admin/workflows/packs/seed", bytes.NewReader([]byte(`{"keys":["marketing_knowledge_capture"]}`)))
	seedReq.Header.Set("Content-Type", "application/json")
	seedResp := serveWorkflowRequest(t, engine, seedReq, testenv.ContractTenantUUID)
	require.Equal(t, http.StatusOK, seedResp.Code)

	packListResp := serveWorkflowRequest(t, engine, httptest.NewRequest(http.MethodGet, "/api/admin/workflows/packs?page_size=10&offset=0", nil), testenv.ContractTenantUUID)
	require.Equal(t, http.StatusOK, packListResp.Code)
	requireResponseItems(t, packListResp.Body.Bytes())

	packResp := serveWorkflowRequest(t, engine, httptest.NewRequest(http.MethodGet, "/api/admin/workflows/packs/marketing_knowledge_capture", nil), testenv.ContractTenantUUID)
	require.Equal(t, http.StatusOK, packResp.Code)
}

func chdirBackendRoot(t *testing.T) {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "../../.."))
	current, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(root))
	t.Cleanup(func() { require.NoError(t, os.Chdir(current)) })
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	require.NoError(t, err)
	return raw
}

func requireResponseItems(t *testing.T, raw []byte) {
	t.Helper()
	var resp struct {
		Data struct {
			Items []any `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(raw, &resp))
	require.NotEmpty(t, resp.Data.Items)
}

func responseDataString(t *testing.T, raw []byte, key string) string {
	t.Helper()
	var resp struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(raw, &resp))
	value, _ := resp.Data[key].(string)
	return value
}
