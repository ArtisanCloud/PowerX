package plugin_identity

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	cap "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	"github.com/ArtisanCloud/PowerX/pkg/cache"
	core "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	cm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	im "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	gw "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	sm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	tm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type fixture struct {
	s          *Service
	db         *gorm.DB
	store      cache.ICache
	user       reqctx.CoreXClaims
	svc        context.Context
	tenant     string
	plugin     string
	key        gw.IntegrationGatewayAPIKey
	permission im.Permission
	session    im.RefreshToken
}

func newFixture(t *testing.T, root bool) fixture {
	t.Helper()
	old := core.PowerXSchema
	core.PowerXSchema = "main"
	t.Cleanup(func() { core.PowerXSchema = old })
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&im.User{}, &im.Member{}, &im.RefreshToken{}, &tm.Tenant{}, &im.Role{}, &im.Permission{}, &im.RolePermission{}, &im.RoleBinding{}, &im.MemberAssignment{}, &cm.CapabilityRecord{}, &cm.CapabilityRegistration{}, &sm.PluginInstanceConfig{}, &gw.IntegrationGatewayAPIKey{}, &gw.IntegrationGatewayAPIKeyPermission{}, &im.APIKeyProfile{}))
	tenant, pluginID := uuid.NewString(), "com.powerx.plugins.identity-fixture"
	user := im.User{Email: uuid.NewString() + "@fixture.invalid", Status: 1, IsRoot: root}
	require.NoError(t, db.Create(&user).Error)
	ten := tm.Tenant{PowerUUIDModel: core.PowerUUIDModel{UUID: uuid.MustParse(tenant)}, Key: uuid.NewString(), Name: "fixture", Status: 1}
	require.NoError(t, db.Create(&ten).Error)
	member := im.Member{TenantUUID: tenant, UserUUID: user.UUID.String(), UserID: user.ID, Username: "member", Status: 1}
	memberUUID := ""
	if !root {
		require.NoError(t, db.Create(&member).Error)
		memberUUID = member.UUID.String()
	}
	session := im.RefreshToken{JTI: uuid.NewString(), UserUUID: user.UUID.String(), MemberUUID: memberUUID, TenantUUID: tenant, ExpiresAt: time.Now().Add(time.Hour).UnixMilli()}
	require.NoError(t, db.Create(&session).Error)
	u := reqctx.CoreXClaims{TenantUUID: tenant, UserUUID: user.UUID.String(), UserID: user.ID, MemberUUID: memberUUID, MemberID: member.ID, IsRoot: root, Scope: "access", RegisteredClaims: jwt.RegisteredClaims{Issuer: "powerx-auth", Audience: []string{"user"}, ID: session.JTI, ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute))}}
	profile := im.APIKeyProfile{TenantUUID: tenant, Name: "fixture", Status: 1}
	require.NoError(t, db.Create(&profile).Error)
	key := gw.IntegrationGatewayAPIKey{TenantUUID: tenant, ProfileID: profile.ID, Name: "fixture", KeyPrefix: "pxk_fixture", KeyHash: "fixture-hash", Status: "active", PluginOwnerPolicy: datatypes.JSON(fmt.Sprintf(`{"version":1,"mode":"key","plugin_ids":[%q]}`, pluginID))}
	require.NoError(t, db.Create(&key).Error)
	require.NoError(t, db.Create(&gw.IntegrationGatewayAPIKeyPermission{APIKeyUUID: key.UUID, Scope: APIKeyScope, Action: "read", ResourceType: "capability", ResourcePattern: "iam_identity_context_read", Effect: "allow", PluginID: pluginID}).Error)
	meta, _ := json.Marshal(map[string]any{"capability_id": CapabilityID, "api_key_explicit": true, "api_key": map[string]any{"scope": APIKeyScope, "action": "read", "resource_type": "capability", "resource_pattern": "iam_identity_context_read", "effect": "allow"}})
	require.NoError(t, db.Create(&im.Permission{Module: "iam", Resource: "identity_service", Action: "read", Effect: "allow", Status: im.PermissionStatusActive, AllowAPIKey: true, Meta: meta}).Error)
	require.NoError(t, db.Create(&cm.CapabilityRecord{CapabilityID: CapabilityID, PluginID: "corex.platform", PluginVersion: "v1", Title: "identity", Status: "published"}).Error)
	require.NoError(t, db.Create(&cm.CapabilityRegistration{CapabilityID: CapabilityID, TenantUUID: tenant, ContractRef: "v1", Status: "published", Version: 1, RoutingPolicyID: uuid.New()}).Error)
	cred, _ := json.Marshal(map[string]any{"client_id": "client-fixture", "allowed_capabilities": []string{CapabilityID}})
	require.NoError(t, db.Create(&sm.PluginInstanceConfig{TenantUUID: tenant, PluginID: pluginID, Key: "auth.credentials", Enabled: true, ValueJSON: cred}).Error)
	permission := im.Permission{Module: "operations", Resource: "order", Action: "read", Effect: "allow", Status: im.PermissionStatusActive, Source: "plugin:" + pluginID, Meta: datatypes.JSON(`{"type":"action"}`)}
	require.NoError(t, db.Create(&permission).Error)
	role := im.Role{TenantUUID: tenant, Code: "role_user", Name: "member"}
	require.NoError(t, db.Create(&role).Error)
	require.NoError(t, db.Create(&im.RolePermission{RoleID: role.ID, PermissionID: permission.ID, RoleUUID: role.UUID, PermissionUUID: permission.PermissionUUID}).Error)
	if !root {
		require.NoError(t, db.Create(&im.RoleBinding{TenantUUID: tenant, RoleID: role.ID, RoleUUID: role.UUID.String(), SubjectType: im.SubMember, SubjectID: member.ID, SubjectUUID: memberUUID}).Error)
	}
	store := cache.NewMemoryCache()
	s := NewService(db, store, Options{Namespace: "fixture", UserIssuer: "powerx-auth", UserAudience: "user"})
	ctx := reqctx.WithAuthenticatedAPIKeyHash(reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), tenant), &reqctx.CoreXClaims{TenantUUID: tenant, Platforms: []string{"api_key"}}), key.KeyHash)
	return fixture{s, db, store, u, ctx, tenant, pluginID, key, permission, session}
}
func (f fixture) issue(t *testing.T, transport string) string {
	t.Helper()
	ticket, err := f.s.Issue(context.Background(), IssueInput{PluginID: f.plugin, Claims: f.user, Method: "GET", Path: "/admin/operations/orders", Transport: transport, RequestID: uuid.NewString(), TraceID: uuid.NewString(), RequiredPermission: "operations.order:read"})
	require.NoError(t, err)
	return ticket
}
func query(ticket string) Query { return Query{Schema: QuerySchema, ContextTicket: ticket} }

func TestRequestIdentityLiveSubjectPermissionAndSession(t *testing.T) {
	f := newFixture(t, false)
	ticket := f.issue(t, "http")
	result, err := f.s.Resolve(f.svc, query(ticket))
	require.NoError(t, err)
	require.Equal(t, f.user.UserUUID, result.UserUUID)
	require.NotNil(t, result.MemberUUID)
	require.False(t, result.IsRoot)
	require.Equal(t, []string{"operations.order:read"}, result.Permissions)
	require.NoError(t, f.db.Model(&f.permission).Update("status", im.PermissionStatusDeprecated).Error)
	_, err = f.s.Resolve(f.svc, query(ticket))
	require.Equal(t, "IAM_CONTEXT_REQUEST_FORBIDDEN", dto.CodeOf(err))
	require.NoError(t, f.db.Model(&f.permission).Update("status", im.PermissionStatusActive).Error)
	now := time.Now().UnixMilli()
	require.NoError(t, f.db.Model(&f.session).Update("revoked_at", now).Error)
	_, err = f.s.Resolve(f.svc, query(ticket))
	require.Equal(t, 401, dto.StatusCode(err))
	require.Equal(t, "IAM_CONTEXT_USER_SESSION_INVALID", dto.CodeOf(err))
}
func TestRequestIdentityRootWSAndAdminPlanes(t *testing.T) {
	f := newFixture(t, true)
	ticket := f.issue(t, "ws_grant")
	result, err := f.s.Resolve(f.svc, query(ticket))
	require.NoError(t, err)
	require.True(t, result.IsRoot)
	require.Nil(t, result.MemberUUID)
	require.Equal(t, "ws_grant", result.Transport)
	admin := reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), f.tenant), &f.user)
	result, err = f.s.ResolveAdmin(admin, query(ticket))
	require.NoError(t, err)
	require.True(t, result.IsRoot)
	_, err = f.s.Resolve(admin, query(ticket))
	require.Equal(t, "IAM_CONTEXT_SERVICE_CREDENTIAL_INVALID", dto.CodeOf(err))
	_, err = f.s.ResolveAdmin(f.svc, query(ticket))
	require.Equal(t, "IAM_CONTEXT_USER_SESSION_INVALID", dto.CodeOf(err))
	wrong := f.user
	wrong.UserUUID = uuid.NewString()
	_, err = f.s.ResolveAdmin(reqctx.WithClaims(admin, &wrong), query(ticket))
	require.Equal(t, 403, dto.StatusCode(err))
	require.NoError(t, f.db.Model(&im.User{}).Where("uuid=?", f.user.UserUUID).Update("is_root", false).Error)
	_, err = f.s.Resolve(f.svc, query(ticket))
	require.Equal(t, "IAM_CONTEXT_USER_FORBIDDEN", dto.CodeOf(err))
}
func TestRequestIdentityCredentialOwnerAndTicketIsolation(t *testing.T) {
	f := newFixture(t, false)
	ticket := f.issue(t, "ws_upgrade")
	_, err := f.s.Resolve(f.svc, query(""))
	require.Equal(t, "IAM_CONTEXT_PROOF_REQUIRED", dto.CodeOf(err))
	_, err = f.s.Resolve(f.svc, query("forged"))
	require.Equal(t, "IAM_CONTEXT_PROOF_INVALID", dto.CodeOf(err))
	other := NewService(f.db, f.store, Options{Namespace: "other-deployment"})
	_, err = other.Resolve(f.svc, query(ticket))
	require.Equal(t, "IAM_CONTEXT_PROOF_EXPIRED", dto.CodeOf(err))
	require.NoError(t, f.db.Model(&f.key).Update("plugin_owner_policy", datatypes.JSON(`{"version":1,"mode":"key","plugin_ids":[]}`)).Error)
	_, err = f.s.Resolve(f.svc, query(ticket))
	require.Equal(t, "IAM_CONTEXT_PLUGIN_FORBIDDEN", dto.CodeOf(err))
	require.NoError(t, f.db.Model(&f.key).Update("status", "revoked").Error)
	_, err = f.s.Resolve(f.svc, query(ticket))
	require.Equal(t, "IAM_CONTEXT_SERVICE_CREDENTIAL_INVALID", dto.CodeOf(err))
}
func TestRequestIdentitySTSAndExpiredProof(t *testing.T) {
	f := newFixture(t, false)
	ticket := f.issue(t, "http")
	c := reqctx.CoreXClaims{TenantUUID: f.tenant, PluginID: f.plugin, RegisteredClaims: jwt.RegisteredClaims{Issuer: "powerx-sts", Subject: "client:client-fixture", Audience: []string{"powerx:api"}, ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute))}}
	ctx := reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), f.tenant), &c)
	_, err := f.s.Resolve(ctx, query(ticket))
	require.NoError(t, err)
	c.PluginID = "plugin.foreign"
	_, err = f.s.Resolve(ctx, query(ticket))
	require.Equal(t, 403, dto.StatusCode(err))
	c.PluginID = f.plugin
	c.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Second))
	_, err = f.s.Resolve(ctx, query(ticket))
	require.Equal(t, "IAM_CONTEXT_SERVICE_CREDENTIAL_EXPIRED", dto.CodeOf(err))
	f.s.clock = func() time.Time { return time.Now().Add(2 * time.Minute) }
	_, err = f.s.Resolve(f.svc, query(ticket))
	require.Equal(t, "IAM_CONTEXT_PROOF_EXPIRED", dto.CodeOf(err))
}
func TestRequestIdentityStrictInvocation(t *testing.T) {
	f := newFixture(t, false)
	ticket := f.issue(t, "http")
	i := Invoker{Service: f.s}
	in := cap.CoreCapabilityInvokeInput{CapabilityID: CapabilityID, TenantUUID: f.tenant, Endpoint: Endpoint, Method: "INVOKE", Body: map[string]interface{}{"operation": "resolve_request_identity", "request": query(ticket)}}
	result, err := i.InvokeCoreCapability(f.svc, in)
	require.NoError(t, err)
	require.NotNil(t, result["result"])
	in.Body = map[string]any{"operation": "resolve_request_identity", "request": map[string]any{"schema": QuerySchema, "context_ticket": ticket, "user_uuid": uuid.NewString()}}
	_, err = i.InvokeCoreCapability(f.svc, in)
	require.Equal(t, 400, dto.StatusCode(err))
	in.Endpoint = "https://arbitrary.invalid"
	_, err = i.InvokeCoreCapability(f.svc, in)
	require.Equal(t, 400, dto.StatusCode(err))
}

type failedStore struct{ cache.ICache }

func (f failedStore) Get(context.Context, string) ([]byte, error) {
	return nil, fmt.Errorf("fixture-store-unavailable")
}
func TestRequestIdentityDependencyFailureAndMemberDisable(t *testing.T) {
	f := newFixture(t, false)
	ticket := f.issue(t, "http")
	original := f.s.store
	f.s.store = failedStore{original}
	_, err := f.s.Resolve(f.svc, query(ticket))
	require.Equal(t, 503, dto.StatusCode(err))
	require.Equal(t, "IAM_CONTEXT_UNAVAILABLE", dto.CodeOf(err))
	f.s.store = original
	require.NoError(t, f.db.Model(&im.Member{}).Where("uuid=?", f.user.MemberUUID).Update("status", 2).Error)
	_, err = f.s.Resolve(f.svc, query(ticket))
	require.Equal(t, "IAM_CONTEXT_USER_FORBIDDEN", dto.CodeOf(err))
}
