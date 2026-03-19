package skillscontract

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	skillshttp "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/skills"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	skillmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/skills"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestHTTPAdminSkillsContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	deps := setupSkillsDeps(t)
	seedSkillsFixtures(t, deps.DB)

	engine := gin.New()
	protected := engine.Group("/api/v1")
	protected.Use(func(c *gin.Context) {
		claims := &reqctx.CoreXClaims{
			MemberUUID: "root-admin",
			IsRoot:     true,
			Roles:      []string{"system_admin"},
		}
		ctx := reqctx.WithClaims(c.Request.Context(), claims)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	skillshttp.RegisterAPIRoutes(engine.Group("/api/v1"), protected, deps)

	t.Run("catalog list", func(t *testing.T) {
		resp := doJSONRequest(t, engine, http.MethodGet, "/api/v1/admin/skills/catalog", nil)
		require.Equal(t, http.StatusOK, resp.Code)
		var body map[string]interface{}
		require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &body))
		data := body["data"].(map[string]interface{})
		items := data["items"].([]interface{})
		require.NotEmpty(t, items)
	})

	t.Run("register and list", func(t *testing.T) {
		payload := map[string]interface{}{
			"skill_id": "skill.contract.http",
			"version":  "1.0.0",
			"source":   "plugin",
			"bundle_ref": map[string]interface{}{
				"uri":      "s3://skills/contract-http-1.0.0.tgz",
				"checksum": "sha256-contract-http-100",
			},
			"manifest": map[string]interface{}{"name": "contract-http"},
		}
		createResp := doJSONRequest(t, engine, http.MethodPost, "/api/v1/admin/skills", payload)
		require.Equal(t, http.StatusCreated, createResp.Code)

		listResp := doJSONRequest(t, engine, http.MethodGet, "/api/v1/admin/skills?skill_id=skill.contract.http", nil)
		require.Equal(t, http.StatusOK, listResp.Code)
		var body map[string]interface{}
		require.NoError(t, json.Unmarshal(listResp.Body.Bytes(), &body))
		data := body["data"].(map[string]interface{})
		items := data["items"].([]interface{})
		require.Len(t, items, 1)
	})

	t.Run("publish and rollback", func(t *testing.T) {
		publishResp := doJSONRequest(t, engine, http.MethodPost, "/api/v1/admin/skills/skill.lifecycle.contract/publish", map[string]interface{}{
			"version":       "2.0.0",
			"approval_note": "approved",
		})
		require.Equal(t, http.StatusOK, publishResp.Code)

		rollbackResp := doJSONRequest(t, engine, http.MethodPost, "/api/v1/admin/skills/skill.lifecycle.contract/rollback", map[string]interface{}{
			"target_version": "1.0.0",
			"reason":         "manual rollback",
		})
		require.Equal(t, http.StatusOK, rollbackResp.Code)

		var v1, v2 skillmodel.SkillRegistryRecord
		require.NoError(t, deps.DB.WithContext(context.Background()).
			Where("skill_id = ? AND version = ?", "skill.lifecycle.contract", "1.0.0").
			Take(&v1).Error)
		require.NoError(t, deps.DB.WithContext(context.Background()).
			Where("skill_id = ? AND version = ?", "skill.lifecycle.contract", "2.0.0").
			Take(&v2).Error)
		require.True(t, v1.IsLatestPublished)
		require.False(t, v2.IsLatestPublished)
	})

	t.Run("list traces by plan id", func(t *testing.T) {
		require.NoError(t, deps.DB.Create(&skillmodel.SkillExecutionTrace{
			TraceID:                "trace-contract-plan-001",
			TenantUUID:             "tenant-contract",
			SkillID:                "flow.contract.alpha",
			Version:                "1.0.0",
			Entrypoint:             "task-contract-1",
			ProtocolUsed:           "agent",
			InvokePath:             "agent.invoke.plan",
			Status:                 "completed",
			PlanID:                 "plan-contract-001",
			NodeID:                 "task-contract-1",
			NodeStatus:             "completed",
			RetryTrace:             "",
			FallbackUsed:           false,
			AuthorizationCheckPass: true,
		}).Error)

		resp := doJSONRequest(t, engine, http.MethodGet, "/api/v1/admin/skills/traces?plan_id=plan-contract-001&limit=20", nil)
		require.Equal(t, http.StatusOK, resp.Code)

		var body map[string]interface{}
		require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &body))
		data := body["data"].(map[string]interface{})
		items := data["items"].([]interface{})
		require.NotEmpty(t, items)
		first := items[0].(map[string]interface{})
		require.Equal(t, "plan-contract-001", first["plan_id"])
		require.Equal(t, "task-contract-1", first["node_id"])
		require.Equal(t, "completed", first["node_status"])
	})
}

func doJSONRequest(t *testing.T, h http.Handler, method, path string, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	var raw []byte
	if body != nil {
		var err error
		raw, err = json.Marshal(body)
		require.NoError(t, err)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	h.ServeHTTP(resp, req)
	return resp
}

func setupSkillsDeps(t *testing.T) *shared.Deps {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&parseTime=true&_loc=UTC"), &gorm.Config{})
	require.NoError(t, err)

	prevSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = prevSchema })

	require.NoError(t, db.AutoMigrate(
		&skillmodel.SkillRegistryRecord{},
		&skillmodel.OfficialSkillCatalogEntry{},
		&skillmodel.SkillCapabilityBinding{},
		&skillmodel.SkillExecutionTrace{},
		&skillmodel.SkillLifecycleAudit{},
	))
	return &shared.Deps{DB: db}
}

func seedSkillsFixtures(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Create(&skillmodel.OfficialSkillCatalogEntry{
		CatalogSkillID:     "catalog.summary",
		SkillID:            "skill.catalog.summary",
		RecommendedVersion: "1.2.0",
		RiskLevel:          "L2",
		Category:           "knowledge",
		Summary:            "official skill",
		Active:             true,
	}).Error)

	require.NoError(t, db.Create(&skillmodel.SkillRegistryRecord{
		SkillID:           "skill.lifecycle.contract",
		Version:           "1.0.0",
		Source:            skillmodel.SkillSourcePlugin,
		Status:            skillmodel.SkillStatusPublished,
		IsLatestPublished: true,
		BundleURI:         "s3://skills/skill.lifecycle.contract-1.0.0.tgz",
		Checksum:          "sha256-lifecycle-100",
		ImportType:        "upload",
		UpdatedBy:         "seed",
	}).Error)
	require.NoError(t, db.Create(&skillmodel.SkillRegistryRecord{
		SkillID:           "skill.lifecycle.contract",
		Version:           "2.0.0",
		Source:            skillmodel.SkillSourcePlugin,
		Status:            skillmodel.SkillStatusDraft,
		IsLatestPublished: false,
		BundleURI:         "s3://skills/skill.lifecycle.contract-2.0.0.tgz",
		Checksum:          "sha256-lifecycle-200",
		ImportType:        "upload",
		UpdatedBy:         "seed",
	}).Error)
}
