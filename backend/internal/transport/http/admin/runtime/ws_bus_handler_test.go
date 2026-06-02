package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	aclservice "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/acl"
	"github.com/ArtisanCloud/PowerX/internal/transport/websocket/bus"
	"github.com/ArtisanCloud/PowerX/pkg/auth"
	"github.com/ArtisanCloud/PowerX/pkg/auth/middleware"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	eventfabricmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestWSBusGrantRejectsMissingRegistry(t *testing.T) {
	gin.SetMode(gin.TestMode)

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

	grantBody, _ := json.Marshal(map[string]any{
		"topics": []string{"custom.progress"},
	})
	grantReq := httptest.NewRequest(http.MethodPost, "/api/v1/internal/ws-bus/grant", bytes.NewReader(grantBody))
	grantReq.Header.Set("Content-Type", "application/json")
	grantRec := httptest.NewRecorder()
	router.ServeHTTP(grantRec, grantReq)
	if grantRec.Code != http.StatusInternalServerError {
		t.Fatalf("expected registry failure, got status=%d body=%s", grantRec.Code, grantRec.Body.String())
	}
}

func TestWSBusSTSGrantRejectsMissingRegistry(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const (
		tenantUUID = "11111111-1111-1111-1111-111111111111"
		secret     = "test-secret"
		issuer     = "powerx-auth"
	)

	token, err := auth.GenerateAccessJWT(reqctx.CoreXClaims{
		TenantUUID: tenantUUID,
		MemberUUID: "client:com.powerx.plugins.test." + tenantUUID,
	}, "powerx-sts", []string{"powerx:api"}, time.Minute, []byte(secret))
	if err != nil {
		t.Fatalf("GenerateAccessJWT error: %v", err)
	}

	router := gin.New()
	authMW := middleware.APIKeyOrJwtMiddleware(
		nil,
		[]byte(secret),
		issuer,
		[]string{"user"},
		[]string{"access"},
		nil,
		middleware.WithExtraTokenChecks(middleware.TokenCheck{
			Issuer:    "powerx-sts",
			Audiences: []string{"powerx:api"},
		}),
	)
	protectedGroup := router.Group("/api/v1")
	protectedGroup.Use(authMW)
	RegisterAPIRoutes(nil, protectedGroup, nil)

	grantBody, _ := json.Marshal(map[string]any{
		"topics": []string{"custom.sts.progress"},
	})
	grantReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/runtime/ws-bus/grant", bytes.NewReader(grantBody))
	grantReq.Header.Set("Content-Type", "application/json")
	grantReq.Header.Set("Authorization", "Bearer "+token)
	grantRec := httptest.NewRecorder()
	router.ServeHTTP(grantRec, grantReq)
	if grantRec.Code != http.StatusInternalServerError {
		t.Fatalf("expected registry failure, got status=%d body=%s", grantRec.Code, grantRec.Body.String())
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
	if publishRec.Code != http.StatusInternalServerError {
		t.Fatalf("expected authorizer failure, got status=%d body=%s", publishRec.Code, publishRec.Body.String())
	}
}

type topicLookupStub struct {
	topic         *eventfabricmodel.TopicDefinition
	templateTopic *eventfabricmodel.TopicDefinition
	createdTopic  *eventfabricmodel.TopicDefinition
	err           error
}

func (s topicLookupStub) FindByComposite(_ *gin.Context, _, _, _ string) (*eventfabricmodel.TopicDefinition, error) {
	return s.topic, s.err
}

func (s topicLookupStub) FindTemplateMatch(_ *gin.Context, _, _, _ string) (*eventfabricmodel.TopicDefinition, error) {
	return s.templateTopic, nil
}

func (s topicLookupStub) CreateFromTemplate(_ *gin.Context, template *eventfabricmodel.TopicDefinition, namespace, name, _ string) (*eventfabricmodel.TopicDefinition, error) {
	if s.createdTopic != nil {
		return s.createdTopic, nil
	}
	if template == nil {
		return nil, nil
	}
	return &eventfabricmodel.TopicDefinition{
		PowerUUIDModel: coremodel.PowerUUIDModel{UUID: uuid.New()},
		TenantKey:      template.TenantKey,
		ScopeID:        template.TenantKey,
		Namespace:      namespace,
		Name:           name,
	}, nil
}

func TestWSBusGrantRegistryMissRejectsTopic(t *testing.T) {
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
	if grantRec.Code != http.StatusNotFound {
		t.Fatalf("expected topic not found, got status=%d body=%s", grantRec.Code, grantRec.Body.String())
	}
}

func TestWSBusGrantInstantiatesApprovedRuntimeTemplate(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tenantUUID := "tenant-ws-bus"
	memberUUID := "member-fda3589b"
	topic := fmt.Sprintf("ai_craft.shopify.product.sync.progress.member.tenant_%s.member_%s", tenantUUID, memberUUID)
	h := &wsBusHandler{
		topics: topicLookupStub{
			templateTopic: &eventfabricmodel.TopicDefinition{
				PowerUUIDModel: coremodel.PowerUUIDModel{UUID: uuid.New()},
				TenantKey:      tenantUUID,
				Namespace:      "ai_craft.shopify.product.sync.progress.member.tenant_{{tenant_uuid}}",
				Name:           "member_{{member_uuid}}",
				PayloadFormat:  "json",
				VersioningMode: "strict",
				MaxRetry:       5,
				AckTimeoutSec:  30,
			},
		},
		acl: aclGrantStub{},
	}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := reqctx.WithTenantUUID(context.Background(), tenantUUID)
		ctx = reqctx.WithIsRoot(ctx, true)
		c.Request = c.Request.WithContext(ctx)
		reqctx.CopyCtxToGin(c)
		c.Next()
	})
	router.POST("/api/v1/internal/ws-bus/grant", h.grant)

	grantBody, _ := json.Marshal(map[string]any{
		"topics": []string{topic},
	})
	grantReq := httptest.NewRequest(http.MethodPost, "/api/v1/internal/ws-bus/grant", bytes.NewReader(grantBody))
	grantReq.Header.Set("Content-Type", "application/json")
	grantRec := httptest.NewRecorder()
	router.ServeHTTP(grantRec, grantReq)
	if grantRec.Code != http.StatusOK {
		t.Fatalf("grant failed: status=%d body=%s", grantRec.Code, grantRec.Body.String())
	}
}

type aclGrantStub struct{}

func (aclGrantStub) Grant(_ *gin.Context, _ aclservice.GrantRequest) ([]*aclservice.Binding, error) {
	return nil, nil
}

func TestWSBusAdminRuntimeGrantRejectsMissingRegistry(t *testing.T) {
	gin.SetMode(gin.TestMode)

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

	grantBody, _ := json.Marshal(map[string]any{
		"topics": []string{"custom.progress.admin_runtime"},
	})
	grantReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/runtime/ws-bus/grant", bytes.NewReader(grantBody))
	grantReq.Header.Set("Content-Type", "application/json")
	grantRec := httptest.NewRecorder()
	router.ServeHTTP(grantRec, grantReq)
	if grantRec.Code != http.StatusInternalServerError {
		t.Fatalf("expected registry failure, got status=%d body=%s", grantRec.Code, grantRec.Body.String())
	}
}
