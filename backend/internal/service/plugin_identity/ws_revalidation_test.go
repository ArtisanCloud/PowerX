package plugin_identity

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	cap "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	im "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	gw "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	sm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

func wsQuery(result Result) WSQuery {
	return WSQuery{Schema: WSQuerySchema, Handle: result.WSRevalidationHandle}
}

func TestWSHandleSurvivesSourceTicketWithoutExtendingAccess(t *testing.T) {
	f := newFixture(t, false)
	ticket := f.issue(t, "ws_grant")
	initial, err := f.s.Resolve(f.svc, query(ticket))
	require.NoError(t, err)
	require.Regexp(t, wsHandlePattern, initial.WSRevalidationHandle)
	require.Equal(t, initial.UserSessionExpiresAt, initial.WSRevalidationExpiresAt)
	second, err := f.s.Resolve(f.svc, query(ticket))
	require.NoError(t, err)
	require.Equal(t, initial.WSRevalidationHandle, second.WSRevalidationHandle)
	now := f.s.clock()
	f.s.clock = func() time.Time { return now.Add(61 * time.Second) }
	_, err = f.s.Resolve(f.svc, query(ticket))
	require.Equal(t, "IAM_CONTEXT_PROOF_EXPIRED", dto.CodeOf(err))
	current, err := f.s.RevalidateWS(f.svc, wsQuery(initial))
	require.NoError(t, err)
	require.Equal(t, WSResultSchema, current.Schema)
	require.Equal(t, initial.WSRevalidationExpiresAt, current.WSRevalidationExpiresAt)
	require.Equal(t, []string{"operations.order:read"}, current.Permissions)
	f.s.clock = func() time.Time { return f.user.ExpiresAt.Time }
	_, err = f.s.RevalidateWS(f.svc, wsQuery(initial))
	require.Equal(t, "IAM_CONTEXT_WS_HANDLE_EXPIRED", dto.CodeOf(err))
}

func TestWSRevalidationRejectsCurrentRevocation(t *testing.T) {
	cases := []struct {
		name   string
		change func(t *testing.T, f fixture)
		status int
		code   string
	}{
		{"logout", func(t *testing.T, f fixture) {
			require.NoError(t, f.db.Model(&f.session).Update("revoked_at", time.Now().UnixMilli()).Error)
		}, 401, "IAM_CONTEXT_USER_SESSION_INVALID"},
		{"member disabled", func(t *testing.T, f fixture) {
			require.NoError(t, f.db.Model(&im.Member{}).Where("uuid=?", f.user.MemberUUID).Update("status", 2).Error)
		}, 403, "IAM_CONTEXT_USER_FORBIDDEN"},
		{"user disabled", func(t *testing.T, f fixture) {
			require.NoError(t, f.db.Model(&im.User{}).Where("uuid=?", f.user.UserUUID).Update("status", 2).Error)
		}, 403, "IAM_CONTEXT_USER_FORBIDDEN"},
		{"permission withdrawn", func(t *testing.T, f fixture) {
			require.NoError(t, f.db.Model(&f.permission).Update("status", im.PermissionStatusDeprecated).Error)
		}, 403, "IAM_CONTEXT_REQUEST_FORBIDDEN"},
		{"role binding withdrawn", func(t *testing.T, f fixture) {
			require.NoError(t, f.db.Where("subject_uuid=?", f.user.MemberUUID).Delete(&im.RoleBinding{}).Error)
		}, 403, "IAM_CONTEXT_REQUEST_FORBIDDEN"},
		{"service grant withdrawn", func(t *testing.T, f fixture) {
			require.NoError(t, f.db.Where("api_key_uuid=?", f.key.UUID).Delete(&gw.IntegrationGatewayAPIKeyPermission{}).Error)
		}, 403, "IAM_CONTEXT_SERVICE_FORBIDDEN"},
		{"key revoked", func(t *testing.T, f fixture) {
			require.NoError(t, f.db.Model(&f.key).Update("status", "revoked").Error)
		}, 401, "IAM_CONTEXT_SERVICE_CREDENTIAL_INVALID"},
		{"plugin owner withdrawn", func(t *testing.T, f fixture) {
			require.NoError(t, f.db.Model(&f.key).Update("plugin_owner_policy", datatypes.JSON(`{"version":1,"mode":"key","plugin_ids":[]}`)).Error)
		}, 403, "IAM_CONTEXT_PLUGIN_FORBIDDEN"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, false)
			initial, err := f.s.Resolve(f.svc, query(f.issue(t, "ws_upgrade")))
			require.NoError(t, err)
			tc.change(t, f)
			result, err := f.s.RevalidateWS(f.svc, wsQuery(initial))
			require.Equal(t, tc.status, dto.StatusCode(err))
			require.Equal(t, tc.code, dto.CodeOf(err))
			require.Empty(t, result.Permissions)
		})
	}
}

func TestWSRevalidationOwnerAndAdminSessionIsolation(t *testing.T) {
	f := newFixture(t, true)
	ticket := f.issue(t, "ws_grant")
	initial, err := f.s.Resolve(f.svc, query(ticket))
	require.NoError(t, err)
	admin := reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), f.tenant), &f.user)
	inspected, err := f.s.ResolveAdmin(admin, query(ticket))
	require.NoError(t, err)
	require.Empty(t, inspected.WSRevalidationHandle, "Admin inspection must not mint a server handle")
	current, err := f.s.RevalidateWSAdmin(admin, wsQuery(initial))
	require.NoError(t, err)
	require.Nil(t, current.MemberUUID)
	_, err = f.s.RevalidateWSAdmin(f.svc, wsQuery(initial))
	require.Equal(t, "IAM_CONTEXT_USER_SESSION_INVALID", dto.CodeOf(err))
	wrongSession := f.user
	wrongSession.ID = uuid.NewString()
	_, err = f.s.RevalidateWSAdmin(reqctx.WithClaims(admin, &wrongSession), wsQuery(initial))
	require.Equal(t, "IAM_CONTEXT_USER_FORBIDDEN", dto.CodeOf(err))
	other := *reqctx.GetClaims(f.svc)
	other.TenantUUID = uuid.NewString()
	_, err = f.s.RevalidateWS(reqctx.WithClaims(reqctx.WithTenantUUID(f.svc, other.TenantUUID), &other), wsQuery(initial))
	require.Error(t, err)
	sts := reqctx.CoreXClaims{TenantUUID: f.tenant, PluginID: f.plugin, Scope: "access", RegisteredClaims: jwt.RegisteredClaims{Issuer: "powerx-sts", Subject: "client:client-fixture", Audience: []string{"powerx:api"}, ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}}
	ctx := reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), f.tenant), &sts)
	_, err = f.s.RevalidateWS(ctx, wsQuery(initial))
	require.NoError(t, err)
	sts.PluginID = "com.powerx.plugins.other"
	sts.Subject = "client:other-client"
	credentials, err := json.Marshal(map[string]any{"client_id": "other-client", "allowed_capabilities": []string{CapabilityID}})
	require.NoError(t, err)
	require.NoError(t, f.db.Create(&sm.PluginInstanceConfig{TenantUUID: f.tenant, PluginID: sts.PluginID, Key: "auth.credentials", Enabled: true, ValueJSON: credentials}).Error)
	_, err = f.s.RevalidateWS(reqctx.WithClaims(ctx, &sts), wsQuery(initial))
	require.Equal(t, "IAM_CONTEXT_PLUGIN_FORBIDDEN", dto.CodeOf(err))
}

func TestWSRevalidationRequiresServerHandleAndTypedOperation(t *testing.T) {
	f := newFixture(t, false)
	regular, err := f.s.Resolve(f.svc, query(f.issue(t, "http")))
	require.NoError(t, err)
	require.Empty(t, regular.WSRevalidationHandle)
	for _, pair := range []struct{ handle, code string }{{"", "IAM_CONTEXT_WS_HANDLE_REQUIRED"}, {"forged", "IAM_CONTEXT_WS_HANDLE_INVALID"}, {f.issue(t, "ws_grant"), "IAM_CONTEXT_WS_HANDLE_INVALID"}} {
		_, err = f.s.RevalidateWS(f.svc, WSQuery{Schema: WSQuerySchema, Handle: pair.handle})
		require.Equal(t, pair.code, dto.CodeOf(err))
	}
	initial, err := f.s.Resolve(f.svc, query(f.issue(t, "ws_upgrade")))
	require.NoError(t, err)
	other := NewService(f.db, f.store, Options{Namespace: "other-deployment", UserIssuer: "powerx-auth", UserAudience: "user"})
	_, err = other.RevalidateWS(f.svc, wsQuery(initial))
	require.Equal(t, "IAM_CONTEXT_WS_HANDLE_EXPIRED", dto.CodeOf(err))
	raw, err := json.Marshal(WSOperation{Operation: "revalidate_ws_identity", Request: wsQuery(initial)})
	require.NoError(t, err)
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &body))
	invoker := Invoker{Service: f.s}
	result, err := invoker.InvokeCoreCapability(f.svc, cap.CoreCapabilityInvokeInput{CapabilityID: CapabilityID, Endpoint: Endpoint, Method: "INVOKE", TenantUUID: f.tenant, Body: body})
	require.NoError(t, err)
	require.Equal(t, initial.WSRevalidationHandle, result["result"].(Result).WSRevalidationHandle)
	body["request"].(map[string]interface{})["user_uuid"] = uuid.NewString()
	_, err = invoker.InvokeCoreCapability(f.svc, cap.CoreCapabilityInvokeInput{CapabilityID: CapabilityID, Endpoint: Endpoint, Method: "INVOKE", TenantUUID: f.tenant, Body: body})
	require.Equal(t, "IAM_CONTEXT_INVALID_ARGUMENT", dto.CodeOf(err))
}

func TestWSRevalidationDependencyFailureNeverUsesPreviousIdentity(t *testing.T) {
	f := newFixture(t, false)
	initial, err := f.s.Resolve(f.svc, query(f.issue(t, "ws_grant")))
	require.NoError(t, err)
	db, err := f.db.DB()
	require.NoError(t, err)
	require.NoError(t, db.Close())
	result, err := f.s.RevalidateWS(f.svc, wsQuery(initial))
	require.Equal(t, 503, dto.StatusCode(err))
	require.Equal(t, "IAM_CONTEXT_UNAVAILABLE", dto.CodeOf(err))
	require.Empty(t, result.Permissions)
	require.Empty(t, result.WSRevalidationHandle)
}
