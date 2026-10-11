package bootstrap

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	commonv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/common/v1"
	stsv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/auth/sts/v1"
	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	proxy "github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager/router"
	authsvc "github.com/ArtisanCloud/PowerX/internal/service/auth"
	capservice "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	caprouter "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/router"
	pluginservice "github.com/ArtisanCloud/PowerX/internal/service/plugin"
	identity "github.com/ArtisanCloud/PowerX/internal/service/plugin_identity"
	"github.com/ArtisanCloud/PowerX/internal/service/setting"
	authgrpc "github.com/ArtisanCloud/PowerX/internal/transport/grpc/auth"
	admin "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/plugin_identity"
	authhttp "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/user/auth"
	caphttp "github.com/ArtisanCloud/PowerX/internal/transport/http/openapi/capability_registry"
	transport "github.com/ArtisanCloud/PowerX/internal/transport/http/openapi/plugin_identity"
	"github.com/ArtisanCloud/PowerX/pkg/auth"
	"github.com/ArtisanCloud/PowerX/pkg/auth/middleware"
	"github.com/ArtisanCloud/PowerX/pkg/cache"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/database"
	core "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	cm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	im "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	gw "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	sm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	tm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestLivePluginRequestIdentity(t *testing.T) {
	path := os.Getenv("POWERX_IDENTITY_LIVE_CONFIG")
	if path == "" {
		t.Skip("opt-in private PostgreSQL/Redis config")
	}
	secretBytes := make([]byte, 32)
	_, err := rand.Read(secretBytes)
	require.NoError(t, err)
	secret := []byte(hex.EncodeToString(secretBytes))
	previousJWT := auth.GetJWTSecret()
	auth.SetJWTSecret(secret)
	t.Cleanup(func() { auth.SetJWTSecret(previousJWT) })
	t.Setenv("POWERX_AUTH_JWT_SECRET", string(secret))
	cfg, err := config.Load(path)
	require.NoError(t, err)
	db, err := database.Connect(cfg.Database)
	require.NoError(t, err)
	db = db.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)})
	schema := "identity_accept_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	previous := core.PowerXSchema
	core.PowerXSchema = schema
	require.NoError(t, db.Exec(`CREATE SCHEMA "`+schema+`"`).Error)
	t.Cleanup(func() {
		require.NoError(t, db.Exec(`DROP SCHEMA "`+schema+`" CASCADE`).Error)
		sql, _ := db.DB()
		_ = sql.Close()
		core.PowerXSchema = previous
	})
	require.NoError(t, db.AutoMigrate(&im.User{}, &im.Member{}, &im.RefreshToken{}, &tm.Tenant{}, &im.Role{}, &im.Permission{}, &im.RolePermission{}, &im.RoleBinding{}, &im.MemberAssignment{}, &cm.CapabilityRecord{}, &cm.CapabilityRegistration{}, &cm.AdapterEndpoint{}, &cm.RoutingPolicy{}, &cm.FallbackPlan{}, &sm.PluginInstanceConfig{}, &gw.IntegrationGatewayAPIKey{}, &gw.IntegrationGatewayAPIKeyPermission{}, &im.APIKeyProfile{}, &im.APIKey{}))
	redisStore, err := cache.NewCache(&cfg.Cache)
	require.NoError(t, err)
	svc := identity.NewService(db, redisStore, identity.Options{Namespace: schema, UserIssuer: "powerx-auth", UserAudience: "user"})
	pluginID := "com.powerx.acceptance.identity"
	tenant := tm.Tenant{Key: schema, Name: "identity fixture", Status: 1}
	require.NoError(t, db.Create(&tenant).Error)
	user := im.User{Email: schema + "@fixture.invalid", Status: 1}
	require.NoError(t, db.Create(&user).Error)
	member := im.Member{TenantUUID: tenant.UUID.String(), UserUUID: user.UUID.String(), UserID: user.ID, Username: "identity", Status: 1}
	require.NoError(t, db.Create(&member).Error)
	role := im.Role{TenantUUID: tenant.UUID.String(), Code: "role_user", Name: "identity"}
	require.NoError(t, db.Create(&role).Error)
	require.NoError(t, db.Create(&im.RoleBinding{TenantUUID: tenant.UUID.String(), RoleUUID: role.UUID.String(), RoleID: role.ID, SubjectType: im.SubMember, SubjectUUID: member.UUID.String(), SubjectID: member.ID}).Error)
	permission := im.Permission{Module: "operations", Resource: "order", Action: "read", Effect: "allow", Source: "plugin:" + pluginID, Status: im.PermissionStatusActive, Meta: datatypes.JSON(`{"type":"action"}`)}
	permission.Meta = datatypes.JSON(`{"type":"api","independent":true,"business_permission_code":"operations.order:read","protocol_bindings":[{"channel":"rest","method":"GET","path":"/admin/operations/orders"},{"channel":"rest","method":"POST","path":"/admin/runtime/ws-bus/grant"}]}`)
	require.NoError(t, db.Create(&permission).Error)
	require.NoError(t, db.Create(&im.RolePermission{RoleUUID: role.UUID, PermissionUUID: permission.PermissionUUID, RoleID: role.ID, PermissionID: permission.ID}).Error)
	session := im.RefreshToken{JTI: uuid.NewString(), TenantUUID: tenant.UUID.String(), UserUUID: user.UUID.String(), MemberUUID: member.UUID.String(), ExpiresAt: time.Now().Add(time.Hour).UnixMilli()}
	require.NoError(t, db.Create(&session).Error)
	claims := reqctx.CoreXClaims{TenantUUID: tenant.UUID.String(), UserUUID: user.UUID.String(), UserID: user.ID, MemberUUID: member.UUID.String(), MemberID: member.ID}
	claims.ID = session.JTI
	userToken, err := auth.GenerateAccessJWT(claims, "powerx-auth", []string{"user"}, time.Minute*15, secret)
	require.NoError(t, err)
	require.NoError(t, db.Create(&cm.CapabilityRecord{CapabilityID: identity.CapabilityID, PluginID: "corex.platform", PluginVersion: "v1", Title: "identity", Status: "published"}).Error)
	policy := cm.RoutingPolicy{Strategy: "weighted_round_robin"}
	require.NoError(t, db.Create(&policy).Error)
	registration := cm.CapabilityRegistration{CapabilityID: identity.CapabilityID, TenantUUID: tenant.UUID.String(), ContractRef: "v1", Status: "published", Version: 1, RoutingPolicyID: policy.UUID}
	require.NoError(t, db.Create(&registration).Error)
	require.NoError(t, db.Create(&cm.AdapterEndpoint{RegistrationID: registration.ID, CapabilityID: identity.CapabilityID, TenantUUID: tenant.UUID.String(), AdapterID: "identity-core-internal", TransportType: "core_internal", Endpoint: identity.Endpoint, Weight: 1, TimeoutMS: 5000, IsActive: true, Labels: datatypes.JSON(`{"method":"INVOKE"}`)}).Error)
	meta, _ := json.Marshal(map[string]any{"capability_id": identity.CapabilityID, "api_key_explicit": true, "api_key": map[string]any{"scope": identity.APIKeyScope, "action": "read", "resource_type": "capability", "resource_pattern": "iam_identity_context_read", "effect": "allow"}})
	require.NoError(t, db.Create(&im.Permission{Module: "iam", Resource: "identity_service", Action: "read", Effect: "allow", Status: im.PermissionStatusActive, AllowAPIKey: true, Meta: meta}).Error)
	creds := setting.NewPluginInstanceConfigService(&shared.Deps{DB: db})
	cid, clientSecret, err := creds.EnsureCredentials(context.Background(), tenant.UUID.String(), pluginID, &setting.ClientCredential{AllowedAudiences: []string{"powerx:api"}, AllowedScopes: []string{"access"}, AllowedCapabilities: []string{identity.CapabilityID}})
	require.NoError(t, err)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	ring := authgrpc.NewKeyRing()
	ring.AddHS("default", secret)
	grpcServer := grpc.NewServer()
	stsv1.RegisterSTSServiceServer(grpcServer, authgrpc.NewSTSServiceServerWithRing(&shared.Deps{DB: db}, ring))
	go grpcServer.Serve(listener)
	t.Cleanup(func() { grpcServer.Stop(); listener.Close() })
	conn, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer conn.Close()
	exchange, err := stsv1.NewSTSServiceClient(conn).Exchange(context.Background(), &stsv1.ExchangeRequest{Ctx: &commonv1.RequestContext{RequestId: uuid.NewString()}, ClientId: cid, ClientSecret: clientSecret, Audience: "powerx:api", Scope: "access", TtlSeconds: 300})
	require.NoError(t, err)
	require.EqualValues(t, 200, exchange.GetMeta().GetCode())
	bearer := "Bearer " + exchange.GetData().GetAccessToken()
	apiKey := "pxk_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	sum := sha256.Sum256([]byte(apiKey))
	keyHash := hex.EncodeToString(sum[:])
	profile := im.APIKeyProfile{TenantUUID: tenant.UUID.String(), Name: "identity", Status: 1}
	require.NoError(t, db.Create(&profile).Error)
	key := gw.IntegrationGatewayAPIKey{TenantUUID: tenant.UUID.String(), ProfileID: profile.ID, Name: "identity", KeyPrefix: apiKey[:12], KeyHash: keyHash, Status: "active", PluginOwnerPolicy: datatypes.JSON(fmt.Sprintf(`{"version":1,"mode":"key","plugin_ids":[%q]}`, pluginID))}
	require.NoError(t, db.Create(&key).Error)
	require.NoError(t, db.Create(&im.APIKey{TenantUUID: tenant.UUID.String(), ProfileID: profile.ID, KeyHash: keyHash}).Error)
	grant := gw.IntegrationGatewayAPIKeyPermission{APIKeyUUID: key.UUID, Scope: identity.APIKeyScope, Action: "read", ResourceType: "capability", ResourcePattern: "iam_identity_context_read", Effect: "allow", PluginID: pluginID}
	require.NoError(t, db.Create(&grant).Error)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		trace := c.GetHeader("X-Trace-ID")
		if trace == "" {
			trace = uuid.NewString()
		}
		c.Request = c.Request.WithContext(reqctx.WithTraceID(c.Request.Context(), trace))
		c.Header("X-Trace-ID", trace)
		c.Set("request_id", trace)
		c.Next()
	})
	authMW := middleware.APIKeyOrJwtMiddleware(db, secret, "powerx-auth", []string{"user"}, []string{"access"}, nil, middleware.WithExtraTokenChecks(middleware.TokenCheck{Issuer: "powerx-sts", Audiences: []string{"powerx:api"}}))
	group := engine.Group("/api/v1", authMW)
	catalog := capservice.NewRegistryService(capservice.RegistryServiceOptions{DB: db})
	invocation := capservice.NewInvocationService(capservice.InvocationServiceOptions{Catalog: catalog, Router: caprouter.NewService(caprouter.ServiceOptions{DB: db}), CoreInvoker: &identity.Invoker{Service: svc}})
	deps := &shared.Deps{DB: db, PluginRequestIdentity: svc, CapabilityCatalogSvc: catalog, CapabilityInvocationSvc: invocation}
	transport.Register(group, deps)
	admin.Register(group, deps)
	caphttp.RegisterTenantRoutes(group, deps)
	userAuth := authsvc.NewAuthService(db, authsvc.AuthOptions{JWTSecret: secret, Issuer: "powerx-auth", Audience: "user"})
	group.POST("/admin/user/auth/logout", authhttp.NewAuthUserHandler(deps).LogoutHandler(userAuth))
	coreServer := httptest.NewServer(engine)
	t.Cleanup(coreServer.Close)
	records := []map[string]any{{"name": "real_sts_exchange", "http_status": 200, "trace_id": exchange.GetMeta().GetRequestId(), "credentials_redacted": true}}
	call := func(name, route, authorization string, body any) (int, []byte) {
		raw, _ := json.Marshal(body)
		request, err := http.NewRequest("POST", coreServer.URL+route, bytes.NewReader(raw))
		require.NoError(t, err)
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Authorization", authorization)
		request.Header.Set("X-Trace-ID", uuid.NewString())
		response, err := http.DefaultClient.Do(request)
		require.NoError(t, err)
		defer response.Body.Close()
		data, _ := io.ReadAll(response.Body)
		var parsed any
		_ = json.Unmarshal(data, &parsed)
		records = append(records, map[string]any{"name": name, "http_status": response.StatusCode, "trace_id": response.Header.Get("X-Trace-ID"), "response": redactLiveIdentityProofs(parsed)})
		return response.StatusCode, data
	}
	var latestTicket string
	var latestWSHandle string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		latestTicket = r.Header.Get(identity.Header)
		status, data := call("gateway_sts_identity", "/api/v1/tenant/iam/identity-contexts:resolve", bearer, identity.Query{Schema: identity.QuerySchema, ContextTicket: latestTicket})
		// 模拟 Framework：服务端保留复核 handle，浏览器 grant/WS 响应不得包含它。
		if status == 200 {
			var envelope struct {
				Data identity.Result `json:"data"`
			}
			require.NoError(t, json.Unmarshal(data, &envelope))
			if envelope.Data.WSRevalidationHandle != "" {
				latestWSHandle = envelope.Data.WSRevalidationHandle
				var browserResponse map[string]any
				require.NoError(t, json.Unmarshal(data, &browserResponse))
				body := browserResponse["data"].(map[string]any)
				delete(body, "ws_revalidation_handle")
				delete(body, "ws_revalidation_expires_at")
				data, _ = json.Marshal(browserResponse)
			}
		}
		if websocket.IsWebSocketUpgrade(r) && status == 200 {
			connection, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
			if err == nil {
				defer connection.Close()
				_ = connection.WriteMessage(websocket.TextMessage, data)
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write(data)
	}))
	t.Cleanup(upstream.Close)
	dynamic := proxy.NewDynamicRouter("/_p", engine, authMW)
	dynamic.BindTenantPluginGuard(pluginservice.NewTenantPluginInstanceService(db).RequireEnabled)
	dynamic.BindRequestIdentityIssuer(svc)
	dynamic.BindAuthorizer(pluginIAMAuthorizer{db: db}, "powerx-auth", time.Minute)
	dynamic.InstallPolicy(pluginID, &proxy.Policy{HTTPBase: "/api/v1", Routes: map[string]proxy.Permission{"GET:/admin/operations/orders": {Module: "operations", Resource: "order", Action: "read"}, "POST:/admin/runtime/ws-bus/grant": {Module: "operations", Resource: "order", Action: "read"}}})
	target, _ := url.Parse(upstream.URL)
	dynamic.MountAPIProxy(pluginID, target, "/api/v1", "/healthz")
	request, err := http.NewRequest("GET", coreServer.URL+"/_p/"+pluginID+"/api/v1/admin/operations/orders", nil)
	require.NoError(t, err)
	request.Header.Set("Authorization", "Bearer "+userToken)
	request.Header.Set(identity.Header, "spoofed-ticket")
	request.Header.Set("X-Trace-ID", uuid.NewString())
	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)
	data, _ := io.ReadAll(response.Body)
	response.Body.Close()
	require.Equal(t, 200, response.StatusCode, "%s", data)
	require.NotEmpty(t, latestTicket)
	require.NotEqual(t, "spoofed-ticket", latestTicket)
	request, err = http.NewRequest("POST", coreServer.URL+"/_p/"+pluginID+"/api/v1/admin/runtime/ws-bus/grant", bytes.NewReader([]byte(`{}`)))
	require.NoError(t, err)
	request.Header.Set("Authorization", "Bearer "+userToken)
	request.Header.Set("X-Trace-ID", uuid.NewString())
	response, err = http.DefaultClient.Do(request)
	require.NoError(t, err)
	data, _ = io.ReadAll(response.Body)
	response.Body.Close()
	require.Equal(t, 200, response.StatusCode, "%s", data)
	require.Contains(t, string(data), `"transport":"ws_grant"`)
	wsHeaders := http.Header{}
	wsHeaders.Set("Authorization", "Bearer "+userToken)
	wsHeaders.Set("X-Trace-ID", uuid.NewString())
	ws, upgrade, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(coreServer.URL, "http")+"/_p/"+pluginID+"/api/v1/admin/operations/orders", wsHeaders)
	require.NoError(t, err)
	require.Equal(t, 101, upgrade.StatusCode)
	_, data, err = ws.ReadMessage()
	require.NoError(t, err)
	require.Contains(t, string(data), `"transport":"ws_upgrade"`)
	require.NotContains(t, string(data), "ws_revalidation_handle")
	ws.Close()
	require.NotEmpty(t, latestWSHandle)
	wsQuery := identity.WSQuery{Schema: identity.WSQuerySchema, Handle: latestWSHandle}
	wsRoute := "/api/v1/tenant/iam/identity-contexts:revalidate-ws"
	// Framework 实际使用 typed invocation；必须穿过真实 Selector / Router / Invoker。
	typedWS := func(name, authorization string, query identity.WSQuery, wantStatus int, wantReason string) {
		trace := uuid.NewString()
		status, data := call(name, "/api/v1/tenant/invocations", authorization, map[string]any{
			"capability_id": identity.CapabilityID, "preferred_protocol": "core_internal", "trace_id": trace,
			"payload": map[string]any{"body": identity.WSOperation{Operation: "revalidate_ws_identity", Request: query}},
		})
		require.Equal(t, wantStatus, status, "%s", data)
		if wantReason != "" {
			var envelope struct {
				Code       int    `json:"code"`
				ErrorCode  string `json:"error_code"`
				ReasonCode string `json:"reason_code"`
				Details    struct {
					TraceID    string `json:"trace_id"`
					ReasonCode string `json:"reason_code"`
				} `json:"details"`
			}
			require.NoError(t, json.Unmarshal(data, &envelope))
			require.Equal(t, wantStatus, envelope.Code)
			require.Equal(t, wantReason, envelope.ErrorCode)
			require.Equal(t, wantReason, envelope.ReasonCode)
			require.Equal(t, wantReason, envelope.Details.ReasonCode)
			require.Equal(t, trace, envelope.Details.TraceID)
			require.NotContains(t, string(data), latestWSHandle)
		}
	}
	typedWS("typed_ws_real_sts", bearer, wsQuery, 200, "")
	typedWS("typed_ws_real_api_key", "ApiKey "+apiKey, wsQuery, 200, "")
	missingHandle := identity.WSQuery{Schema: identity.WSQuerySchema, Handle: "pxwsi_v1_" + strings.Repeat("A", 43)}
	typedWS("typed_ws_expired_sts", bearer, missingHandle, 401, "IAM_CONTEXT_WS_HANDLE_EXPIRED")
	typedWS("typed_ws_expired_api_key", "ApiKey "+apiKey, missingHandle, 401, "IAM_CONTEXT_WS_HANDLE_EXPIRED")
	// 损坏隔离 Redis 中的 proof 模拟依赖不可用；检查 503 后立即恢复原记录。
	wsSum := sha256.Sum256([]byte(latestWSHandle))
	wsKey := "iam:ws-identity:v1:" + schema + ":" + hex.EncodeToString(wsSum[:])
	wsProof, err := redisStore.Get(context.Background(), wsKey)
	require.NoError(t, err)
	require.NoError(t, redisStore.Set(context.Background(), wsKey, []byte("{"), 15*time.Minute))
	typedWS("typed_ws_unavailable_sts", bearer, wsQuery, 503, "IAM_CONTEXT_UNAVAILABLE")
	typedWS("typed_ws_unavailable_api_key", "ApiKey "+apiKey, wsQuery, 503, "IAM_CONTEXT_UNAVAILABLE")
	require.NoError(t, redisStore.Set(context.Background(), wsKey, wsProof, 15*time.Minute))
	status, data := call("ws_revalidation_real_sts", wsRoute, bearer, wsQuery)
	require.Equal(t, 200, status)
	require.Contains(t, string(data), identity.WSResultSchema)
	status, _ = call("ws_revalidation_real_api_key", wsRoute, "ApiKey "+apiKey, wsQuery)
	require.Equal(t, 200, status)
	status, _ = call("ws_revalidation_forged_handle", wsRoute, bearer, identity.WSQuery{Schema: identity.WSQuerySchema, Handle: "forged"})
	require.Equal(t, 401, status)
	status, _ = call("ws_revalidation_identity_override", wsRoute, bearer, map[string]any{"schema": identity.WSQuerySchema, "ws_revalidation_handle": latestWSHandle, "user_uuid": uuid.NewString()})
	require.Equal(t, 400, status)
	require.NoError(t, db.Model(&member).Update("status", 2).Error)
	typedWS("typed_ws_member_disabled_sts", bearer, wsQuery, 403, "IAM_CONTEXT_USER_FORBIDDEN")
	typedWS("typed_ws_member_disabled_api_key", "ApiKey "+apiKey, wsQuery, 403, "IAM_CONTEXT_USER_FORBIDDEN")
	status, _ = call("ws_revalidation_member_disabled", wsRoute, bearer, wsQuery)
	require.Equal(t, 403, status)
	require.NoError(t, db.Model(&member).Update("status", 1).Error)
	rootUser := im.User{Email: schema + "root@fixture.invalid", Status: 1, IsRoot: true}
	require.NoError(t, db.Create(&rootUser).Error)
	rootSession := im.RefreshToken{JTI: uuid.NewString(), TenantUUID: tenant.UUID.String(), UserUUID: rootUser.UUID.String(), ExpiresAt: time.Now().Add(time.Hour).UnixMilli()}
	require.NoError(t, db.Create(&rootSession).Error)
	rootClaims := reqctx.CoreXClaims{TenantUUID: tenant.UUID.String(), UserUUID: rootUser.UUID.String(), UserID: rootUser.ID, IsRoot: true}
	rootClaims.ID = rootSession.JTI
	rootToken, err := auth.GenerateAccessJWT(rootClaims, "powerx-auth", []string{"user"}, 15*time.Minute, secret)
	require.NoError(t, err)
	request, err = http.NewRequest("GET", coreServer.URL+"/_p/"+pluginID+"/api/v1/admin/operations/orders", nil)
	require.NoError(t, err)
	request.Header.Set("Authorization", "Bearer "+rootToken)
	request.Header.Set("X-Trace-ID", uuid.NewString())
	response, err = http.DefaultClient.Do(request)
	require.NoError(t, err)
	data, _ = io.ReadAll(response.Body)
	response.Body.Close()
	require.Equal(t, 200, response.StatusCode, "%s", data)
	require.Contains(t, string(data), `"member_uuid":null`)
	require.Contains(t, string(data), `"is_root":true`)
	status, _ = call("root_admin_own_context", "/api/v1/admin/iam/identity-contexts:resolve", "Bearer "+rootToken, identity.Query{Schema: identity.QuerySchema, ContextTicket: latestTicket})
	require.Equal(t, 200, status)
	status, _ = call("service_credential_rejected_at_admin_plane", "/api/v1/admin/iam/identity-contexts:resolve", bearer, identity.Query{Schema: identity.QuerySchema, ContextTicket: latestTicket})
	require.Equal(t, 401, status)
	// Restore an ordinary user's source ticket for revocation tests.
	request, _ = http.NewRequest("GET", coreServer.URL+"/_p/"+pluginID+"/api/v1/admin/operations/orders", nil)
	request.Header.Set("Authorization", "Bearer "+userToken)
	request.Header.Set("X-Trace-ID", uuid.NewString())
	response, err = http.DefaultClient.Do(request)
	require.NoError(t, err)
	response.Body.Close()
	require.Equal(t, 200, response.StatusCode)
	status, _ = call("real_api_key_identity", "/api/v1/tenant/iam/identity-contexts:resolve", "ApiKey "+apiKey, identity.Query{Schema: identity.QuerySchema, ContextTicket: latestTicket})
	require.Equal(t, 200, status)
	status, _ = call("user_credential_rejected_at_service_plane", "/api/v1/tenant/iam/identity-contexts:resolve", "Bearer "+userToken, identity.Query{Schema: identity.QuerySchema, ContextTicket: latestTicket})
	require.Equal(t, 401, status)
	status, _ = call("unknown_identity_override", "/api/v1/tenant/iam/identity-contexts:resolve", bearer, map[string]any{"schema": identity.QuerySchema, "context_ticket": latestTicket, "user_uuid": uuid.NewString()})
	require.Equal(t, 400, status)
	status, _ = call("missing_context_proof", "/api/v1/tenant/iam/identity-contexts:resolve", bearer, identity.Query{Schema: identity.QuerySchema})
	require.Equal(t, 401, status)
	require.NoError(t, db.Model(&key).Update("plugin_owner_policy", datatypes.JSON(`{"version":1,"mode":"key","plugin_ids":[]}`)).Error)
	status, _ = call("api_key_owner_rejected", "/api/v1/tenant/iam/identity-contexts:resolve", "ApiKey "+apiKey, identity.Query{Schema: identity.QuerySchema, ContextTicket: latestTicket})
	require.Equal(t, 403, status)
	status, _ = call("ws_revalidation_api_key_owner_rejected", wsRoute, "ApiKey "+apiKey, wsQuery)
	require.Equal(t, 403, status)
	require.NoError(t, db.Model(&key).Update("plugin_owner_policy", datatypes.JSON(fmt.Sprintf(`{"version":1,"mode":"key","plugin_ids":[%q]}`, pluginID))).Error)
	expired := reqctx.CoreXClaims{TenantUUID: tenant.UUID.String(), PluginID: pluginID, Scope: "access", RegisteredClaims: jwt.RegisteredClaims{Issuer: "powerx-sts", Subject: "client:" + cid, Audience: []string{"powerx:api"}, ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Minute))}}
	expiredJWT, err := jwt.NewWithClaims(jwt.SigningMethodHS256, expired).SignedString(secret)
	require.NoError(t, err)
	status, data = call("expired_service_credential", "/api/v1/tenant/iam/identity-contexts:resolve", "Bearer "+expiredJWT, identity.Query{Schema: identity.QuerySchema, ContextTicket: latestTicket})
	require.Equal(t, 401, status)
	require.Contains(t, string(data), "IAM_CONTEXT_SERVICE_CREDENTIAL_EXPIRED")
	require.NoError(t, db.Model(&permission).Update("status", im.PermissionStatusDeprecated).Error)
	status, _ = call("permission_revoked", "/api/v1/tenant/iam/identity-contexts:resolve", bearer, identity.Query{Schema: identity.QuerySchema, ContextTicket: latestTicket})
	require.Equal(t, 403, status)
	status, _ = call("ws_revalidation_permission_revoked", wsRoute, bearer, wsQuery)
	require.Equal(t, 403, status)
	status, _ = call("ws_revalidation_api_key_permission_revoked", wsRoute, "ApiKey "+apiKey, wsQuery)
	require.Equal(t, 403, status)
	require.NoError(t, db.Model(&permission).Update("status", im.PermissionStatusActive).Error)
	refresh, err := auth.GenerateRefreshJWT(claims, "powerx-auth", []string{"user"}, session.JTI, time.Hour, secret)
	require.NoError(t, err)
	status, _ = call("real_user_logout", "/api/v1/admin/user/auth/logout", "Bearer "+userToken, map[string]any{"refresh_token": refresh})
	require.Equal(t, 200, status)
	status, _ = call("human_session_revoked", "/api/v1/tenant/iam/identity-contexts:resolve", bearer, identity.Query{Schema: identity.QuerySchema, ContextTicket: latestTicket})
	typedWS("typed_ws_after_logout_sts", bearer, wsQuery, 401, "IAM_CONTEXT_USER_SESSION_INVALID")
	typedWS("typed_ws_after_logout_api_key", "ApiKey "+apiKey, wsQuery, 401, "IAM_CONTEXT_USER_SESSION_INVALID")
	require.Equal(t, 401, status)
	status, data = call("ws_revalidation_after_real_logout", wsRoute, bearer, wsQuery)
	require.Equal(t, 401, status)
	require.Contains(t, string(data), "IAM_CONTEXT_USER_SESSION_INVALID")
	status, _ = call("ws_revalidation_api_key_after_real_logout", wsRoute, "ApiKey "+apiKey, wsQuery)
	require.Equal(t, 401, status)
	require.NoError(t, db.Delete(&grant).Error)
	status, _ = call("service_grant_revoked", "/api/v1/tenant/iam/identity-contexts:resolve", "ApiKey "+apiKey, identity.Query{Schema: identity.QuerySchema, ContextTicket: latestTicket})
	require.Equal(t, 403, status)
	status, _ = call("ws_revalidation_service_grant_revoked", wsRoute, "ApiKey "+apiKey, wsQuery)
	require.Equal(t, 403, status)
	t.Logf("Agent-owned fixture PID=%d HTTP=%s gRPC=%s; all listeners close on cleanup", os.Getpid(), coreServer.Listener.Addr(), listener.Addr())
	if path := os.Getenv("POWERX_IDENTITY_EVIDENCE"); path != "" {
		encoded, _ := json.MarshalIndent(map[string]any{"schema": "powerx.iam.request-identity-acceptance/v1", "fixture_kind": "isolated_postgresql_and_real_redis_core_proxy_http_sts_exchange", "credentials_redacted": true, "records": records, "process_pid": os.Getpid()}, "", "  ")
		require.NoError(t, os.WriteFile(path, encoded, 0600))
	}
}

func redactLiveIdentityProofs(value any) any {
	switch current := value.(type) {
	case map[string]any:
		out := map[string]any{}
		for key, item := range current {
			if key == "context_ticket" || key == "ws_revalidation_handle" {
				out[key] = "[REDACTED]"
			} else {
				out[key] = redactLiveIdentityProofs(item)
			}
		}
		return out
	case []any:
		out := make([]any, len(current))
		for index, item := range current {
			out[index] = redactLiveIdentityProofs(item)
		}
		return out
	default:
		return current
	}
}
