package skillsintegration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	skillservice "github.com/ArtisanCloud/PowerX/internal/service/skills"
	settingmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	skillmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/skills"
	skillrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/skills"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

func TestSkillInvokeUnifiedPath(t *testing.T) {
	db := setupSkillsDB(t)
	require.NoError(t, db.Create(&skillmodel.SkillRegistryRecord{
		SkillID:           "skill.invoke.unified",
		Version:           "1.0.0",
		Source:            skillmodel.SkillSourcePlugin,
		Status:            skillmodel.SkillStatusPublished,
		IsLatestPublished: true,
		BundleURI:         "s3://skills/skill.invoke.unified-1.0.0.tgz",
		Checksum:          "sha256:invoke-unified",
		ImportType:        "upload",
		UpdatedBy:         "seed",
	}).Error)
	require.NoError(t, db.Create(&skillmodel.SkillCapabilityBinding{
		SkillID:         "skill.invoke.unified",
		Version:         "1.0.0",
		CapabilityID:    "cap.skill.unified",
		BindingStatus:   "active",
		VisibilityScope: "tenant",
		CreatedBy:       "seed",
		UpdatedBy:       "seed",
	}).Error)

	registryRepo := skillrepo.NewSkillRegistryRepository(db)
	bindingRepo := skillrepo.NewSkillCapabilityBindingRepository(db)
	traceRepo := skillrepo.NewSkillExecutionTraceRepository(db)
	auditRepo := skillrepo.NewSkillLifecycleAuditRepository(db)
	invokeSvc := skillservice.NewInvokeService(registryRepo, skillservice.NewAuditTraceService(traceRepo, auditRepo))
	adapterSvc := skillservice.NewAdapterService(invokeSvc, bindingRepo)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		ctx := reqctx.WithTenantUUID(c.Request.Context(), "tenant-us3-unified")
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	engine.POST("/api/tenant/invocations", func(c *gin.Context) {
		var req struct {
			CapabilityID      string                 `json:"capability_id"`
			PreferredProtocol string                 `json:"preferred_protocol"`
			Payload           map[string]interface{} `json:"payload"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		result, err := adapterSvc.InvokeUnified(c.Request.Context(), skillservice.UnifiedInvokeRequest{
			TenantUUID:        reqctx.GetTenantUUID(c.Request.Context()),
			CapabilityID:      req.CapabilityID,
			PreferredProtocol: req.PreferredProtocol,
			Payload:           req.Payload,
		})
		if err != nil {
			code, envelope := skillservice.MapInvokeError(err)
			c.JSON(code, envelope)
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"trace_id":      result.TraceID,
			"status":        result.Status,
			"protocol_used": result.ProtocolUsed,
			"result":        result.Result,
		})
	})

	body := map[string]interface{}{
		"capability_id":      "cap.skill.unified",
		"preferred_protocol": "skill",
		"payload":            map[string]interface{}{"input": "hello"},
	}
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/tenant/invocations", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)
	require.Equal(t, http.StatusOK, resp.Code)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &payload))
	require.Equal(t, "skill", payload["protocol_used"])
	require.Equal(t, "completed", payload["status"])
}

func TestSkillInvokeUnifiedPath_WithTenantSourcePolicy(t *testing.T) {
	db := setupSkillsDB(t)
	require.NoError(t, db.AutoMigrate(&settingmodel.TenantSetting{}))

	require.NoError(t, db.Create(&settingmodel.TenantSetting{
		TenantUUID: "tenant-us3-policy",
		Key:        skillservice.TenantSettingKeySkillSourceAllowlist,
		ValueJSON:  datatypes.JSON([]byte(`["builtin"]`)),
		Group:      "ai",
		Editable:   true,
	}).Error)

	require.NoError(t, db.Create(&skillmodel.SkillRegistryRecord{
		SkillID:           "skill.policy.builtin",
		Version:           "1.0.0",
		Source:            skillmodel.SkillSourceBuiltin,
		Status:            skillmodel.SkillStatusPublished,
		IsLatestPublished: true,
		BundleURI:         "s3://skills/skill.policy.builtin-1.0.0.tgz",
		Checksum:          "sha256:policy-builtin",
		ImportType:        "upload",
		UpdatedBy:         "seed",
	}).Error)
	require.NoError(t, db.Create(&skillmodel.SkillRegistryRecord{
		SkillID:           "skill.policy.plugin",
		Version:           "1.0.0",
		Source:            skillmodel.SkillSourcePlugin,
		Status:            skillmodel.SkillStatusPublished,
		IsLatestPublished: true,
		BundleURI:         "s3://skills/skill.policy.plugin-1.0.0.tgz",
		Checksum:          "sha256:policy-plugin",
		ImportType:        "upload",
		UpdatedBy:         "seed",
	}).Error)
	require.NoError(t, db.Create(&skillmodel.SkillCapabilityBinding{
		SkillID:         "skill.policy.builtin",
		Version:         "1.0.0",
		CapabilityID:    "cap.skill.policy",
		BindingStatus:   "active",
		VisibilityScope: "tenant",
		CreatedBy:       "seed",
		UpdatedBy:       "seed",
	}).Error)
	require.NoError(t, db.Create(&skillmodel.SkillCapabilityBinding{
		SkillID:         "skill.policy.plugin",
		Version:         "1.0.0",
		CapabilityID:    "cap.skill.policy",
		BindingStatus:   "active",
		VisibilityScope: "tenant",
		CreatedBy:       "seed",
		UpdatedBy:       "seed",
	}).Error)

	registryRepo := skillrepo.NewSkillRegistryRepository(db)
	bindingRepo := skillrepo.NewSkillCapabilityBindingRepository(db)
	traceRepo := skillrepo.NewSkillExecutionTraceRepository(db)
	auditRepo := skillrepo.NewSkillLifecycleAuditRepository(db)
	invokeSvc := skillservice.NewInvokeService(registryRepo, skillservice.NewAuditTraceService(traceRepo, auditRepo))
	adapterSvc := skillservice.NewAdapterService(invokeSvc, bindingRepo).
		WithSourcePolicyResolver(skillservice.NewDBSourcePolicyResolver(db))

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		ctx := reqctx.WithTenantUUID(c.Request.Context(), "tenant-us3-policy")
		ctx = reqctx.WithEnv(ctx, "default")
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	engine.POST("/api/tenant/invocations", func(c *gin.Context) {
		var req struct {
			CapabilityID      string                 `json:"capability_id"`
			PreferredProtocol string                 `json:"preferred_protocol"`
			Context           map[string]interface{} `json:"context"`
			Payload           map[string]interface{} `json:"payload"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		result, err := adapterSvc.InvokeUnified(c.Request.Context(), skillservice.UnifiedInvokeRequest{
			TenantUUID:        reqctx.GetTenantUUID(c.Request.Context()),
			Env:               reqctx.GetEnv(c.Request.Context()),
			CapabilityID:      req.CapabilityID,
			PreferredProtocol: req.PreferredProtocol,
			Context:           req.Context,
			Payload:           req.Payload,
		})
		if err != nil {
			code, envelope := skillservice.MapInvokeError(err)
			c.JSON(code, envelope)
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"trace_id":      result.TraceID,
			"status":        result.Status,
			"protocol_used": result.ProtocolUsed,
			"skill_id":      result.SkillID,
		})
	})

	body := map[string]interface{}{
		"capability_id":      "cap.skill.policy",
		"preferred_protocol": "skill",
		"context":            map[string]interface{}{"agent_id": 1001},
		"payload":            map[string]interface{}{"query": "plugin preferred query"},
	}
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/tenant/invocations", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)
	require.Equal(t, http.StatusOK, resp.Code)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &payload))
	require.Equal(t, "skill.policy.builtin", payload["skill_id"])
}
