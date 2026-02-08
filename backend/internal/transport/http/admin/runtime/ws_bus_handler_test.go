package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/transport/websocket/bus"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
)

func TestWSBusRegisterThenPublish(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Reset hub for test isolation.
	originHub := bus.DefaultHub
	bus.DefaultHub = bus.NewHub()
	t.Cleanup(func() { bus.DefaultHub = originHub })

	tenantUUID := "tenant-ws-bus"

	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := reqctx.WithTenantUUID(context.Background(), tenantUUID)
		ctx = reqctx.WithIsRoot(ctx, true)
		c.Request = c.Request.WithContext(ctx)
		reqctx.CopyCtxToGin(c)
		c.Next()
	})
	protectedGroup := router.Group("/api/v1")
	RegisterAPIRoutes(nil, protectedGroup, nil)

	// Register dynamic topic.
	registerBody, _ := json.Marshal(map[string]any{
		"topics": []string{"custom.progress"},
	})
	registerReq := httptest.NewRequest(http.MethodPost, "/api/v1/internal/ws-bus/register", bytes.NewReader(registerBody))
	registerReq.Header.Set("Content-Type", "application/json")
	registerRec := httptest.NewRecorder()
	router.ServeHTTP(registerRec, registerReq)
	if registerRec.Code != http.StatusOK {
		t.Fatalf("register failed: status=%d body=%s", registerRec.Code, registerRec.Body.String())
	}

	// Publish to dynamic topic.
	publishBody, _ := json.Marshal(map[string]any{
		"topic":   "custom.progress",
		"payload": map[string]any{"ok": true},
	})
	publishReq := httptest.NewRequest(http.MethodPost, "/api/v1/internal/ws-bus/publish", bytes.NewReader(publishBody))
	publishReq.Header.Set("Content-Type", "application/json")
	publishRec := httptest.NewRecorder()
	router.ServeHTTP(publishRec, publishReq)
	if publishRec.Code != http.StatusOK {
		t.Fatalf("publish failed: status=%d body=%s", publishRec.Code, publishRec.Body.String())
	}
}

func TestWSBusPublishRejectsUnregisteredTopic(t *testing.T) {
	gin.SetMode(gin.TestMode)

	originHub := bus.DefaultHub
	bus.DefaultHub = bus.NewHub()
	t.Cleanup(func() { bus.DefaultHub = originHub })

	tenantUUID := "tenant-ws-bus"
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := reqctx.WithTenantUUID(context.Background(), tenantUUID)
		ctx = reqctx.WithIsRoot(ctx, true)
		c.Request = c.Request.WithContext(ctx)
		reqctx.CopyCtxToGin(c)
		c.Next()
	})
	protectedGroup := router.Group("/api/v1")
	RegisterAPIRoutes(nil, protectedGroup, nil)

	publishBody, _ := json.Marshal(map[string]any{
		"topic":   "custom.unregistered",
		"payload": map[string]any{"ok": true},
	})
	publishReq := httptest.NewRequest(http.MethodPost, "/api/v1/internal/ws-bus/publish", bytes.NewReader(publishBody))
	publishReq.Header.Set("Content-Type", "application/json")
	publishRec := httptest.NewRecorder()
	router.ServeHTTP(publishRec, publishReq)
	if publishRec.Code != http.StatusForbidden {
		t.Fatalf("expected forbidden, got status=%d body=%s", publishRec.Code, publishRec.Body.String())
	}
}
