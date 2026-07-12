package agenttracecontract

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	agenttraceapi "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/agenttrace"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAgentTraceRootOnlyContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api/v1")
	agenttraceapi.RegisterAPIRoutes(nil, group, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/agent-traces/messages/message-contract/report?tenant_uuid=tenant-contract&session_id=session-contract", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "AGENT_TRACE_ROOT_REQUIRED", body["message"])
}
