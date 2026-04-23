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

func TestSkillTraceListByA2ATeamAndHandoff(t *testing.T) {
	db := setupSkillsDB(t)
	require.NoError(t, db.Create(&skillmodel.SkillExecutionTrace{
		TraceID:                "trace-a2a-team-1-task-a",
		TenantUUID:             "tenant-a",
		SkillID:                "agent.retriever",
		ProtocolUsed:           "agent.agent_handoff",
		InvokePath:             "agent.invoke.plan",
		Status:                 "completed",
		PlanID:                 "plan-a2a-1",
		NodeID:                 "n1",
		TeamID:                 "team-1",
		HandoffTaskID:          "task-a",
		HandoffTraceID:         "handoff-trace-a",
		NodeStatus:             "completed",
		FallbackUsed:           false,
		AuthorizationCheckPass: true,
	}).Error)
	require.NoError(t, db.Create(&skillmodel.SkillExecutionTrace{
		TraceID:                "trace-a2a-team-1-task-b",
		TenantUUID:             "tenant-a",
		SkillID:                "agent.reviewer",
		ProtocolUsed:           "agent.agent_handoff",
		InvokePath:             "agent.invoke.plan",
		Status:                 "failed",
		PlanID:                 "plan-a2a-1",
		NodeID:                 "n2",
		TeamID:                 "team-1",
		HandoffTaskID:          "task-b",
		HandoffTraceID:         "handoff-trace-b",
		NodeStatus:             "failed",
		RetryTrace:             "timeout",
		FallbackUsed:           false,
		AuthorizationCheckPass: true,
	}).Error)
	require.NoError(t, db.Create(&skillmodel.SkillExecutionTrace{
		TraceID:                "trace-a2a-team-2-task-a",
		TenantUUID:             "tenant-b",
		SkillID:                "agent.retriever",
		ProtocolUsed:           "agent.agent_handoff",
		InvokePath:             "agent.invoke.plan",
		Status:                 "completed",
		PlanID:                 "plan-a2a-2",
		NodeID:                 "n3",
		TeamID:                 "team-2",
		HandoffTaskID:          "task-a",
		HandoffTraceID:         "handoff-trace-c",
		NodeStatus:             "completed",
		FallbackUsed:           false,
		AuthorizationCheckPass: true,
	}).Error)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	protected := engine.Group("/api/v1")
	protected.Use(func(c *gin.Context) {
		claims := &reqctx.CoreXClaims{
			MemberUUID: "root-admin",
			IsRoot:     true,
			Roles:      []string{"system_admin"},
			TenantUUID: "tenant-a",
		}
		ctx := reqctx.WithClaims(c.Request.Context(), claims)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	adminskills.RegisterAPIRoutes(engine.Group("/api/v1"), protected, &shared.Deps{DB: db})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/skills/traces?team_id=team-1&handoff_task_id=task-b&limit=20", nil)
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)
	require.Equal(t, http.StatusOK, resp.Code)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &payload))
	data, _ := payload["data"].(map[string]any)
	items, _ := data["items"].([]any)
	require.Len(t, items, 1)
	row, _ := items[0].(map[string]any)
	require.Equal(t, "team-1", row["team_id"])
	require.Equal(t, "task-b", row["handoff_task_id"])
	require.Equal(t, "failed", row["node_status"])

	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/admin/skills/traces?team_id=team-1&tenant_uuid=tenant-b&limit=20", nil)
	resp2 := httptest.NewRecorder()
	engine.ServeHTTP(resp2, req2)
	require.Equal(t, http.StatusOK, resp2.Code)
	require.NoError(t, json.Unmarshal(resp2.Body.Bytes(), &payload))
	data2, _ := payload["data"].(map[string]any)
	items2, _ := data2["items"].([]any)
	require.Len(t, items2, 0)
}
