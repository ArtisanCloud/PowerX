package workflowcontract

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	workflowhttp "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/workflow"
	"github.com/ArtisanCloud/PowerX/tests/workflow/testenv"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestWorkflowHTTPRequiresAuthorization(t *testing.T) {
	gin.SetMode(gin.TestMode)
	env := testenv.New(t)

	engine := gin.New()
	public := engine.Group("/api")
	protected := engine.Group("/api")
	protected.Use(func(c *gin.Context) {
		if c.GetHeader("Authorization") == "" {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		c.Next()
	})
	deps := &shared.Deps{Workflow: &shared.WorkflowDeps{Service: env.Service}}
	workflowhttp.RegisterAPIRoutes(public, protected, deps)

	body, _ := json.Marshal(map[string]any{
		"tenant_id": 1001,
		"name":      "secure",
		"steps":     []map[string]any{{"id": "start", "type": "agent"}},
	})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/workflows/definitions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(rr, req)
	require.Equal(t, http.StatusUnauthorized, rr.Code)
}
