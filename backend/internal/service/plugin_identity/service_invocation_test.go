package plugin_identity

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	cap "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	caprouter "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/router"
	"github.com/ArtisanCloud/PowerX/pkg/cache"
	cm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	im "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	gw "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	sm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/pluginpermission"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/stsproof"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

func serviceCallFixture(t *testing.T) (fixture, context.Context, cap.ServiceInvocationBinding) {
	f := newFixture(t, false)
	capID := f.plugin + ".messages.reply"
	binding := cap.ServiceInvocationBinding{TenantUUID: f.tenant, TargetPluginID: f.plugin, CapabilityID: capID, ProviderVersion: "1.2.3", CapabilitiesHash: "catalog-hash", ProtocolHash: "protocol-hash", Method: "POST", Path: "/api/v1/service/messages/reply", BodySHA256: cap.ServiceBodySHA256([]byte(`{"text":"hello"}`)), TraceID: uuid.NewString(), RequestID: uuid.NewString()}
	protocols, _ := json.Marshal([]cm.ProtocolBinding{{Channel: "rest", Endpoint: binding.Path, Method: binding.Method, AuthType: cap.PluginServiceAuthType, ActorContext: "service_actor", ResourceScope: "tenant", SchemaRef: "contracts/messages.yaml#/ReplyRequest"}})
	require.NoError(t, f.db.Create(&cm.CapabilityRecord{CapabilityID: capID, PluginID: f.plugin, PluginVersion: binding.ProviderVersion, CapabilitiesHash: binding.CapabilitiesHash, ProtocolHash: binding.ProtocolHash, Title: "Reply", Status: "published", Protocols: protocols}).Error)
	require.NoError(t, f.db.Create(&cm.CapabilityRegistration{CapabilityID: capID, TenantUUID: f.tenant, ContractRef: "1.0.0", Version: 1, Status: "published", RoutingPolicyID: uuid.New()}).Error)
	require.NoError(t, f.db.Create(&sm.PluginInstanceConfig{TenantUUID: f.tenant, PluginID: "com.example.ai", Key: "auth.credentials", Enabled: true, ValueJSON: datatypes.JSON(`{"client_id":"ai","allowed_capabilities":["` + capID + `"]}`)}).Error)
	c := reqctx.CoreXClaims{TenantUUID: f.tenant, PluginID: "com.example.ai", RegisteredClaims: jwt.RegisteredClaims{Issuer: "powerx-sts", Subject: "client:ai", Audience: []string{"powerx:api"}, ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute))}}
	c.ID = uuid.NewString()
	require.NoError(t, stsproof.Record(context.Background(), f.store, &c))
	ctx := reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), f.tenant), &c)
	return f, ctx, binding
}

func invocationQuery(ticket string, b cap.ServiceInvocationBinding) ServiceInvocationQuery {
	return ServiceInvocationQuery{Schema: ServiceInvocationQuerySchema, InvocationTicket: ticket, CapabilityID: b.CapabilityID, CapabilityVersion: "1.0.0", Method: b.Method, Path: b.Path, BodySHA256: b.BodySHA256}
}

func TestServiceInvocationProofBindingAndRevocation(t *testing.T) {
	f, source, b := serviceCallFixture(t)
	ticket, err := f.s.IssueServiceInvocation(source, b)
	require.NoError(t, err)
	q := invocationQuery(ticket, b)
	result, err := f.s.ResolveServiceInvocation(f.svc, q)
	require.NoError(t, err)
	require.Equal(t, "service_actor", result.ActorType)
	require.Equal(t, "sts:com.example.ai:client:ai", result.CallerSubject)
	require.Equal(t, "com.example.ai", result.CallerPluginID)
	require.Equal(t, "1.0.0", result.CapabilityVersion)
	for _, alter := range []func(*ServiceInvocationQuery){
		func(q *ServiceInvocationQuery) { q.CapabilityID += ".other" },
		func(q *ServiceInvocationQuery) { q.CapabilityVersion = "2.0.0" },
		func(q *ServiceInvocationQuery) { q.Method = "DELETE" },
		func(q *ServiceInvocationQuery) { q.Path = "/api/v1/service/admin" },
		func(q *ServiceInvocationQuery) { q.BodySHA256 = cap.ServiceBodySHA256([]byte("forged")) },
	} {
		altered := q
		alter(&altered)
		_, err = f.s.ResolveServiceInvocation(f.svc, altered)
		require.Equal(t, "IAM_SERVICE_INVOCATION_REQUEST_MISMATCH", dto.CodeOf(err))
	}
	admin := reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), f.tenant), &f.user)
	_, err = f.s.ResolveServiceInvocation(admin, q)
	require.Equal(t, 401, dto.StatusCode(err))
	_, err = f.s.IssueServiceInvocation(admin, b)
	require.Equal(t, 401, dto.StatusCode(err))
	for _, tenant := range []string{uuid.NewString()} {
		_, err = f.s.ResolveServiceInvocation(reqctx.WithTenantUUID(f.svc, tenant), q)
		require.Error(t, err)
	}
	// 未绑定到目标插件的服务凭据即使拥有解析能力，也不能读取票据。
	other := reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), f.tenant), &reqctx.CoreXClaims{TenantUUID: f.tenant, PluginID: "com.example.other", RegisteredClaims: jwt.RegisteredClaims{Issuer: "powerx-sts", Subject: "client:other", Audience: []string{"powerx:api"}, ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute))}})
	otherClaims := reqctx.GetClaims(other)
	otherClaims.ID = uuid.NewString()
	require.NoError(t, stsproof.Record(other, f.store, otherClaims))
	require.NoError(t, f.db.Create(&sm.PluginInstanceConfig{TenantUUID: f.tenant, PluginID: "com.example.other", Key: "auth.credentials", Enabled: true, ValueJSON: datatypes.JSON(`{"client_id":"other","allowed_capabilities":["` + CapabilityID + `"]}`)}).Error)
	_, err = f.s.ResolveServiceInvocation(other, q)
	require.Equal(t, "IAM_SERVICE_INVOCATION_TARGET_FORBIDDEN", dto.CodeOf(err))
	for _, change := range []struct {
		model          any
		where, column  string
		value, restore any
	}{
		{&sm.PluginInstanceConfig{}, "plugin_id='com.example.ai'", "value_json", datatypes.JSON(`{"client_id":"ai","allowed_capabilities":[]}`), datatypes.JSON(`{"client_id":"ai","allowed_capabilities":["` + b.CapabilityID + `"]}`)},
		{&cm.CapabilityRecord{}, "capability_id='" + b.CapabilityID + "'", "status", "draft", "published"},
		{&cm.CapabilityRegistration{}, "capability_id='" + b.CapabilityID + "'", "status", "disabled", "published"},
		{&cm.CapabilityRecord{}, "capability_id='" + b.CapabilityID + "'", "protocol_hash", "changed", b.ProtocolHash},
	} {
		require.NoError(t, f.db.Model(change.model).Where(change.where).Update(change.column, change.value).Error)
		_, err = f.s.ResolveServiceInvocation(f.svc, q)
		require.Error(t, err)
		require.NoError(t, f.db.Model(change.model).Where(change.where).Update(change.column, change.restore).Error)
	}
	f.s.clock = func() time.Time { return time.Now().Add(2 * time.Minute) }
	_, err = f.s.ResolveServiceInvocation(f.svc, q)
	require.Equal(t, "IAM_SERVICE_INVOCATION_PROOF_EXPIRED", dto.CodeOf(err))
}

func TestServiceInvocationDispatchUsesFixedRouteAndNoAdminBearer(t *testing.T) {
	f, source, b := serviceCallFixture(t)
	f.db.Config.DisableForeignKeyConstraintWhenMigrating = true
	require.NoError(t, f.db.AutoMigrate(&cm.RoutingPolicy{}, &cm.AdapterEndpoint{}, &cm.FallbackPlan{}))
	policy := cm.RoutingPolicy{Strategy: "weighted_round_robin"}
	require.NoError(t, f.db.Create(&policy).Error)
	var reg cm.CapabilityRegistration
	require.NoError(t, f.db.Where("capability_id=?", b.CapabilityID).First(&reg).Error)
	require.NoError(t, f.db.Model(&reg).Update("routing_policy_id", policy.UUID).Error)
	require.NoError(t, f.db.Create(&cm.AdapterEndpoint{RegistrationID: reg.ID, CapabilityID: b.CapabilityID, TenantUUID: f.tenant, AdapterID: "service", TransportType: "rest", Endpoint: plugin_mgr.CatalogPluginEndpoint(f.plugin, "rest", b.Path), IsActive: true, Weight: 100, Labels: datatypes.JSON(`{"method":"POST"}`)}).Error)
	var ticket string
	calls := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "" || r.Header.Get("X-PowerX-Is-Root") != "" || r.Header.Get("X-Tenant-UUID") != "" {
			t.Error("untrusted identity forwarded")
			w.WriteHeader(403)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		ticket = r.Header.Get(cap.PluginServiceIdentityHeader)
		q := invocationQuery(ticket, b)
		q.BodySHA256 = cap.ServiceBodySHA256(raw)
		q.Method = r.Method
		q.Path = r.URL.Path
		result, err := f.s.ResolveServiceInvocation(f.svc, q)
		if err != nil {
			t.Errorf("verification failed: %v", err)
			w.WriteHeader(403)
			return
		}
		if result.RequestID != r.Header.Get("X-Request-ID") || result.TraceID != r.Header.Get("X-Trace-ID") {
			t.Error("correlation mismatch")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"accepted": true, "caller": result.CallerSubject})
	}))
	t.Cleanup(target.Close)
	catalog := cap.NewRegistryService(cap.RegistryServiceOptions{DB: f.db})
	svc := cap.NewInvocationService(cap.InvocationServiceOptions{Catalog: catalog, Router: caprouter.NewService(caprouter.ServiceOptions{DB: f.db}), ServiceIdentityIssuer: f.s, HTTPClient: target.Client()})
	svc.BindPluginServiceTargetResolver(func(ctx context.Context, tenant, pluginID, version, requestPath string) (string, error) {
		require.Equal(t, f.tenant, tenant)
		require.Equal(t, f.plugin, pluginID)
		require.Equal(t, b.ProviderVersion, version)
		require.Equal(t, b.Path, requestPath)
		return target.URL + requestPath, nil
	})
	in := cap.InvocationInput{TenantUUID: f.tenant, CapabilityID: b.CapabilityID, PreferredProtocol: "rest", Payload: map[string]any{"body": map[string]any{"text": "hello"}}}
	result, err := svc.Invoke(source, in)
	require.NoError(t, err)
	require.Equal(t, true, result.Result["accepted"])
	require.NotEmpty(t, result.TraceID)
	require.Equal(t, 1, calls)
	_, err = f.s.ResolveServiceInvocation(f.svc, invocationQuery(ticket, b))
	require.Equal(t, "IAM_SERVICE_INVOCATION_PROOF_EXPIRED", dto.CodeOf(err), "completed call ticket is removed")
	for _, field := range []string{"endpoint", "method", "headers", "tenant_uuid", "caller_plugin_id"} {
		in.Payload[field] = "forged"
		_, err = svc.Invoke(source, in)
		require.Equal(t, "CAPABILITY_SERVICE_PAYLOAD_INVALID", dto.CodeOf(err))
		delete(in.Payload, field)
	}
	admin := reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), f.tenant), &f.user)
	_, err = svc.Invoke(admin, in)
	require.Equal(t, "CAPABILITY_SERVICE_IDENTITY_REQUIRED", dto.CodeOf(err))
	require.Equal(t, 1, calls)
}

func TestServiceInvocationAPIKeyRetainsCallerAndExactGrant(t *testing.T) {
	f, _, b := serviceCallFixture(t)
	spec := pluginpermission.APIKeySpec{CapabilityID: b.CapabilityID, PluginID: f.plugin, Scope: "_scope.messages.reply", Action: "create", ResourceType: "capability", ResourcePattern: "message_reply", Effect: "allow"}
	meta, err := pluginpermission.ExplicitAPIKeyMetadata(&spec, f.plugin, b.CapabilityID, true)
	require.NoError(t, err)
	raw, _ := json.Marshal(meta)
	require.NoError(t, f.db.Create(&im.Permission{Module: "messages", Resource: "service_reply", Action: "create", Effect: "allow", Status: im.PermissionStatusActive, AllowAPIKey: true, Source: "plugin:" + f.plugin, Meta: raw}).Error)
	key := gw.IntegrationGatewayAPIKey{TenantUUID: f.tenant, ProfileID: f.key.ProfileID, Name: "AI caller", Status: "active", KeyHash: "private-fixture-source-hash", PluginOwnerPolicy: datatypes.JSON(`{"version":1,"mode":"key","plugin_ids":["com.example.ai"]}`)}
	require.NoError(t, f.db.Create(&key).Error)
	grant := gw.IntegrationGatewayAPIKeyPermission{APIKeyUUID: key.UUID, PluginID: f.plugin, Scope: spec.Scope, Action: spec.Action, ResourceType: spec.ResourceType, ResourcePattern: spec.ResourcePattern, Effect: "allow"}
	require.NoError(t, f.db.Create(&grant).Error)
	grantStatusID := cap.GrantStatusCapabilityID
	require.NoError(t, f.db.Create(&cm.CapabilityRecord{CapabilityID: grantStatusID, PluginID: "corex.platform", PluginVersion: "v1", Title: "Grant status", Status: "published"}).Error)
	require.NoError(t, f.db.Create(&cm.CapabilityRegistration{CapabilityID: grantStatusID, TenantUUID: f.tenant, ContractRef: "v1", Version: 1, Status: "published", RoutingPolicyID: uuid.New()}).Error)
	require.NoError(t, f.db.Create(&im.Permission{Module: "capability_registry", Resource: "grant-status", Action: "read", Effect: "allow", Status: im.PermissionStatusActive, AllowAPIKey: true, Meta: datatypes.JSON(`{"capability_id":"com.corex.capabilities.grant_status.read","api_key_explicit":true,"api_key":{"scope":"_scope.capabilities.grant_status.read","action":"read","resource_type":"api","resource_pattern":"grant-status","effect":"allow"}}`)}).Error)
	require.NoError(t, f.db.Create(&gw.IntegrationGatewayAPIKeyPermission{APIKeyUUID: key.UUID, Scope: "_scope.capabilities.grant_status.read", Action: "read", ResourceType: "api", ResourcePattern: "grant-status", Effect: "allow"}).Error)
	ctx := reqctx.WithAuthenticatedAPIKeyHash(reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), f.tenant), &reqctx.CoreXClaims{TenantUUID: f.tenant, Platforms: []string{"api_key"}}), key.KeyHash)
	statuses, err := cap.NewGrantStatusService(f.db).CheckCurrentCredential(ctx, []string{b.CapabilityID})
	require.NoError(t, err)
	require.Equal(t, cap.GrantStatusGranted, statuses[0].Status)
	ticket, err := f.s.IssueServiceInvocation(ctx, b)
	require.NoError(t, err)
	result, err := f.s.ResolveServiceInvocation(f.svc, invocationQuery(ticket, b))
	require.NoError(t, err)
	require.Equal(t, "api_key:"+key.UUID.String(), result.CallerSubject)
	require.Equal(t, "com.example.ai", result.CallerPluginID)
	require.Equal(t, f.plugin, result.TargetPluginID)
	for _, owner := range []string{"", "com.example.other"} {
		require.NoError(t, f.db.Model(&grant).Update("plugin_id", owner).Error)
		_, err = f.s.ResolveServiceInvocation(f.svc, invocationQuery(ticket, b))
		require.Error(t, err)
		_, err = f.s.IssueServiceInvocation(ctx, b)
		require.Error(t, err)
		statuses, err = cap.NewGrantStatusService(f.db).CheckCurrentCredential(ctx, []string{b.CapabilityID})
		require.NoError(t, err)
		require.Equal(t, cap.GrantStatusNotGranted, statuses[0].Status)
	}
	require.NoError(t, f.db.Model(&grant).Update("plugin_id", f.plugin).Error)
	require.NoError(t, f.db.Model(&key).Update("status", "revoked").Error)
	_, err = f.s.ResolveServiceInvocation(f.svc, invocationQuery(ticket, b))
	require.Error(t, err)
}

type unavailableServiceInvocationCache struct{ cache.ICache }

func (unavailableServiceInvocationCache) Get(context.Context, string) ([]byte, error) {
	return nil, errors.New("cache unavailable")
}
func (unavailableServiceInvocationCache) Set(context.Context, string, interface{}, time.Duration) error {
	return errors.New("cache unavailable")
}

func TestServiceInvocationCacheFailureIsUnavailable(t *testing.T) {
	f, source, b := serviceCallFixture(t)
	ticket, err := f.s.IssueServiceInvocation(source, b)
	require.NoError(t, err)
	f.s.store = unavailableServiceInvocationCache{f.store}
	_, err = f.s.ResolveServiceInvocation(f.svc, invocationQuery(ticket, b))
	require.Equal(t, 503, dto.StatusCode(err))
	_, err = f.s.IssueServiceInvocation(source, b)
	require.Equal(t, 503, dto.StatusCode(err))
}
