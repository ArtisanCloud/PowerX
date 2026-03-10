package skillsintegration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	skillservice "github.com/ArtisanCloud/PowerX/internal/service/skills"
	adminskills "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/skills"
	skillmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/skills"
	skillrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/skills"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSkillAuditManagementActionsComplete(t *testing.T) {
	db := setupSkillsDB(t)
	registryRepo := skillrepo.NewSkillRegistryRepository(db)
	traceRepo := skillrepo.NewSkillExecutionTraceRepository(db)
	auditRepo := skillrepo.NewSkillLifecycleAuditRepository(db)
	auditSvc := skillservice.NewAuditTraceService(traceRepo, auditRepo)
	importSvc := skillservice.NewImportService(registryRepo, auditSvc)
	lifecycleSvc := skillservice.NewLifecycleService(registryRepo, auditSvc)
	invokeSvc := skillservice.NewInvokeService(registryRepo, auditSvc)

	// import
	_, err := importSvc.ImportDraft(context.Background(), skillservice.ImportRequest{
		SkillID:    "skill.audit.flow",
		Version:    "1.0.0",
		Source:     skillmodel.SkillSourcePlugin,
		BundleURI:  "s3://skills/skill.audit.flow-1.0.0.tgz",
		Checksum:   "sha256:audit-flow-100",
		ImportType: skillservice.ImportTypeUpload,
		Operator:   "tester",
	})
	require.NoError(t, err)
	require.NoError(t, lifecycleSvc.Publish(context.Background(), "skill.audit.flow", "1.0.0", "tester", "publish v1"))

	// prepare v2 then publish/rollback
	require.NoError(t, db.Create(&skillmodel.SkillRegistryRecord{
		SkillID:    "skill.audit.flow",
		Version:    "2.0.0",
		Source:     skillmodel.SkillSourcePlugin,
		Status:     skillmodel.SkillStatusDraft,
		BundleURI:  "s3://skills/skill.audit.flow-2.0.0.tgz",
		Checksum:   "sha256:audit-flow-200",
		ImportType: "upload",
		UpdatedBy:  "seed",
	}).Error)
	require.NoError(t, lifecycleSvc.Publish(context.Background(), "skill.audit.flow", "2.0.0", "tester", "publish v2"))
	require.NoError(t, lifecycleSvc.Rollback(context.Background(), "skill.audit.flow", "1.0.0", "tester", "rollback v1"))

	// bind action through handler endpoint
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

	bindBody := map[string]interface{}{
		"version":       "1.0.0",
		"capability_id": "cap.audit.flow",
		"tool_grants":   []string{"grant.read"},
	}
	raw, _ := json.Marshal(bindBody)
	bindReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/skills/skill.audit.flow/bind-capability", bytes.NewReader(raw))
	bindReq.Header.Set("Content-Type", "application/json")
	bindResp := httptest.NewRecorder()
	engine.ServeHTTP(bindResp, bindReq)
	require.Equal(t, http.StatusOK, bindResp.Code)

	// invoke trace
	_, err = invokeSvc.Resolve(context.Background(), skillservice.InvokeRequest{
		TenantUUID: "tenant-audit",
		SkillID:    "skill.audit.flow",
		InvokePath: "tenant.skills.invoke",
	})
	require.NoError(t, err)

	audits, err := auditRepo.ListBySkill(context.Background(), "skill.audit.flow", 20)
	require.NoError(t, err)
	actionSet := map[string]bool{}
	for _, audit := range audits {
		actionSet[audit.Action] = true
	}
	require.True(t, actionSet["import"])
	require.True(t, actionSet["publish"])
	require.True(t, actionSet["rollback"])
	require.True(t, actionSet["bind"])

	traces, err := traceRepo.List(context.Background(), skillrepo.SkillExecutionTraceFilter{
		SkillID: "skill.audit.flow",
		Limit:   10,
	})
	require.NoError(t, err)
	require.NotEmpty(t, traces)
}
