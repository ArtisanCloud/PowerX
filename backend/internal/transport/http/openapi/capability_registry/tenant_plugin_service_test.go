package capability_registry

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	cap "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	core "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	cm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	sm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestTrustedPluginServiceHTTPDoesNotInjectProxyCredentials(t *testing.T) {
	old := core.PowerXSchema
	core.PowerXSchema = "main"
	t.Cleanup(func() { core.PowerXSchema = old })
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&cm.CapabilityRecord{}, &cm.CapabilityRegistration{}, &sm.PluginInstanceConfig{}))
	capID := "com.example.messages.reply"
	protocols, _ := json.Marshal([]cm.ProtocolBinding{{Channel: "rest", Method: "POST", Endpoint: "/api/v1/service/messages/reply", AuthType: cap.PluginServiceAuthType, ActorContext: "service_actor", ResourceScope: "tenant"}})
	require.NoError(t, db.Create(&cm.CapabilityRecord{CapabilityID: capID, PluginID: "com.example.messages", PluginVersion: "1.0.0", Title: "Reply", Status: "published", Protocols: protocols}).Error)
	require.NoError(t, db.Create(&cm.CapabilityRegistration{CapabilityID: capID, TenantUUID: apiKeyCatalogTestTenant, Status: "published", Version: 1, ContractRef: "1.0.0", RoutingPolicyID: uuid.New()}).Error)
	credential := sm.PluginInstanceConfig{TenantUUID: apiKeyCatalogTestTenant, PluginID: "com.example.ai", Key: "auth.credentials", Enabled: true, ValueJSON: datatypes.JSON(`{"client_id":"ai","allowed_capabilities":["` + capID + `"]}`)}
	require.NoError(t, db.Create(&credential).Error)
	invoker := &apiKeyCatalogInvoker{}
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(reqctx.WithClaims(reqctx.WithTenantUUID(c.Request.Context(), apiKeyCatalogTestTenant), &reqctx.CoreXClaims{TenantUUID: apiKeyCatalogTestTenant, PluginID: "com.example.ai", RegisteredClaims: jwt.RegisteredClaims{Issuer: "powerx-sts", Subject: "client:ai", Audience: []string{"powerx:api"}, ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute))}}))
		c.Next()
	})
	RegisterTenantRoutes(engine.Group("/api/v1"), &shared.Deps{DB: db, CapabilityCatalogSvc: cap.NewRegistryService(cap.RegistryServiceOptions{DB: db}), CapabilitySelector: cap.NewSelector(cap.SelectorOptions{Invoker: invoker})})
	call := func() *httptest.ResponseRecorder {
		raw, _ := json.Marshal(map[string]any{"capability_id": capID, "preferred_protocol": "rest", "payload": map[string]any{"body": map[string]any{"text": "hello"}}})
		r := httptest.NewRequest(http.MethodPost, "/api/v1/tenant/invocations", bytes.NewReader(raw))
		r.Header.Set("Authorization", "Bearer fixture-caller-credential")
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, r)
		return w
	}
	w := call()
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Len(t, invoker.input.Payload, 1)
	require.NotContains(t, invoker.input.Payload, "headers")
	require.NotContains(t, invoker.input.Payload, "endpoint")
	require.NoError(t, db.Model(&credential).Update("value_json", datatypes.JSON(`{"client_id":"ai","allowed_capabilities":[]}`)).Error)
	w = call()
	require.Equal(t, 403, w.Code, w.Body.String())
}
