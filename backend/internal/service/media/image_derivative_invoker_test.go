package media

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	cap "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	permissions "github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/apikeypermissions"
	hosttest "github.com/ArtisanCloud/PowerX/internal/testutil/runtime_host"
	cm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	im "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	gm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestImageDerivativeServiceGrantsAndTypedBoundary(t *testing.T) {
	t.Setenv("POWERX_MEDIA_TEST_DSN", "")
	f := newDerivativeFixture(t)
	a, _ := f.upload(t, imageFixture(t))
	f.consume(t)
	db := hosttest.Database(t)
	f.s.DB = db
	require.NoError(t, db.AutoMigrate(&im.APIKeyProfile{}, &im.Permission{}, &im.APIKeyProfilePermission{}, &gm.IntegrationGatewayAPIKey{}, &gm.IntegrationGatewayAPIKeyPermission{}))
	dir, err := filepath.Abs("../../../config/platform_capabilities")
	require.NoError(t, err)
	t.Setenv("PLATFORM_CAPABILITIES_DIR", dir)
	rows, err := permissions.BuildPlatformCapabilityPermissions()
	require.NoError(t, err)
	for _, row := range rows {
		var meta map[string]any
		require.NoError(t, json.Unmarshal(row.Meta, &meta))
		if id, ok := meta["capability_id"].(string); ok && strings.HasPrefix(id, "com.corex.media.image_derivatives.") {
			require.NoError(t, db.Create(&row).Error)
		}
	}
	for _, id := range []string{ImageDerivativesReadCapability, ImageDerivativesEnsureCapability} {
		require.NoError(t, db.Create(&cm.CapabilityRecord{CapabilityID: id, PluginID: "core", PluginVersion: "1", Title: id, CapabilitiesHash: id, ProtocolHash: id, Status: "published"}).Error)
		require.NoError(t, db.Create(&cm.CapabilityRegistration{TenantUUID: mediaTenantUUID, CapabilityID: id, ContractRef: "v1", RoutingPolicyID: uuid.New(), Status: "published", Version: 1}).Error)
	}
	key := gm.IntegrationGatewayAPIKey{TenantUUID: mediaTenantUUID, ProfileID: 1, Name: "media-test", KeyPrefix: "test", KeyHash: "media-test-hash", Status: "active"}
	require.NoError(t, db.Create(&key).Error)
	grant := gm.IntegrationGatewayAPIKeyPermission{APIKeyUUID: key.UUID, Scope: "_scope.media.image_derivatives.read", Action: "read", ResourceType: "capability", ResourcePattern: "media_image_derivatives", Effect: "allow"}
	require.NoError(t, db.Create(&grant).Error)
	ctx := reqctx.WithAuthenticatedAPIKeyHash(reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), mediaTenantUUID), &reqctx.CoreXClaims{TenantUUID: mediaTenantUUID, MemberID: 1, Platforms: []string{"api_key"}}), key.KeyHash)
	invoker := &ImageDerivativeInvoker{Service: f.s}
	in := cap.CoreCapabilityInvokeInput{CapabilityID: ImageDerivativesReadCapability, TenantUUID: mediaTenantUUID, Method: "INVOKE", Endpoint: ImageDerivativesEndpoint, Body: map[string]interface{}{"operation": "status", "asset_uuid": a.UUID}}
	out, err := invoker.InvokeCoreCapability(ctx, in)
	require.NoError(t, err)
	require.Equal(t, "ready", out["result"].(*ImageDerivatives).Status)
	// 实际 grant-status 能读取，但仅 read grant 不能提交生成。
	access, err := cap.NewGrantStatusService(db).CurrentAccess(ctx)
	require.NoError(t, err)
	require.True(t, access.Granted[ImageDerivativesReadCapability])
	require.False(t, access.Granted[ImageDerivativesEnsureCapability])
	ensure := in
	ensure.CapabilityID = ImageDerivativesEnsureCapability
	ensure.Body = map[string]interface{}{"operation": "ensure", "asset_uuid": a.UUID}
	_, err = invoker.InvokeCoreCapability(ctx, ensure)
	require.Error(t, err)
	ensureGrant := gm.IntegrationGatewayAPIKeyPermission{APIKeyUUID: key.UUID, Scope: "_scope.media.image_derivatives.ensure", Action: "ensure", ResourceType: "capability", ResourcePattern: "media_image_derivatives", Effect: "allow"}
	require.NoError(t, db.Create(&ensureGrant).Error)
	_, err = invoker.InvokeCoreCapability(ctx, ensure)
	require.NoError(t, err)
	for _, field := range []string{"headers", "endpoint", "method", "tenant_uuid", "payload"} {
		bad := in
		bad.Body = map[string]interface{}{"operation": "status", "asset_uuid": a.UUID, field: "forbidden"}
		_, err = invoker.InvokeCoreCapability(ctx, bad)
		require.Error(t, err)
	}
	bad := in
	bad.Payload = map[string]interface{}{"endpoint": "/admin/media/assets"}
	_, err = invoker.InvokeCoreCapability(ctx, bad)
	require.Error(t, err)
	bad = in
	bad.TenantUUID = uuid.NewString()
	_, err = invoker.InvokeCoreCapability(ctx, bad)
	require.Error(t, err)
	_, err = invoker.InvokeCoreCapability(context.Background(), in)
	require.Error(t, err)
	// Registration 撤销及 grant 撤销都实时生效，不复用先前允许的结果。
	require.NoError(t, db.Model(&cm.CapabilityRegistration{}).Where("tenant_uuid=? AND capability_id=?", mediaTenantUUID, ImageDerivativesReadCapability).Update("status", "disabled").Error)
	_, err = invoker.InvokeCoreCapability(ctx, in)
	require.Error(t, err)
	require.NoError(t, db.Model(&cm.CapabilityRegistration{}).Where("tenant_uuid=? AND capability_id=?", mediaTenantUUID, ImageDerivativesReadCapability).Update("status", "published").Error)
	require.NoError(t, db.Delete(&grant).Error)
	_, err = invoker.InvokeCoreCapability(ctx, in)
	require.Error(t, err)
}
