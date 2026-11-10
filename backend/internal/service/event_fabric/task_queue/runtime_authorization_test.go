package taskqueue

import (
	"context"
	"encoding/json"
	"testing"

	cap "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	cm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	im "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	gm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

func TestRuntimeQueueLiveAPIKeyGrantAndStrictBoundary(t *testing.T) {
	d, db, _ := fixture(t, "fixture_environment")
	require.NoError(t, db.AutoMigrate(&cm.CapabilityRecord{}, &cm.CapabilityRegistration{}, &im.Permission{}, &gm.IntegrationGatewayAPIKey{}, &gm.IntegrationGatewayAPIKeyPermission{}))
	tenant := uuid.NewString()
	for _, id := range []string{EnqueueCapability, ConsumeCapability} {
		require.NoError(t, db.Create(&cm.CapabilityRecord{CapabilityID: id, PluginID: "core", PluginVersion: "1", Title: id, CapabilitiesHash: id, ProtocolHash: id, Status: "published"}).Error)
		require.NoError(t, db.Create(&cm.CapabilityRegistration{CapabilityID: id, TenantUUID: tenant, ContractRef: "taskqueue.v2", Status: "published", Version: 1, RoutingPolicyID: uuid.New()}).Error)
	}
	meta, _ := json.Marshal(map[string]any{"capability_id": EnqueueCapability, "api_key_explicit": true, "api_key": map[string]any{"scope": "_scope.runtime.taskqueue.enqueue", "action": "enqueue", "resource_type": "capability", "resource_pattern": "runtime_taskqueue_enqueue", "effect": "allow"}})
	require.NoError(t, db.Create(&im.Permission{Module: "runtime_taskqueue", Resource: "service_enqueue", Action: "enqueue", Effect: "allow", Status: im.PermissionStatusActive, AllowAPIKey: true, Meta: datatypes.JSON(meta)}).Error)
	key := gm.IntegrationGatewayAPIKey{TenantUUID: tenant, ProfileID: 1, Name: "fixture", KeyPrefix: "fixture", KeyHash: "fixture_hash", Status: "active"}
	require.NoError(t, db.Create(&key).Error)
	grant := gm.IntegrationGatewayAPIKeyPermission{APIKeyUUID: key.UUID, Scope: "_scope.runtime.taskqueue.enqueue", Action: "enqueue", ResourceType: "capability", ResourcePattern: "runtime_taskqueue_enqueue", Effect: "allow"}
	require.NoError(t, db.Create(&grant).Error)
	ctx := reqctx.WithAuthenticatedAPIKeyHash(reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), tenant), &reqctx.CoreXClaims{TenantUUID: tenant, MemberID: 1, Platforms: []string{"api_key"}}), key.KeyHash)
	invoker := NewRuntimeInvoker(db, d)
	msg := runtimeFixtureMessage()
	msg.TenantKey = tenant
	body := RuntimeRequest{ContractVersion: RuntimeContract, Operation: "enqueue", TenantKey: tenant, Message: msg}
	raw, _ := json.Marshal(body)
	var object map[string]any
	require.NoError(t, json.Unmarshal(raw, &object))
	in := cap.CoreCapabilityInvokeInput{CapabilityID: EnqueueCapability, TenantUUID: tenant, Method: "INVOKE", Endpoint: RuntimeEndpoint, Body: object, Payload: map[string]any{"body": body}}
	_, err := invoker.InvokeCoreCapability(ctx, in)
	require.NoError(t, err)
	forbidden := in
	forbidden.Endpoint = "http://127.0.0.1/fixture"
	_, err = invoker.InvokeCoreCapability(ctx, forbidden)
	require.Equal(t, 400, dto.StatusCode(err))
	forbidden = in
	forbidden.Payload = map[string]any{"body": body, "headers": map[string]any{}}
	_, err = invoker.InvokeCoreCapability(ctx, forbidden)
	require.Equal(t, 400, dto.StatusCode(err))
	forbidden = in
	forbidden.CapabilityID = ConsumeCapability
	_, err = invoker.InvokeCoreCapability(ctx, forbidden)
	require.Equal(t, 403, dto.StatusCode(err))
	_, err = invoker.ApplyAdmin(ctx, body)
	require.Equal(t, 403, dto.StatusCode(err))
	require.NoError(t, db.Unscoped().Delete(&grant).Error)
	_, err = invoker.InvokeCoreCapability(ctx, in)
	require.Equal(t, 403, dto.StatusCode(err))
	require.NoError(t, db.Create(&grant).Error)
	require.NoError(t, db.Create(&cm.CapabilityRegistration{CapabilityID: EnqueueCapability, TenantUUID: tenant, ContractRef: "taskqueue.v2", Status: "disabled", Version: 2, RoutingPolicyID: uuid.New()}).Error)
	_, err = invoker.InvokeCoreCapability(ctx, in)
	require.Equal(t, 403, dto.StatusCode(err))
}

func TestRuntimeQueueAdminRequiresUserJWTAndLiveRBAC(t *testing.T) {
	d, db, _ := fixture(t, "fixture_environment")
	tenant, user := uuid.NewString(), uuid.NewString()
	invoker := NewRuntimeInvoker(db, d)
	msg := runtimeFixtureMessage()
	msg.TenantKey = tenant
	body := RuntimeRequest{ContractVersion: RuntimeContract, Operation: "enqueue", TenantKey: tenant, Message: msg}
	ctx := reqctx.WithUserID(reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), tenant), &reqctx.CoreXClaims{TenantUUID: tenant, UserUUID: user}), 1)
	_, err := invoker.ApplyAdmin(ctx, body)
	require.Equal(t, 403, dto.StatusCode(err))
	_, err = invoker.ApplyAdmin(reqctx.WithIsRoot(ctx, true), body)
	require.NoError(t, err)
	body.TenantKey = uuid.NewString()
	_, err = invoker.ApplyAdmin(reqctx.WithIsRoot(ctx, true), body)
	require.Equal(t, 400, dto.StatusCode(err))
}
