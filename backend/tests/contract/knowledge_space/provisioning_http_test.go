package knowledge_space_contract

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/tests/knowledge_space/testenv"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestProvisioningHTTPFlow(t *testing.T) {
	env := testenv.New(t)
	t.Cleanup(env.Close)

	engine := env.Engine()
	policyID := env.SeedPolicyTemplate("default", "v1")

	createBody := map[string]any{
		"tenantId":       env.TenantID().String(),
		"spaceName":      "finance-ops",
		"departmentCode": "FIN-OPS",
		"quotas": map[string]any{
			"cpuCores":             4,
			"storageGb":            200,
			"ingestionConcurrency": 2,
		},
		"policyTemplateVersionId": fmt.Sprint(policyID),
		"featureFlags":            []string{"masking.strict", "fusion.guardrails"},
	}

	req := newJSONRequest(http.MethodPost, "/api/admin/knowledge-spaces", createBody)
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)
	require.Equal(t, http.StatusCreated, resp.Code)

	var createResp apiResponse[knowledgeSpaceView]
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &createResp))
	require.Equal(t, http.StatusCreated, createResp.Code)
	require.Equal(t, "finance-ops", createResp.Data.SpaceName)
	require.Equal(t, "pending_iam", createResp.Data.Status)
	require.Equal(t, "FIN-OPS", createResp.Data.DepartmentCode)
	require.Equal(t, int64(4), createResp.Data.Quotas.CPUCores)
	require.Equal(t, int64(200), createResp.Data.Quotas.StorageGB)
	require.Equal(t, 2, createResp.Data.Quotas.IngestionConcurrency)
	require.NotEmpty(t, createResp.Data.SpaceID)
	require.NotEmpty(t, createResp.Data.AuditToken)

	spaceID := createResp.Data.SpaceID

	updateBody := map[string]any{
		"quotas": map[string]any{
			"cpuCores":             6,
			"storageGb":            256,
			"ingestionConcurrency": 3,
		},
		"status": "active",
	}

	updateReq := newJSONRequest(http.MethodPatch, fmt.Sprintf("/api/admin/knowledge-spaces/%s", spaceID), updateBody)
	updateResp := httptest.NewRecorder()
	engine.ServeHTTP(updateResp, updateReq)
	require.Equal(t, http.StatusOK, updateResp.Code)

	var patched apiResponse[knowledgeSpaceView]
	require.NoError(t, json.Unmarshal(updateResp.Body.Bytes(), &patched))
	require.Equal(t, "active", patched.Data.Status)
	require.Equal(t, int64(6), patched.Data.Quotas.CPUCores)
	require.Equal(t, int64(256), patched.Data.Quotas.StorageGB)
	require.Equal(t, 3, patched.Data.Quotas.IngestionConcurrency)

	retireReq := newJSONRequest(http.MethodPost, fmt.Sprintf("/api/admin/knowledge-spaces/%s/retire", spaceID), map[string]any{
		"reason": "ops sunset",
	})
	retireResp := httptest.NewRecorder()
	engine.ServeHTTP(retireResp, retireReq)
	require.Equal(t, http.StatusAccepted, retireResp.Code)

	var retired apiResponse[knowledgeSpaceView]
	require.NoError(t, json.Unmarshal(retireResp.Body.Bytes(), &retired))
	require.Equal(t, "retired", retired.Data.Status)
	require.NotEmpty(t, retired.Data.RetentionExpiresAt)

	expiry, err := time.Parse(time.RFC3339, retired.Data.RetentionExpiresAt)
	require.NoError(t, err)
	require.True(t, expiry.After(time.Now()))

	dupReq := newJSONRequest(http.MethodPost, "/api/admin/knowledge-spaces", createBody)
	dupResp := httptest.NewRecorder()
	engine.ServeHTTP(dupResp, dupReq)
	require.Equal(t, http.StatusConflict, dupResp.Code)
}

func newJSONRequest(method, path string, body map[string]any) *http.Request {
	payload, _ := json.Marshal(body)
	req := httptest.NewRequest(method, path, bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", uuid.NewString())
	return req
}

type apiResponse[T any] struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

type knowledgeSpaceView struct {
	SpaceID             string     `json:"spaceId"`
	TenantID            string     `json:"tenantId"`
	SpaceName           string     `json:"spaceName"`
	DepartmentCode      string     `json:"departmentCode"`
	Status              string     `json:"status"`
	PolicyTemplateID    string     `json:"policyTemplateVersionId"`
	FeatureFlags        []string   `json:"featureFlags"`
	AuditToken          string     `json:"auditToken"`
	RetentionExpiresAt  string     `json:"retentionExpiresAt"`
	Quotas              quotaView  `json:"quotas"`
	IamStatus           string     `json:"iamStatus"`
	PendingIAMUpdatedAt *time.Time `json:"pendingIamUpdatedAt"`
}

type quotaView struct {
	CPUCores             int64 `json:"cpuCores"`
	StorageGB            int64 `json:"storageGb"`
	IngestionConcurrency int   `json:"ingestionConcurrency"`
}
