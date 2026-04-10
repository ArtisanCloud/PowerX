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
	eventfabricmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
)

func TestWSBusGrantThenPublish(t *testing.T) {
	gin.SetMode(gin.TestMode)
	bus.SetDynamicTopicCompatEnabledForTest(true)
	t.Cleanup(func() { bus.SetDynamicTopicCompatEnabledForTest(false) })

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

	// Grant topic actions.
	grantBody, _ := json.Marshal(map[string]any{
		"topics": []string{"custom.progress"},
	})
	grantReq := httptest.NewRequest(http.MethodPost, "/api/v1/internal/ws-bus/grant", bytes.NewReader(grantBody))
	grantReq.Header.Set("Content-Type", "application/json")
	grantRec := httptest.NewRecorder()
	router.ServeHTTP(grantRec, grantReq)
	if grantRec.Code != http.StatusOK {
		t.Fatalf("grant failed: status=%d body=%s", grantRec.Code, grantRec.Body.String())
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
	bus.SetDynamicTopicCompatEnabledForTest(true)
	t.Cleanup(func() { bus.SetDynamicTopicCompatEnabledForTest(false) })

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

type topicLookupStub struct {
	topic *eventfabricmodel.TopicDefinition
	err   error
}

func (s topicLookupStub) FindByComposite(_ *gin.Context, _, _, _ string) (*eventfabricmodel.TopicDefinition, error) {
	return s.topic, s.err
}

func TestWSBusGrantRegistryMissFallbackToDynamic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	bus.SetDynamicTopicCompatEnabledForTest(true)
	t.Cleanup(func() { bus.SetDynamicTopicCompatEnabledForTest(false) })

	originHub := bus.DefaultHub
	bus.DefaultHub = bus.NewHub()
	t.Cleanup(func() { bus.DefaultHub = originHub })

	tenantUUID := "tenant-ws-bus"
	h := &wsBusHandler{topics: topicLookupStub{}}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := reqctx.WithTenantUUID(context.Background(), tenantUUID)
		ctx = reqctx.WithIsRoot(ctx, true)
		c.Request = c.Request.WithContext(ctx)
		reqctx.CopyCtxToGin(c)
		c.Next()
	})
	router.POST("/api/v1/internal/ws-bus/grant", h.grant)
	router.POST("/api/v1/internal/ws-bus/publish", h.publish)

	grantBody, _ := json.Marshal(map[string]any{
		"topics": []string{"custom.runtime.control"},
	})
	grantReq := httptest.NewRequest(http.MethodPost, "/api/v1/internal/ws-bus/grant", bytes.NewReader(grantBody))
	grantReq.Header.Set("Content-Type", "application/json")
	grantRec := httptest.NewRecorder()
	router.ServeHTTP(grantRec, grantReq)
	if grantRec.Code != http.StatusOK {
		t.Fatalf("grant failed: status=%d body=%s", grantRec.Code, grantRec.Body.String())
	}

	publishBody, _ := json.Marshal(map[string]any{
		"topic":   "custom.runtime.control",
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
