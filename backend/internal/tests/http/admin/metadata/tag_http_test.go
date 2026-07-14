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

type httpTagValidator struct{}

func (httpTagValidator) ValidateResource(context.Context, string, string) error {
	return nil
}

func TestTagHTTPCreateReplaceBindingAndList(t *testing.T) {
	db := newHTTPTagTestDB(t)
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
			"product_validator": httpTagValidator{},
		}),
	})

	createResourceType := map[string]any{
		"resource_type":   "product.sku",
		"module":          "corex.product",
		"name_i18n":       map[string]string{"zh-CN": "商品"},
		"validator_key":   "product_validator",
		"binding_enabled": true,
	}
	raw, _ := json.Marshal(createResourceType)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/metadata/resource-types", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected resource type create 201, got %d body=%s", rec.Code, rec.Body.String())
	}

	createTag := map[string]any{
		"namespace":     "corex.product",
		"resource_type": "product.sku",
		"code":          "featured",
		"label_i18n":    map[string]string{"zh-CN": "推荐"},
	}
	raw, _ = json.Marshal(createTag)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/admin/metadata/tags", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected tag create 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	var created struct {
		Data struct {
			Payload struct {
				UUID string `json:"uuid"`
			} `json:"payload"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode tag response: %v body=%s", err, rec.Body.String())
	}
	if created.Data.Payload.UUID == "" {
		t.Fatalf("expected tag uuid body=%s", rec.Body.String())
	}

	resourceUUID := uuid.New().String()
	replace := map[string]any{"resource_type": "product.sku", "resource_uuid": resourceUUID, "tag_uuids": []string{created.Data.Payload.UUID}}
	raw, _ = json.Marshal(replace)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/api/v1/admin/metadata/tag-bindings", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected binding replace 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/metadata/tag-bindings?resource_type=product.sku&resource_uuid="+resourceUUID, nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected binding list 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("featured")) {
		t.Fatalf("expected response to include tag code, body=%s", rec.Body.String())
	}
}

func newHTTPTagTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	gin.SetMode(gin.TestMode)
	oldSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = oldSchema })
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.Tag{}, &model.TagBinding{}, &model.ResourceType{}); err != nil {
		t.Fatalf("migrate tag models: %v", err)
	}
	return db
}
