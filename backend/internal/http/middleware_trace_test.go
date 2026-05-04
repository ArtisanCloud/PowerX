package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestTraceInjectionMiddleware_InjectsPluginIDForIntegrationV2Path(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(TraceInjectionMiddleware())
	r.GET("/api/v2/integration/demo-plugin/webhooks/shopify", func(c *gin.Context) {
		v, ok := c.Request.Context().Value("plugin_id").(string)
		if !ok {
			t.Fatalf("plugin_id not injected into context")
		}
		if v != "com.powerx.plugins.demo-plugin" {
			t.Fatalf("unexpected plugin_id: %s", v)
		}
		if c.GetHeader("X-Request-ID") == "" && c.GetHeader("X-Request-Id") == "" {
			// no-op: request headers may be empty in this test; response headers must be set by middleware.
		}
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v2/integration/demo-plugin/webhooks/shopify", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("unexpected status code: %d", w.Code)
	}
	if w.Header().Get("X-Request-ID") == "" {
		t.Fatalf("expected X-Request-ID in response header")
	}
	if w.Header().Get("X-Trace-ID") == "" {
		t.Fatalf("expected X-Trace-ID in response header")
	}
}

