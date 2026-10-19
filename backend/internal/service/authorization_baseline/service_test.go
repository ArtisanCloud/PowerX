package authorization_baseline

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	cap "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	core "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	cm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	im "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	gw "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	sm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	tm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	baseline "github.com/ArtisanCloud/PowerX/pkg/corex/iam/authorizationbaseline"
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
	s              *Service
	db             *gorm.DB
	user           reqctx.CoreXClaims
	svc            context.Context
	tenant, plugin string
	key            gw.IntegrationGatewayAPIKey
	permission     im.Permission
	member         im.Member
}

func (f fixture) initialize(t *testing.T, codes ...string) InitializationInput {
	t.Helper()
	in := InitializationInput{TenantUUID: f.tenant, PluginID: f.plugin, PermissionCodes: codes, GuardUUID: uuid.NewString(), Operator: "test-operator", Apply: true}
	_, err := f.s.Initialize(context.Background(), in)
	require.NoError(t, err)
	return in
}
func (f fixture) query(codes ...string) Query {
	return Query{Schema: QuerySchema, MemberUUIDs: []string{f.member.UUID.String()}, PermissionCodes: codes}
}

func TestAutomaticBaselineIsReadableWithoutPluginGuardAndReadsStayReadOnly(t *testing.T) {
	f := newFixture(t, false)
	require.NoError(t, baseline.EnsureInitialized(f.db))
	var before im.AuthorizationTracking
	require.NoError(t, f.db.Where("tenant_uuid=?", f.tenant).Take(&before).Error)
	require.Equal(t, baseline.AutomaticInitializer, before.InitializedBy)
	var segmentsBefore []im.AuthorizationSegment
	require.NoError(t, f.db.Order("id").Find(&segmentsBefore).Error)
	for range 2 {
		result, err := f.s.ReadMemberBaselines(f.svc, f.query("operations.order:read"))
		require.NoError(t, err)
		require.True(t, *result.Items[0].Allowed)
		require.Equal(t, "initialized", result.Items[0].BaselineKind)
		require.Nil(t, result.Items[0].GrantedAt)
		precheck, err := f.s.Initialize(context.Background(), InitializationInput{TenantUUID: f.tenant, PluginID: f.plugin, PermissionCodes: []string{"operations.order:read"}})
		require.NoError(t, err)
		require.False(t, precheck.Applied)
		require.Equal(t, "ready", precheck.Tracking["operations.order:read"].Status)
	}
	// An old operator call can reuse a Core receipt but cannot replace its start or guard.
	f.initialize(t, "operations.order:read")
	var after im.AuthorizationTracking
	require.NoError(t, f.db.Where("tenant_uuid=?", f.tenant).Take(&after).Error)
	require.Equal(t, before, after)
	var segmentsAfter []im.AuthorizationSegment
	require.NoError(t, f.db.Order("id").Find(&segmentsAfter).Error)
	require.Equal(t, segmentsBefore, segmentsAfter)
}

func TestStartupDoesNotInventHistoryForExternallyChangedAuthorization(t *testing.T) {
	f := newFixture(t, false)
	require.NoError(t, baseline.EnsureInitialized(f.db))
	before, err := f.s.ReadMemberBaselines(f.svc, f.query("operations.order:read"))
	require.NoError(t, err)
	require.True(t, *before.Items[0].Allowed)
	var streamsBefore []im.AuthorizationStream
	require.NoError(t, f.db.Find(&streamsBefore).Error)
	sqlDB, err := f.db.DB()
	require.NoError(t, err)
	// Simulates unsupported offline SQL; normal IAM APIs must use the coordinator.
	_, err = sqlDB.Exec("DELETE FROM "+(&im.RoleBinding{}).TableName()+" WHERE tenant_uuid=?", f.tenant)
	require.NoError(t, err)
	require.NoError(t, baseline.EnsureInitialized(f.db))
	after, err := f.s.ReadMemberBaselines(f.svc, f.query("operations.order:read"))
	require.NoError(t, err)
	require.Equal(t, baseline.Unavailable, after.Items[0].ErrorCode)
	var streamsAfter []im.AuthorizationStream
	require.NoError(t, f.db.Find(&streamsAfter).Error)
	require.Equal(t, streamsBefore, streamsAfter, "startup must not reconcile unknown history")
}

func TestReadInitializedAndMissingMemberAreDistinctFromDenial(t *testing.T) {
	f := newFixture(t, false)
	cost := im.Permission{Module: "operations", Resource: "cost", Action: "read", Effect: "allow", Status: im.PermissionStatusActive, Source: "plugin:" + f.plugin, Meta: datatypes.JSON(`{"authorization_baseline":true}`)}
	require.NoError(t, f.db.Create(&cost).Error)
	in := f.initialize(t, "operations.order:read", "operations.cost:read")
	result, err := f.s.ReadMemberBaselines(f.svc, f.query("operations.order:read", "operations.cost:read"))
	require.NoError(t, err)
	require.Len(t, result.Items, 2)
	read := result.Items[0]
	require.Equal(t, "decided", read.Status)
	require.True(t, *read.Allowed)
	require.Equal(t, "initialized", read.BaselineKind)
	require.Nil(t, read.GrantedAt)
	require.NotNil(t, read.VisibleSince)
	require.NotEmpty(t, read.AuthorizationRevision)
	require.False(t, *result.Items[1].Allowed)
	query := f.query("operations.order:read", "operations.cost:read")
	query.MemberUUIDs = append(query.MemberUUIDs, uuid.NewString())
	batch, err := f.s.ReadMemberBaselines(f.svc, query)
	require.NoError(t, err)
	require.Len(t, batch.Items, 4)
	require.Equal(t, "IAM_BASELINE_MEMBER_UNAVAILABLE", batch.Items[2].ErrorCode)
	raw, err := json.Marshal(batch.Items[2])
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	require.NotContains(t, decoded, "allowed")
	require.NotContains(t, decoded, "granted_at")
	// 同一收据重放不能移动 T0 或增加修订。
	_, err = f.s.Initialize(context.Background(), in)
	require.NoError(t, err)
	same, err := f.s.ReadMemberBaselines(f.svc, f.query("operations.order:read"))
	require.NoError(t, err)
	require.Equal(t, read.AuthorizationRevision, same.Items[0].AuthorizationRevision)
}

func TestRootMemberRequiresFormalTenantCostGrant(t *testing.T) {
	f := newFixture(t, false)
	require.NoError(t, f.db.Model(&im.User{}).Where("id=?", f.user.UserID).Update("is_root", true).Error)
	cost := im.Permission{Module: "operations", Resource: "cost", Action: "read", Effect: "allow", Status: im.PermissionStatusActive, Source: "plugin:" + f.plugin, Meta: datatypes.JSON(`{"authorization_baseline":true}`)}
	require.NoError(t, f.db.Create(&cost).Error)
	f.initialize(t, "operations.order:read", "operations.cost:read")
	before, err := f.s.ReadMemberBaselines(f.svc, f.query("operations.order:read", "operations.cost:read"))
	require.NoError(t, err)
	require.True(t, *before.Items[0].Allowed, "Root 的正式成员授权可用于业务基线")
	require.Equal(t, baseline.RootBypass, before.Items[1].ErrorCode)
	require.Nil(t, before.Items[1].Allowed)
	role := im.Role{Scope: "tenant", TenantUUID: f.tenant, Code: "cost_reader", Name: "Cost Reader"}
	require.NoError(t, f.db.Create(&role).Error)
	require.NoError(t, f.db.Create(&im.RolePermission{RoleID: role.ID, RoleUUID: role.UUID, PermissionID: cost.ID, PermissionUUID: cost.PermissionUUID}).Error)
	binding := im.RoleBinding{TenantUUID: f.tenant, RoleID: role.ID, RoleUUID: role.UUID.String(), SubjectType: im.SubMember, SubjectID: f.member.ID, SubjectUUID: f.member.UUID.String(), DataScope: im.ScopeTenant}
	require.NoError(t, f.db.Create(&binding).Error)
	allowed, err := f.s.ReadMemberBaselines(f.svc, f.query("operations.cost:read"))
	require.NoError(t, err)
	require.Equal(t, "decided", allowed.Items[0].Status)
	require.True(t, *allowed.Items[0].Allowed)
	require.Equal(t, "TENANT", allowed.Items[0].DataScope)
	require.Equal(t, "native", allowed.Items[0].BaselineKind)
	require.NotNil(t, allowed.Items[0].GrantedAt)
	// 撤销正式来源后，Root 全局旁路仍不能维持个人业务授权或旧阅读窗口。
	require.NoError(t, f.db.Delete(&binding).Error)
	revoked, err := f.s.ReadMemberBaselines(f.svc, f.query("operations.cost:read"))
	require.NoError(t, err)
	require.Equal(t, baseline.RootBypass, revoked.Items[0].ErrorCode)
	require.Nil(t, revoked.Items[0].Allowed)
}

func TestReadUnavailableScopePolicyRootAndRevocation(t *testing.T) {
	f := newFixture(t, false)
	notInitialized, err := f.s.ReadMemberBaselines(f.svc, f.query("operations.order:read"))
	require.NoError(t, err)
	require.Equal(t, "IAM_BASELINE_UNAVAILABLE", notInitialized.Items[0].ErrorCode)
	f.initialize(t, "operations.order:read")
	require.NoError(t, f.db.Model(&im.RoleBinding{}).Where("tenant_uuid=?", f.tenant).Update("data_scope", im.ScopeSelf).Error)
	scoped, err := f.s.ReadMemberBaselines(f.svc, f.query("operations.order:read"))
	require.NoError(t, err)
	require.Equal(t, baseline.UnsupportedScope, scoped.Items[0].ErrorCode)
	require.NoError(t, f.db.Model(&im.RoleBinding{}).Where("tenant_uuid=?", f.tenant).Update("data_scope", im.ScopeTenant).Error)
	before, err := f.s.ReadMemberBaselines(f.svc, f.query("operations.order:read"))
	require.NoError(t, err)
	require.Equal(t, baseline.Unavailable, before.Items[0].ErrorCode)
	require.NoError(t, f.db.Model(&im.Member{}).Where("uuid=?", f.member.UUID).Update("status", 2).Error)
	disabled, err := f.s.ReadMemberBaselines(f.svc, f.query("operations.order:read"))
	require.NoError(t, err)
	require.False(t, *disabled.Items[0].Allowed)
	require.NoError(t, f.db.Model(&im.Member{}).Where("uuid=?", f.member.UUID).Update("status", 1).Error)
	after, err := f.s.ReadMemberBaselines(f.svc, f.query("operations.order:read"))
	require.NoError(t, err)
	require.Equal(t, "decided", after.Items[0].Status)
	require.NotNil(t, after.Items[0].GrantedAt)
	require.NoError(t, f.db.Model(&im.User{}).Where("uuid=?", f.user.UserUUID).Update("is_root", true).Error)
	require.NoError(t, f.db.Where("tenant_uuid=?", f.tenant).Delete(&im.RoleBinding{}).Error)
	bypass, err := f.s.ReadMemberBaselines(f.svc, f.query("operations.order:read"))
	require.NoError(t, err)
	require.Equal(t, baseline.RootBypass, bypass.Items[0].ErrorCode)
}

func TestServiceGrantPublicationOwnerAndExpiryAreLive(t *testing.T) {
	f := newFixture(t, false)
	f.initialize(t, "operations.order:read")
	sts := reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), f.tenant), &reqctx.CoreXClaims{TenantUUID: f.tenant, PluginID: f.plugin, RegisteredClaims: jwt.RegisteredClaims{Issuer: "powerx-sts", Subject: "client:client-fixture", Audience: []string{"powerx:api"}, ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute))}})
	_, err := f.s.ReadMemberBaselines(sts, f.query("operations.order:read"))
	require.NoError(t, err)
	require.NoError(t, f.db.Model(&cm.CapabilityRegistration{}).Where("tenant_uuid=? AND capability_id=?", f.tenant, CapabilityID).Update("status", "disabled").Error)
	_, err = f.s.ReadMemberBaselines(f.svc, f.query("operations.order:read"))
	require.Equal(t, 403, dto.StatusCode(err))
	require.NoError(t, f.db.Model(&cm.CapabilityRegistration{}).Where("tenant_uuid=? AND capability_id=?", f.tenant, CapabilityID).Update("status", "published").Error)
	require.NoError(t, f.db.Model(&gw.IntegrationGatewayAPIKey{}).Where("uuid=?", f.key.UUID).Update("expires_at", time.Now().Add(-time.Minute)).Error)
	_, err = f.s.ReadMemberBaselines(f.svc, f.query("operations.order:read"))
	require.Equal(t, 401, dto.StatusCode(err))
	require.NoError(t, f.db.Model(&gw.IntegrationGatewayAPIKey{}).Where("uuid=?", f.key.UUID).Update("expires_at", nil).Error)
	require.NoError(t, f.db.Where("api_key_uuid=?", f.key.UUID).Delete(&gw.IntegrationGatewayAPIKeyPermission{}).Error)
	_, err = f.s.ReadMemberBaselines(f.svc, f.query("operations.order:read"))
	require.Equal(t, 403, dto.StatusCode(err))
}

func TestAmbiguousPluginAndInvalidBatchAreRejected(t *testing.T) {
	f := newFixture(t, false)
	f.initialize(t, "operations.order:read")
	other := "com.example.other"
	policy, _ := json.Marshal(map[string]any{"version": 1, "mode": "key", "plugin_ids": []string{f.plugin, other}})
	require.NoError(t, f.db.Model(&gw.IntegrationGatewayAPIKey{}).Where("uuid=?", f.key.UUID).Update("plugin_owner_policy", policy).Error)
	require.NoError(t, f.db.Create(&gw.IntegrationGatewayAPIKeyPermission{APIKeyUUID: f.key.UUID, PluginID: other, Scope: APIKeyScope, Action: "read", ResourceType: "capability", ResourcePattern: APIKeyResource, Effect: "allow"}).Error)
	_, err := f.s.ReadMemberBaselines(f.svc, f.query("operations.order:read"))
	require.Equal(t, "IAM_BASELINE_PLUGIN_AMBIGUOUS", dto.CodeOf(err))
	query := f.query("operations.order:read")
	query.MemberUUIDs = append(query.MemberUUIDs, query.MemberUUIDs[0])
	_, err = f.s.ReadMemberBaselines(f.svc, query)
	require.Equal(t, 400, dto.StatusCode(err))
}

func TestWildcardAPIKeyGrantCannotIdentifyTheCallingPlugin(t *testing.T) {
	f := newFixture(t, false)
	f.initialize(t, "operations.order:read")
	require.NoError(t, f.db.Model(&gw.IntegrationGatewayAPIKeyPermission{}).Where("api_key_uuid=?", f.key.UUID).Update("resource_pattern", "*").Error)
	_, err := f.s.ReadMemberBaselines(f.svc, f.query("operations.order:read"))
	require.Equal(t, 403, dto.StatusCode(err))
	require.Equal(t, "IAM_BASELINE_SERVICE_FORBIDDEN", dto.CodeOf(err))
}

func TestTypedInvokerRejectsFreeProxyAndUnknownFields(t *testing.T) {
	f := newFixture(t, false)
	f.initialize(t, "operations.order:read")
	query, _ := json.Marshal(f.query("operations.order:read"))
	var q map[string]any
	require.NoError(t, json.Unmarshal(query, &q))
	body := map[string]any{"operation": Operation, "request": q}
	invoke := &Invoker{Service: f.s}
	in := cap.CoreCapabilityInvokeInput{CapabilityID: CapabilityID, TenantUUID: f.tenant, Method: "INVOKE", Endpoint: Endpoint, Body: body, Payload: map[string]any{"body": body}}
	_, err := invoke.InvokeCoreCapability(f.svc, in)
	require.NoError(t, err)
	in.Payload["headers"] = map[string]any{"Authorization": "untrusted"}
	_, err = invoke.InvokeCoreCapability(f.svc, in)
	require.Equal(t, 400, dto.StatusCode(err))
	delete(in.Payload, "headers")
	q["tenant_uuid"] = uuid.NewString()
	_, err = invoke.InvokeCoreCapability(f.svc, in)
	require.Equal(t, 400, dto.StatusCode(err))
	delete(q, "tenant_uuid")
	body["operation"] = "proxy"
	_, err = invoke.InvokeCoreCapability(f.svc, in)
	require.Equal(t, 400, dto.StatusCode(err))
}

func TestScheduledInternalBoundariesChangeRevisionWithoutAWrite(t *testing.T) {
	f := newFixture(t, false)
	base := baseline.Micro(time.Now())
	half := base.Add(30 * time.Minute)
	one := base.Add(time.Hour)
	two := base.Add(2 * time.Hour)
	require.NoError(t, f.db.Model(&im.RoleBinding{}).Where("tenant_uuid=?", f.tenant).Update("end_at", one.UnixMilli()).Error)
	role := im.Role{TenantUUID: f.tenant, Code: "second_reader"}
	require.NoError(t, f.db.Create(&role).Error)
	require.NoError(t, f.db.Create(&im.RolePermission{RoleID: role.ID, RoleUUID: role.UUID, PermissionID: f.permission.ID, PermissionUUID: f.permission.PermissionUUID}).Error)
	start, end := half.UnixMilli(), two.UnixMilli()
	require.NoError(t, f.db.Create(&im.RoleBinding{TenantUUID: f.tenant, RoleID: role.ID, RoleUUID: role.UUID.String(), SubjectType: im.SubMember, SubjectID: f.member.ID, SubjectUUID: f.member.UUID.String(), StartAt: &start, EndAt: &end}).Error)
	f.initialize(t, "operations.order:read")
	var countBefore, countAfter int64
	require.NoError(t, f.db.Model(&im.AuthorizationSegment{}).Count(&countBefore).Error)
	results := []Result{}
	for _, at := range []time.Time{base.Add(time.Minute), half.Add(time.Minute), one.Add(time.Minute)} {
		f.s.clock = func() time.Time { return at }
		result, err := f.s.ReadMemberBaselines(f.svc, f.query("operations.order:read"))
		require.NoError(t, err)
		require.True(t, *result.Items[0].Allowed)
		results = append(results, result)
	}
	require.Equal(t, results[0].Items[0].VisibleSince, results[2].Items[0].VisibleSince)
	require.NotEqual(t, results[0].Items[0].AuthorizationRevision, results[1].Items[0].AuthorizationRevision)
	require.NotEqual(t, results[1].Items[0].AuthorizationRevision, results[2].Items[0].AuthorizationRevision)
	require.Equal(t, format(time.UnixMilli(two.UnixMilli())), *results[0].Items[0].ValidUntil)
	require.NoError(t, f.db.Model(&im.AuthorizationSegment{}).Count(&countAfter).Error)
	require.Equal(t, countBefore, countAfter)
}

func TestInitializationPreflightReportsEachPermissionAndDoesNotWrite(t *testing.T) {
	f := newFixture(t, false)
	input := InitializationInput{TenantUUID: f.tenant, PluginID: f.plugin, PermissionCodes: []string{"operations.order:read"}}
	before, err := f.s.Initialize(context.Background(), input)
	require.NoError(t, err)
	require.False(t, before.Applied)
	require.Equal(t, "uninitialized", before.Tracking["operations.order:read"].Status)
	var count int64
	require.NoError(t, f.db.Model(&im.AuthorizationTracking{}).Count(&count).Error)
	require.Zero(t, count)
	f.initialize(t, "operations.order:read")
	after, err := f.s.Initialize(context.Background(), input)
	require.NoError(t, err)
	require.False(t, after.Applied)
	require.Equal(t, "ready", after.Tracking["operations.order:read"].Status)
	require.NotNil(t, after.Tracking["operations.order:read"].KnownSince)
}

func TestCorruptSavedDecisionCannotBecomeAllowOrDenial(t *testing.T) {
	f := newFixture(t, false)
	f.initialize(t, "operations.order:read")
	require.NoError(t, f.db.Model(&im.AuthorizationSegment{}).Where("tenant_uuid=?", f.tenant).Update("state", "unknown").Error)
	result, err := f.s.ReadMemberBaselines(f.svc, f.query("operations.order:read"))
	require.NoError(t, err)
	require.Equal(t, "IAM_BASELINE_UNAVAILABLE", result.Items[0].ErrorCode)
	require.Nil(t, result.Items[0].Allowed)
}
func newFixture(t *testing.T, root bool) fixture {
	t.Helper()
	old := core.PowerXSchema
	core.PowerXSchema = "main"
	t.Cleanup(func() { core.PowerXSchema = old })
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return seedServiceFixture(t, db, root)
}

func seedServiceFixture(t *testing.T, db *gorm.DB, root bool) fixture {
	require.NoError(t, db.AutoMigrate(&im.User{}, &im.Member{}, &im.RefreshToken{}, &im.AuthorizationTracking{}, &im.AuthorizationStream{}, &im.AuthorizationSegment{}, &tm.Tenant{}, &im.Role{}, &im.Permission{}, &im.RolePermission{}, &im.RoleBinding{}, &im.MemberAssignment{}, &cm.CapabilityRecord{}, &cm.CapabilityRegistration{}, &sm.PluginInstanceConfig{}, &gw.IntegrationGatewayAPIKey{}, &gw.IntegrationGatewayAPIKeyPermission{}, &im.APIKeyProfile{}))
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
	require.NoError(t, db.Create(&gw.IntegrationGatewayAPIKeyPermission{APIKeyUUID: key.UUID, Scope: APIKeyScope, Action: "read", ResourceType: "capability", ResourcePattern: APIKeyResource, Effect: "allow", PluginID: pluginID}).Error)
	meta, _ := json.Marshal(map[string]any{"capability_id": CapabilityID, "api_key_explicit": true, "api_key": map[string]any{"scope": APIKeyScope, "action": "read", "resource_type": "capability", "resource_pattern": APIKeyResource, "effect": "allow"}})
	require.NoError(t, db.Create(&im.Permission{Module: "iam", Resource: "baseline_service", Action: "read", Effect: "allow", Status: im.PermissionStatusActive, AllowAPIKey: true, Meta: meta}).Error)
	require.NoError(t, db.Create(&cm.CapabilityRecord{CapabilityID: CapabilityID, PluginID: "corex.platform", PluginVersion: "v1", Title: "identity", Status: "published"}).Error)
	require.NoError(t, db.Create(&cm.CapabilityRegistration{CapabilityID: CapabilityID, TenantUUID: tenant, ContractRef: "v1", Status: "published", Version: 1, RoutingPolicyID: uuid.New()}).Error)
	cred, _ := json.Marshal(map[string]any{"client_id": "client-fixture", "allowed_capabilities": []string{CapabilityID}})
	require.NoError(t, db.Create(&sm.PluginInstanceConfig{TenantUUID: tenant, PluginID: pluginID, Key: "auth.credentials", Enabled: true, ValueJSON: cred}).Error)
	permission := im.Permission{Module: "operations", Resource: "order", Action: "read", Effect: "allow", Status: im.PermissionStatusActive, Source: "plugin:" + pluginID, Meta: datatypes.JSON(`{"type":"action","authorization_baseline":true}`)}
	require.NoError(t, db.Create(&permission).Error)
	role := im.Role{TenantUUID: tenant, Code: "role_user", Name: "member"}
	require.NoError(t, db.Create(&role).Error)
	require.NoError(t, db.Create(&im.RolePermission{RoleID: role.ID, PermissionID: permission.ID, RoleUUID: role.UUID, PermissionUUID: permission.PermissionUUID}).Error)
	if !root {
		require.NoError(t, db.Create(&im.RoleBinding{TenantUUID: tenant, RoleID: role.ID, RoleUUID: role.UUID.String(), SubjectType: im.SubMember, SubjectID: member.ID, SubjectUUID: memberUUID}).Error)
	}
	s := NewService(db)
	ctx := reqctx.WithAuthenticatedAPIKeyHash(reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), tenant), &reqctx.CoreXClaims{TenantUUID: tenant, Platforms: []string{"api_key"}}), key.KeyHash)
	require.NoError(t, baseline.Install(db))
	return fixture{s: s, db: db, user: u, svc: ctx, tenant: tenant, plugin: pluginID, key: key, permission: permission, member: member}
}
