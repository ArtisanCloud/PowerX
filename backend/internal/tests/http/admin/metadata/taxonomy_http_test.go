package metadata_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	adminhttp "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/metadata"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/metadata"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestTaxonomyHTTPCreateNodeAndList(t *testing.T) {
	db := newHTTPTaxonomyTestDB(t)
	tenantUUID := uuid.New().String()
	router := gin.New()
	protected := router.Group("/api/v1")
	protected.Use(func(c *gin.Context) {
		ctx := reqctx.WithTenantUUID(c.Request.Context(), tenantUUID)
		ctx = reqctx.WithIsRoot(ctx, true)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	adminhttp.RegisterAPIRoutes(router.Group("/api/v1"), protected, &shared.Deps{DB: db})

	createTaxonomy := map[string]any{
		"namespace": "corex.product.category",
		"module":    "corex.product",
		"name_i18n": map[string]string{"zh-CN": "商品分类"},
		"max_depth": 3,
	}
	raw, _ := json.Marshal(createTaxonomy)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/metadata/taxonomies", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected taxonomy create 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	var created struct {
		Data struct {
			Payload struct {
				UUID string `json:"uuid"`
			} `json:"payload"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode taxonomy response: %v body=%s", err, rec.Body.String())
	}
	if created.Data.Payload.UUID == "" {
		t.Fatalf("expected taxonomy uuid body=%s", rec.Body.String())
	}

	createNode := map[string]any{
		"code":       "root",
		"label_i18n": map[string]string{"zh-CN": "根"},
	}
	raw, _ = json.Marshal(createNode)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/admin/metadata/taxonomies/"+created.Data.Payload.UUID+"/nodes", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected node create 201, got %d body=%s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/metadata/taxonomies/"+created.Data.Payload.UUID+"/nodes", nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected node list 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("root")) {
		t.Fatalf("expected response to include node code, body=%s", rec.Body.String())
	}
}

func newHTTPTaxonomyTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	gin.SetMode(gin.TestMode)
	oldSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = oldSchema })
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.Taxonomy{}, &model.TaxonomyNode{}, &model.Reference{}); err != nil {
		t.Fatalf("migrate taxonomy models: %v", err)
	}
	return db
}
