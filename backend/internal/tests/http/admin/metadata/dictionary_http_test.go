package metadata_test

import (
	"bytes"
	"context"
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

func TestDictionaryHTTPCreateAndList(t *testing.T) {
	db := newHTTPDictionaryTestDB(t)
	tenantUUID := uuid.New().String()
	router := gin.New()
	protected := router.Group("/api/v1")
	protected.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(reqctx.WithTenantUUID(c.Request.Context(), tenantUUID))
		c.Next()
	})
	adminhttp.RegisterAPIRoutes(router.Group("/api/v1"), protected, &shared.Deps{DB: db})

	body := map[string]any{
		"namespace": "corex.customer.level",
		"module":    "corex.customer",
		"name_i18n": map[string]string{"zh-CN": "客户等级"},
	}
	raw, _ := json.Marshal(body)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/metadata/dictionaries", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected create 201, got %d body=%s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/metadata/dictionaries?page=1&page_size=10", nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected list 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("corex.customer.level")) {
		t.Fatalf("expected response to include namespace, body=%s", rec.Body.String())
	}
}

func TestDictionaryHTTPRequiresTenantContext(t *testing.T) {
	db := newHTTPDictionaryTestDB(t)
	router := gin.New()
	adminhttp.RegisterAPIRoutes(router.Group("/api/v1"), router.Group("/api/v1"), &shared.Deps{DB: db})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/metadata/dictionaries", nil).WithContext(context.Background())
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected tenant context failure, got %d", rec.Code)
	}
}

func newHTTPDictionaryTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	gin.SetMode(gin.TestMode)
	oldSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = oldSchema })
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.DictionaryNamespace{}, &model.DictionaryItem{}, &model.Reference{}); err != nil {
		t.Fatalf("migrate metadata dictionary models: %v", err)
	}
	return db
}
