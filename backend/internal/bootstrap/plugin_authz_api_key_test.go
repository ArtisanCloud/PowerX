package bootstrap

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	pmrouter "github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager/router"
	identity "github.com/ArtisanCloud/PowerX/internal/service/plugin_identity"
	"github.com/ArtisanCloud/PowerX/pkg/auth"
	"github.com/ArtisanCloud/PowerX/pkg/auth/middleware"
	capmodels "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	iammodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	gwmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/pluginpermission"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

type rejectingUserIdentityIssuer struct{ t *testing.T }

func (i rejectingUserIdentityIssuer) Issue(context.Context, identity.IssueInput) (string, error) {
	i.t.Fatal("a service Key must not request a user-session identity ticket")
	return "", nil
}

func TestPluginGatewayEvaluatesLiveAPIKeyServiceGrant(t *testing.T) {
	db := newPluginAuthzTestDB(t)
	require.NoError(t, db.AutoMigrate(&iammodel.APIKey{}, &iammodel.APIKeyProfile{}, &gwmodel.IntegrationGatewayAPIKey{}, &gwmodel.IntegrationGatewayAPIKeyPermission{}, &capmodels.CapabilityRecord{}, &capmodels.CapabilityRegistration{}))
	tenant := uuid.NewString()
	pluginID := "com.powerx.plugins.ecommerce"
	capabilityID := pluginID + ".product.spu.subscription.plan.read"
	profile := iammodel.APIKeyProfile{TenantUUID: tenant, Key: "query-only", Name: "query only", Status: 1}
	require.NoError(t, db.Create(&profile).Error)
	raw := uuid.NewString()
	sum := sha256.Sum256([]byte(raw))
	hash := hex.EncodeToString(sum[:])
	key := gwmodel.IntegrationGatewayAPIKey{TenantUUID: tenant, ProfileID: profile.ID, Name: "dedicated", KeyHash: hash, Status: "active"}
	require.NoError(t, db.Create(&key).Error)
	require.NoError(t, db.Create(&iammodel.APIKey{TenantUUID: tenant, ProfileID: profile.ID, KeyHash: hash}).Error)
	spec := &pluginpermission.APIKeySpec{CapabilityID: capabilityID, PluginID: pluginID, Scope: "_scope.ecommerce.subscription_plans.read", Action: "read", ResourceType: "capability", ResourcePattern: "subscription_plans", Effect: "allow"}
	meta, err := pluginpermission.ExplicitAPIKeyMetadata(spec, pluginID, capabilityID, true)
	require.NoError(t, err)
	meta["type"] = "api"
	meta["independent"] = true
	meta["protocol_bindings"] = []any{map[string]any{"channel": "rest", "method": "POST", "path": "/plugin/commerce/subscription-plans/query", "actor_context": "service_actor", "resource_scope": "tenant"}}
	metaRaw, err := json.Marshal(meta)
	require.NoError(t, err)
	definition := iammodel.Permission{Module: "commerce", Resource: "subscription_plans_service", Action: "read", Source: "plugin:" + pluginID, Status: iammodel.PermissionStatusActive, Effect: "allow", AllowAPIKey: true, Meta: datatypes.JSON(metaRaw)}
	require.NoError(t, db.Create(&definition).Error)
	// The Admin endpoint deliberately shares the code. A service grant still
	// cannot authorize its admin_user binding.
	adminMeta := datatypes.JSON([]byte(`{"type":"api","business_permission_code":"commerce.subscription_plans_service:read","protocol_bindings":[{"channel":"rest","method":"POST","path":"/admin/plans","actor_context":"admin_user","resource_scope":"tenant"}]}`))
	require.NoError(t, db.Create(&iammodel.Permission{Module: definition.Module, Resource: "admin_plans", Action: definition.Action, Source: definition.Source, Status: iammodel.PermissionStatusActive, Effect: "allow", Meta: adminMeta}).Error)
	capability := capmodels.CapabilityRecord{CapabilityID: capabilityID, PluginID: pluginID, PluginVersion: "0.3.1", Title: "Read", Status: "published", CapabilitiesHash: "fixture", ProtocolHash: "fixture"}
	require.NoError(t, db.Create(&capability).Error)
	registration := capmodels.CapabilityRegistration{CapabilityID: capabilityID, TenantUUID: tenant, Version: 1, Status: "published", ContractRef: "2.0.0"}
	require.NoError(t, db.Create(&registration).Error)
	grant := gwmodel.IntegrationGatewayAPIKeyPermission{APIKeyUUID: key.UUID, PluginID: pluginID, Scope: spec.Scope, Action: spec.Action, ResourceType: spec.ResourceType, ResourcePattern: spec.ResourcePattern, Effect: "allow"}
	require.NoError(t, db.Create(&grant).Error)

	gin.SetMode(gin.TestMode)
	secret := []byte("plugin-gateway-test")
	auth.SetJWTSecret(secret)
	var upstreamCalls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls.Add(1)
		claims, err := auth.ParseAndValidate(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), secret, "powerx-auth", "plugin:"+pluginID)
		require.NoError(t, err)
		require.Equal(t, tenant, claims.TenantUUID)
		require.EqualValues(t, 0, claims.UserID)
		require.False(t, claims.IsRoot)
		require.Contains(t, claims.Platforms, "api_key")
		require.Equal(t, []string{"commerce.subscription_plans_service:read"}, claims.PermissionCodes)
		require.Empty(t, r.Header.Get(identity.Header))
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(upstream.Close)
	upstreamURL, err := url.Parse(upstream.URL)
	require.NoError(t, err)
	engine := gin.New()
	dr := pmrouter.NewDynamicRouter("/_p", engine, middleware.APIKeyOrJwtMiddleware(db, secret, "powerx-auth", []string{"user"}, []string{"access"}, nil))
	dr.BindAuthorizer(pluginIAMAuthorizer{db: db}, "powerx-auth", time.Minute)
	dr.BindRequestIdentityIssuer(rejectingUserIdentityIssuer{t})
	dr.BindTenantPluginGuard(func(context.Context, string, string) error { return nil })
	dr.MountAPIProxy(pluginID, upstreamURL, "/api/v1", "/healthz")
	host := httptest.NewServer(engine)
	t.Cleanup(host.Close)
	request := func(route string) int {
		r, err := http.NewRequest(http.MethodPost, host.URL+"/_p/"+pluginID+"/api/v1"+route, nil)
		require.NoError(t, err)
		r.Header.Set("Authorization", "ApiKey "+raw)
		resp, err := host.Client().Do(r)
		require.NoError(t, err)
		defer resp.Body.Close()
		return resp.StatusCode
	}
	read := "/plugin/commerce/subscription-plans/query"
	require.Equal(t, http.StatusNoContent, request(read), "a per-Key grant works without any Profile permission or user role")
	require.Equal(t, http.StatusForbidden, request("/admin/plans"))
	for _, tc := range []struct {
		name, table, column string
		bad, good           any
	}{
		{"revoked grant", "grant", "effect", "deny", "allow"},
		{"foreign provider", "grant", "plugin_id", "com.powerx.plugins.other", pluginID},
		{"capability unpublished", "capability", "status", "draft", "published"},
		{"foreign capability provider", "capability", "plugin_id", "com.powerx.plugins.other", pluginID},
		{"registration disabled", "registration", "status", "disabled", "published"},
		{"foreign tenant", "registration", "tenant_uuid", uuid.NewString(), tenant},
		{"declaration disabled", "definition", "allow_api_key", false, true},
		{"Key disabled", "key", "status", "disabled", "active"},
		{"Profile disabled", "profile", "status", 0, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			models := map[string]any{"grant": &grant, "capability": &capability, "registration": &registration, "definition": &definition, "key": &key, "profile": &profile}
			model := models[tc.table]
			require.NoError(t, db.Model(model).Update(tc.column, tc.bad).Error)
			status := request(read)
			require.Contains(t, []int{http.StatusUnauthorized, http.StatusForbidden}, status)
			require.NoError(t, db.Model(model).Update(tc.column, tc.good).Error)
		})
	}
	require.Equal(t, http.StatusNoContent, request(read))
	require.EqualValues(t, 2, upstreamCalls.Load(), "all denied calls must stop at the gateway")
}
