package knowledge_space_contract

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ArtisanCloud/PowerX/tests/knowledge_space/testenv"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestKnowledgeSourcesHTTPFlow(t *testing.T) {
	env := testenv.New(t)
	t.Cleanup(env.Close)

	tplID := env.SeedPolicyTemplate("http-sources", "v1")
	space := env.CreateSpaceFixture("http-sources-space", tplID)
	engine := env.Engine()

	type credential struct {
		ID         string `json:"id"`
		TenantUUID string `json:"tenant_uuid"`
		Provider   string `json:"provider"`
		Label      string `json:"label"`
		Status     string `json:"status"`
		MaskedHint string `json:"maskedHint"`
	}
	type connector struct {
		ID           string `json:"id"`
		TenantUUID   string `json:"tenant_uuid"`
		Provider     string `json:"provider"`
		CredentialID string `json:"credentialId"`
		Status       string `json:"status"`
		LastError    string `json:"lastError"`
	}
	type syncJob struct {
		ID          string `json:"id"`
		TenantUUID  string `json:"tenant_uuid"`
		SpaceID     string `json:"spaceId"`
		Provider    string `json:"provider"`
		ConnectorID string `json:"connectorId"`
		Status      string `json:"status"`
		Scope       any    `json:"scope"`
		LastRunAt   string `json:"lastRunAt"`
		LastOKAt    string `json:"lastOkAt"`
		LastError   string `json:"lastError"`
		LastRunRef  string `json:"lastRunRef"`
	}

	// 1) Create a tenant-level credential.
	credBody := map[string]any{
		"provider": "notion",
		"authType": "token",
		"label":    "notion-dev",
		// 留空 token：连接器会返回占位文档单元，确保 HTTP 合同可在无外部依赖下跑通。
		"token":     "",
		"createdBy": "ops@powerx.local",
	}
	credPayload, _ := json.Marshal(credBody)
	credReq := httptest.NewRequest(http.MethodPost, "/api/admin/knowledge-sources/credentials", bytes.NewReader(credPayload))
	credReq.Header.Set("Content-Type", "application/json")
	credResp := serveKnowledgeRequest(t, engine, credReq, env.TenantUUID().String())
	require.Equal(t, http.StatusCreated, credResp.Code)

	var createdCred apiResponse[credential]
	require.NoError(t, json.Unmarshal(credResp.Body.Bytes(), &createdCred))
	require.Equal(t, http.StatusCreated, createdCred.Code)
	require.NotEmpty(t, createdCred.Data.ID)
	require.Equal(t, env.TenantUUID().String(), createdCred.Data.TenantUUID)
	require.Equal(t, "notion", createdCred.Data.Provider)
	require.Equal(t, "notion-dev", createdCred.Data.Label)
	require.Equal(t, "active", createdCred.Data.Status)

	// 2) List credentials should be tenant-isolated.
	listReq := httptest.NewRequest(http.MethodGet, "/api/admin/knowledge-sources/credentials?provider=notion", nil)
	listResp := serveKnowledgeRequest(t, engine, listReq, env.TenantUUID().String())
	require.Equal(t, http.StatusOK, listResp.Code)

	var creds apiResponse[[]credential]
	require.NoError(t, json.Unmarshal(listResp.Body.Bytes(), &creds))
	require.GreaterOrEqual(t, len(creds.Data), 1)

	otherTenant := uuid.NewString()
	otherListReq := httptest.NewRequest(http.MethodGet, "/api/admin/knowledge-sources/credentials?provider=notion", nil)
	otherListResp := serveKnowledgeRequest(t, engine, otherListReq, otherTenant)
	require.Equal(t, http.StatusOK, otherListResp.Code)
	var otherCreds apiResponse[[]credential]
	require.NoError(t, json.Unmarshal(otherListResp.Body.Bytes(), &otherCreds))
	require.Len(t, otherCreds.Data, 0)

	// 3) Create connector instance bound to the credential.
	connBody := map[string]any{
		"provider":      "notion",
		"credentialId":  createdCred.Data.ID,
		"config":        map[string]any{"mode": "database"},
		"createdBy":     "ops@powerx.local",
		"webhookKeyRef": "",
	}
	connPayload, _ := json.Marshal(connBody)
	connReq := httptest.NewRequest(http.MethodPost, "/api/admin/knowledge-sources/connectors", bytes.NewReader(connPayload))
	connReq.Header.Set("Content-Type", "application/json")
	connResp := serveKnowledgeRequest(t, engine, connReq, env.TenantUUID().String())
	require.Equal(t, http.StatusCreated, connResp.Code)

	var createdConn apiResponse[connector]
	require.NoError(t, json.Unmarshal(connResp.Body.Bytes(), &createdConn))
	require.Equal(t, http.StatusCreated, createdConn.Code)
	require.NotEmpty(t, createdConn.Data.ID)
	require.Equal(t, "notion", createdConn.Data.Provider)
	require.Equal(t, createdCred.Data.ID, createdConn.Data.CredentialID)
	require.Equal(t, "active", createdConn.Data.Status)

	// 4) Update connector config (minimal "update" surface).
	patchBody := map[string]any{
		"config":    map[string]any{"mode": "database", "databaseId": "db_123"},
		"updatedBy": "ops@powerx.local",
	}
	patchPayload, _ := json.Marshal(patchBody)
	patchReq := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/admin/knowledge-sources/connectors/%s", createdConn.Data.ID), bytes.NewReader(patchPayload))
	patchReq.Header.Set("Content-Type", "application/json")
	patchResp := serveKnowledgeRequest(t, engine, patchReq, env.TenantUUID().String())
	require.Equal(t, http.StatusOK, patchResp.Code)

	// 5) Create a space-level sync job.
	jobBody := map[string]any{
		"provider":    "notion",
		"connectorId": createdConn.Data.ID,
		"syncMode":    "incremental",
		"schedule":    "@hourly",
		"scope": map[string]any{
			"databaseId": "db_123",
		},
		"createdBy": "ops@powerx.local",
	}
	jobPayload, _ := json.Marshal(jobBody)
	jobReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/admin/knowledge-spaces/%s/sources/sync-jobs", space.UUID), bytes.NewReader(jobPayload))
	jobReq.Header.Set("Content-Type", "application/json")
	jobResp := serveKnowledgeRequest(t, engine, jobReq, env.TenantUUID().String())
	require.Equal(t, http.StatusCreated, jobResp.Code)

	var createdJob apiResponse[syncJob]
	require.NoError(t, json.Unmarshal(jobResp.Body.Bytes(), &createdJob))
	require.Equal(t, http.StatusCreated, createdJob.Code)
	require.NotEmpty(t, createdJob.Data.ID)
	require.Equal(t, space.UUID.String(), createdJob.Data.SpaceID)
	require.Equal(t, "active", createdJob.Data.Status)

	// 6) Run sync job should enqueue an ingestion job and update last_run_*.
	runBody := map[string]any{
		"requestedBy": "ops@powerx.local",
	}
	runPayload, _ := json.Marshal(runBody)
	runReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/admin/knowledge-spaces/%s/sources/sync-jobs/%s/run", space.UUID, createdJob.Data.ID), bytes.NewReader(runPayload))
	runReq.Header.Set("Content-Type", "application/json")
	runResp := serveKnowledgeRequest(t, engine, runReq, env.TenantUUID().String())
	require.Equal(t, http.StatusAccepted, runResp.Code)

	var runResult apiResponse[map[string]any]
	require.NoError(t, json.Unmarshal(runResp.Body.Bytes(), &runResult))
	require.NotEmpty(t, runResult.Data["sync_job_id"])

	// 7) Query job detail includes last run summary.
	getReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/admin/knowledge-spaces/%s/sources/sync-jobs/%s", space.UUID, createdJob.Data.ID), nil)
	getResp := serveKnowledgeRequest(t, engine, getReq, env.TenantUUID().String())
	require.Equal(t, http.StatusOK, getResp.Code)

	var got apiResponse[syncJob]
	require.NoError(t, json.Unmarshal(getResp.Body.Bytes(), &got))
	require.Equal(t, createdJob.Data.ID, got.Data.ID)
	require.NotEmpty(t, got.Data.LastRunAt)
	require.NotEmpty(t, got.Data.LastRunRef)
	require.True(t, got.Data.LastError == "" || got.Data.LastError == "degraded", "expected empty or degraded error marker")

	// 8) Pause job (disable for now).
	pauseBody := map[string]any{
		"reason":      "maintenance",
		"requestedBy": "ops@powerx.local",
	}
	pausePayload, _ := json.Marshal(pauseBody)
	pauseReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/admin/knowledge-spaces/%s/sources/sync-jobs/%s/pause", space.UUID, createdJob.Data.ID), bytes.NewReader(pausePayload))
	pauseReq.Header.Set("Content-Type", "application/json")
	pauseResp := serveKnowledgeRequest(t, engine, pauseReq, env.TenantUUID().String())
	require.Equal(t, http.StatusOK, pauseResp.Code)
}

