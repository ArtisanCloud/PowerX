package skillsintegration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	skillsopenapi "github.com/ArtisanCloud/PowerX/internal/transport/http/openapi/skills"
	skillmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/skills"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

func TestSkillInvokeDirectPath(t *testing.T) {
	db := setupSkillsDB(t)
	require.NoError(t, db.Create(&skillmodel.SkillRegistryRecord{
		SkillID:           "skill.invoke.incident-triage",
		Version:           "1.0.0",
		Source:            skillmodel.SkillSourcePlugin,
		Status:            skillmodel.SkillStatusPublished,
		IsLatestPublished: true,
		BundleURI:         "s3://skills/skill.invoke.incident-triage-1.0.0.tgz",
		Checksum:          "sha256:invoke-direct",
		ImportType:        "upload",
		UpdatedBy:         "seed",
	}).Error)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	group := engine.Group("/api")
	group.Use(func(c *gin.Context) {
		ctx := reqctx.WithTenantUUID(c.Request.Context(), "tenant-us3-direct")
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	skillsopenapi.RegisterTenantRoutes(group, &shared.Deps{DB: db})

	body := map[string]interface{}{
		"skill_id": "skill.invoke.incident-triage",
		"payload":  map[string]interface{}{"incident_id": "INC-1001", "context": "database warning"},
	}
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/tenant/skills/invoke", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)
	require.Equal(t, http.StatusOK, resp.Code)

	var data map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &data))
	payload := data["data"].(map[string]interface{})
	require.Equal(t, "skill", payload["protocol_used"])
	require.Equal(t, "completed", payload["status"])
	require.NotEmpty(t, payload["trace_id"])
}

func TestSkillInvokeDirectPromptTemplateExecutor(t *testing.T) {
	db := setupSkillsDB(t)
	manifest := datatypes.JSON([]byte(`{
		"name":"Prompt Template Renderer",
		"description":"Render prompt template",
		"version":"1.0.0",
		"schema":"powerx.skill-manifest.v1",
		"entrypoints":["runbook.default"]
	}`))
	require.NoError(t, db.Create(&skillmodel.SkillRegistryRecord{
		SkillID:           "skill.thirdparty.prompt-template",
		Version:           "1.0.0",
		Source:            skillmodel.SkillSourceThirdParty,
		Status:            skillmodel.SkillStatusPublished,
		IsLatestPublished: true,
		BundleURI:         "s3://skills/thirdparty/prompt-template/1.0.0.tgz",
		Checksum:          "sha256:thirdparty-prompt-template",
		ManifestJSON:      manifest,
		ImportType:        "upload",
		UpdatedBy:         "seed",
	}).Error)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	group := engine.Group("/api")
	group.Use(func(c *gin.Context) {
		ctx := reqctx.WithTenantUUID(c.Request.Context(), "tenant-us3-direct")
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	skillsopenapi.RegisterTenantRoutes(group, &shared.Deps{DB: db})

	body := map[string]interface{}{
		"skill_id": "skill.thirdparty.prompt-template",
		"payload": map[string]interface{}{
			"template": "Incident {{ticket}} assigned to {{owner}}",
			"variables": map[string]interface{}{
				"ticket": "PX-101",
				"owner":  "ops",
			},
		},
	}
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/tenant/skills/invoke", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)
	require.Equal(t, http.StatusOK, resp.Code)

	var data map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &data))
	payload := data["data"].(map[string]interface{})
	result := payload["result"].(map[string]interface{})
	require.Equal(t, "Incident PX-101 assigned to ops", result["rendered_text"])
	require.Equal(t, "prompt-template", result["executor"])
	require.Equal(t, false, payload["fallback_used"])
}
