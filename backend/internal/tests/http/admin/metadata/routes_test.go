package metadata_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	adminhttp "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/metadata"
	"github.com/gin-gonic/gin"
)

func TestMetadataRoutesRequireProtectedAdminContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	protected := r.Group("/api/v1")
	protected.Use(func(c *gin.Context) {
		c.AbortWithStatus(http.StatusUnauthorized)
	})
	adminhttp.RegisterAPIRoutes(r.Group("/api/v1"), protected, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/metadata/dictionaries", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected route to be protected, got %d", rec.Code)
	}
}
