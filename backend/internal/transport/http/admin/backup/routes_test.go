package backup

import (
	"bytes"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBackupProtectionAdminBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("POWERX_OPS_BACKUP_ARTIFACT_DIR", t.TempDir())
	for _, prefix := range []string{"/api/v1/admin/ops/backup", "/api/v1/admin/backup"} {
		for _, root := range []bool{false, true} {
			r := gin.New()
			if root {
				r.Use(func(c *gin.Context) {
					c.Request = c.Request.WithContext(reqctx.WithIsRoot(c.Request.Context(), true))
					c.Next()
				})
			}
			RegisterAPIRoutes(nil, r.Group("/api/v1"), &shared.Deps{DB: &gorm.DB{Config: &gorm.Config{}}})
			w := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPatch, prefix+"/jobs/1/protection", bytes.NewBufferString(`{"protected":"wrong"}`))
			request.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, request)
			expected := http.StatusUnauthorized
			if root {
				expected = http.StatusBadRequest
			}
			if w.Code != expected {
				t.Fatalf("root=%v route=%s status=%d body=%s", root, prefix, w.Code, w.Body.String())
			}
		}
	}
}
