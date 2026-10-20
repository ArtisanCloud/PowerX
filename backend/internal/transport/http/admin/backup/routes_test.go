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

func TestBackupJobListRejectsUnknownViewBeforeDatabaseAccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("POWERX_OPS_BACKUP_ARTIFACT_DIR", t.TempDir())
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(reqctx.WithIsRoot(c.Request.Context(), true))
		c.Next()
	})
	RegisterAPIRoutes(nil, r.Group("/custom"), &shared.Deps{DB: &gorm.DB{Config: &gorm.Config{}}})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/custom/admin/ops/backup/jobs?view=delete", nil))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("unknown view: status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestDatabaseDownloadRootAndOneTimeInputBoundary(t *testing.T) {
	t.Setenv("POWERX_OPS_BACKUP_ARTIFACT_DIR", t.TempDir())
	for _, root := range []bool{false, true} {
		r := gin.New()
		if root {
			r.Use(func(c *gin.Context) {
				c.Request = c.Request.WithContext(reqctx.WithIsRoot(c.Request.Context(), true))
				c.Next()
			})
		}
		RegisterAPIRoutes(nil, r.Group("/custom"), &shared.Deps{DB: &gorm.DB{Config: &gorm.Config{}}})
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", "/custom/admin/ops/backup/jobs/0/download", nil))
		want := http.StatusForbidden
		if root {
			want = http.StatusBadRequest
		}
		if w.Code != want {
			t.Fatalf("download root=%v: %d %s", root, w.Code, w.Body.String())
		}
		if root {
			for _, body := range []string{`{}`, `{"policy_id":"1","one_time":true}`, `{"policy_id":"invalid"}`} {
				w = httptest.NewRecorder()
				r.ServeHTTP(w, httptest.NewRequest("POST", "/custom/admin/ops/backup/jobs/run", bytes.NewBufferString(body)))
				if w.Code != 400 {
					t.Fatalf("ambiguous one-time input: %d %s", w.Code, w.Body.String())
				}
			}
		}
	}
}
