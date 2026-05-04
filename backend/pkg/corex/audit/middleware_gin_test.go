package audit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	"github.com/gin-gonic/gin"
)

type captureAuditService struct {
	last *dbm.AuditEvent
}

func (c *captureAuditService) Emit(_ context.Context, evt *dbm.AuditEvent) error {
	c.last = evt
	return nil
}

func TestGinAudit_ResolvesPluginIDForIntegrationRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &captureAuditService{}
	r := gin.New()
	r.Use(GinAudit(NewAuditor(svc)))
	r.POST("/api/v2/integration/demo-plugin/webhooks/shopify", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v2/integration/demo-plugin/webhooks/shopify", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("unexpected status code: %d", w.Code)
	}
	if svc.last == nil {
		t.Fatalf("expected audit event to be emitted")
	}

	var meta map[string]any
	if err := json.Unmarshal(svc.last.Meta, &meta); err != nil {
		t.Fatalf("unmarshal meta failed: %v", err)
	}
	if _, exists := meta["plugin_id"]; exists {
		t.Fatalf("plugin_id should not be duplicated in meta")
	}
	if _, exists := meta["request_id"]; exists {
		t.Fatalf("request_id should not be duplicated in meta")
	}
	if _, exists := meta["trace_id"]; exists {
		t.Fatalf("trace_id should not be duplicated in meta")
	}
	if _, exists := meta["tenant_uuid"]; exists {
		t.Fatalf("tenant_uuid should not be duplicated in meta")
	}
	if _, exists := meta["status"]; !exists {
		t.Fatalf("status should remain in meta as extended diagnostic field")
	}
	if _, exists := meta["latency_ms"]; !exists {
		t.Fatalf("latency_ms should remain in meta as extended diagnostic field")
	}
}
