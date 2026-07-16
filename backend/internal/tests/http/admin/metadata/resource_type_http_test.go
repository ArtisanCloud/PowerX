package metadata_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	metasvc "github.com/ArtisanCloud/PowerX/internal/service/metadata"
	adminhttp "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/metadata"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/metadata"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type httpResourceTypeValidator struct{}

func (httpResourceTypeValidator) ValidateResource(context.Context, string, string) error {
	return nil
}

func TestResourceTypeHTTPCreateListAndUpdate(t *testing.T) {
	db := newHTTPResourceTypeTestDB(t)
	tenantUUID := uuid.New().String()
	router := gin.New()
	protected := router.Group("/api/v1")
	protected.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(reqctx.WithTenantUUID(c.Request.Context(), tenantUUID))
		c.Next()
	})
	adminhttp.RegisterAPIRoutes(router.Group("/api/v1"), protected, &shared.Deps{
		DB: db,
		MetadataResourceValidatorRegistry: metasvc.NewStaticResourceValidatorRegistry(map[string]metasvc.ResourceValidator{
			"product_validator": httpResourceTypeValidator{},
		}),
	})

	create := map[string]any{
		"resource_type":   "product.sku",
		"module":          "corex.product",
		"name_i18n":       map[string]string{"zh-CN": "商品"},
		"validator_key":   "product_validator",
		"binding_enabled": true,
	}
	raw, _ := json.Marshal(create)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/metadata/resource-types", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected create 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	var created struct {
		Data struct {
			Payload struct {
				UUID            string `json:"uuid"`
				ResourceType    string `json:"resource_type"`
				ValidatorStatus string `json:"validator_status"`
			} `json:"payload"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v body=%s", err, rec.Body.String())
	}
	if created.Data.Payload.UUID == "" || created.Data.Payload.ResourceType != "product.sku" || created.Data.Payload.ValidatorStatus != metasvc.ValidatorStatusAvailable {
		t.Fatalf("unexpected create payload: %+v", created.Data.Payload)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/metadata/resource-types?module=corex.product&page=1&page_size=10", nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected list 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("product.sku")) {
		t.Fatalf("expected list body to include resource type, body=%s", rec.Body.String())
	}

	update := map[string]any{"binding_enabled": false}
	raw, _ = json.Marshal(update)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPatch, "/api/v1/admin/metadata/resource-types/"+created.Data.Payload.UUID, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected update 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestResourceTypeHTTPRejectsEnabledWithoutValidatorKey(t *testing.T) {
	db := newHTTPResourceTypeTestDB(t)
	tenantUUID := uuid.New().String()
	router := gin.New()
	protected := router.Group("/api/v1")
	protected.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(reqctx.WithTenantUUID(c.Request.Context(), tenantUUID))
		c.Next()
	})
	adminhttp.RegisterAPIRoutes(router.Group("/api/v1"), protected, &shared.Deps{DB: db})

	create := map[string]any{
		"resource_type":   "product.sku",
		"module":          "corex.product",
		"name_i18n":       map[string]string{"zh-CN": "商品"},
		"binding_enabled": true,
	}
	raw, _ := json.Marshal(create)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/metadata/resource-types", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected create conflict, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func newHTTPResourceTypeTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	gin.SetMode(gin.TestMode)
	oldSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = oldSchema })
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.ResourceType{}); err != nil {
		t.Fatalf("migrate resource type model: %v", err)
	}
	return db
}
