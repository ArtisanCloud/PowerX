package skillsintegration

import (
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

func TestSkillTraceQueryCrossTenantBlocked(t *testing.T) {
	db := setupSkillsDB(t)
	require.NoError(t, db.Create(&skillmodel.SkillExecutionTrace{
		TraceID:                "trace-skill-tenant-a",
		TenantUUID:             "tenant-a",
		SkillID:                "skill.tenant.iso",
		Version:                "1.0.0",
		Entrypoint:             "runbook.default",
		ProtocolUsed:           "skill",
		InvokePath:             "tenant.skills.invoke",
		Status:                 "completed",
		LatencyMS:              12,
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
			TenantUUID: "tenant-b",
		}
		ctx := reqctx.WithClaims(c.Request.Context(), claims)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	adminskills.RegisterAPIRoutes(engine.Group("/api/v1"), protected, &shared.Deps{DB: db})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/skills/traces/trace-skill-tenant-a?tenant_uuid=tenant-b", nil)
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)
	require.Equal(t, http.StatusNotFound, resp.Code)
}
