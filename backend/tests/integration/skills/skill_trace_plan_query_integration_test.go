package skillsintegration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	adminskills "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/skills"
	skillmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/skills"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSkillTraceListByPlanID(t *testing.T) {
	db := setupSkillsDB(t)
	require.NoError(t, db.Create(&skillmodel.SkillExecutionTrace{
		TraceID:                "trace-plan-1",
		TenantUUID:             "tenant-a",
		SkillID:                "flow.alpha",
		Version:                "",
		Entrypoint:             "t-alpha",
		ProtocolUsed:           "agent",
		InvokePath:             "agent.invoke.plan",
		Status:                 "completed",
		PlanID:                 "plan-test-001",
		NodeID:                 "t-alpha",
		NodeStatus:             "completed",
		RetryTrace:             "",
		FallbackUsed:           false,
		AuthorizationCheckPass: true,
	}).Error)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	protected := engine.Group("/api/v1")
	protected.Use(func(c *gin.Context) {
		claims := &reqctx.CoreXClaims{MemberUUID: "root-admin", IsRoot: true, Roles: []string{"system_admin"}}
		ctx := reqctx.WithClaims(c.Request.Context(), claims)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	adminskills.RegisterAPIRoutes(engine.Group("/api/v1"), protected, &shared.Deps{DB: db})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/skills/traces?plan_id=plan-test-001", nil)
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)
	require.Equal(t, http.StatusOK, resp.Code)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &payload))
	data, _ := payload["data"].(map[string]any)
	items, _ := data["items"].([]any)
	require.Len(t, items, 1)
	row, _ := items[0].(map[string]any)
	require.Equal(t, "plan-test-001", row["plan_id"])
	require.Equal(t, "t-alpha", row["node_id"])
	require.Equal(t, "completed", row["node_status"])
}
