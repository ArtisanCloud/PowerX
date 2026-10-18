package notifications

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
	nm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/notification"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestNotificationServiceGrantsAndTypedCreate(t *testing.T) {
	t.Setenv("POWERX_RUNTIME_HOST_TEST_POSTGRES_DSN", "")
	db := hosttest.Database(t)
	stmt := &gorm.Statement{DB: db}
	require.NoError(t, stmt.Parse(&nm.Notification{}))
	for _, f := range stmt.Schema.Fields {
		f.DefaultValue = strings.TrimSuffix(f.DefaultValue, "::jsonb")
	}
	require.NoError(t, db.AutoMigrate(&nm.Notification{}, &im.User{}, &im.Member{}, &im.APIKeyProfile{}, &im.Permission{}, &im.APIKeyProfilePermission{}, &gm.IntegrationGatewayAPIKey{}, &gm.IntegrationGatewayAPIKeyPermission{}))
	tenant := uuid.NewString()
	ctx, credential := hosttest.Actor(t, db, tenant, "plugin.notification.test")
	u := im.User{Email: "notice@fixture.test", Status: 1}
	require.NoError(t, db.Create(&u).Error)
	member := im.Member{TenantUUID: tenant, UserID: u.ID, Username: "recipient", Status: 1}
	require.NoError(t, db.Create(&member).Error)
	require.NoError(t, db.Create(&cm.CapabilityRecord{CapabilityID: CreateCapabilityID, PluginID: "core", PluginVersion: "1", Status: "published"}).Error)
	reg := cm.CapabilityRegistration{CapabilityID: CreateCapabilityID, TenantUUID: tenant, ContractRef: "v1", Status: "published", Version: 1, RoutingPolicyID: uuid.New()}
	require.NoError(t, db.Create(&reg).Error)
	s := NewService(db)
	require.Error(t, s.AuthorizeCreate(ctx))
	require.Error(t, s.AuthorizeCreate(context.Background()))
	hosttest.Grant(t, db, credential, CreateCapabilityID)
	require.NoError(t, s.AuthorizeCreate(ctx))
	var published map[string]interface{}
	i := &CapabilityInvoker{Service: s, Publish: func(_ context.Context, got string, payload any) {
		require.Equal(t, tenant, got)
		published = payload.(map[string]interface{})
	}}
	in := cap.CoreCapabilityInvokeInput{CapabilityID: CreateCapabilityID, TenantUUID: tenant, Method: "INVOKE", Endpoint: CapabilityEndpoint, Body: map[string]interface{}{"operation": "create", "title": "reminder", "content": "generic content update", "member_uuid": member.UUID.String(), "navigation": map[string]any{"label": "fixture_action", "path": "/_p/plugin.test/admin/order?target=opaque"}}}
	_, err := i.InvokeCoreCapability(ctx, in)
	require.NoError(t, err)
	require.Equal(t, member.UUID.String(), published["userId"])
	actions, ok := published["actions"].([]Action)
	require.True(t, ok)
	require.Len(t, actions, 1)
	require.Equal(t, "/_p/plugin.test/admin/order?target=opaque", actions[0].Params["path"])
	for _, field := range []string{"endpoint", "headers", "method", "tenant_uuid", "payload"} {
		bad := in
		bad.Body = map[string]interface{}{"operation": "create", "title": "x", "content": "x", field: "forbidden"}
		_, err = i.InvokeCoreCapability(ctx, bad)
		require.Error(t, err)
	}
	require.NoError(t, db.Model(&reg).Update("status", "disabled").Error)
	require.Error(t, s.AuthorizeCreate(ctx))
	require.NoError(t, db.Model(&reg).Update("status", "published").Error)
	hosttest.Grant(t, db, credential)
	require.Error(t, s.AuthorizeCreate(ctx))
	// One explicit service grant covers the fixed typed and existing REST bindings.
	dir, err := filepath.Abs("../../../config/platform_capabilities")
	require.NoError(t, err)
	t.Setenv("PLATFORM_CAPABILITIES_DIR", dir)
	rows, err := permissions.BuildPlatformCapabilityPermissions()
	require.NoError(t, err)
	for _, row := range rows {
		var meta map[string]any
		require.NoError(t, json.Unmarshal(row.Meta, &meta))
		if meta["capability_id"] == CreateCapabilityID || meta["capability_id"] == ReadReceiptCapabilityID {
			require.NoError(t, db.Create(&row).Error)
		}
	}
	key := gm.IntegrationGatewayAPIKey{TenantUUID: tenant, ProfileID: 1, Name: "notice fixture", KeyPrefix: "test", KeyHash: "notification-test-hash", Status: "active"}
	require.NoError(t, db.Create(&key).Error)
	keyCtx := reqctx.WithAuthenticatedAPIKeyHash(reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), tenant), &reqctx.CoreXClaims{TenantUUID: tenant, Platforms: []string{"api_key"}}), key.KeyHash)
	require.Error(t, s.AuthorizeCreate(keyCtx))
	grant := gm.IntegrationGatewayAPIKeyPermission{APIKeyUUID: key.UUID, Scope: "_scope.notifications.create", Action: "create", ResourceType: "capability", ResourcePattern: "notifications", Effect: "allow"}
	require.NoError(t, db.Create(&grant).Error)
	require.NoError(t, s.AuthorizeCreate(keyCtx))
	require.NoError(t, db.Create(&cm.CapabilityRecord{CapabilityID: ReadReceiptCapabilityID, PluginID: "core", PluginVersion: "1", Status: "published"}).Error)
	require.NoError(t, db.Create(&cm.CapabilityRegistration{CapabilityID: ReadReceiptCapabilityID, TenantUUID: tenant, ContractRef: "v1", Status: "published", Version: 1, RoutingPolicyID: uuid.New()}).Error)
	notice, err := s.Create(keyCtx, CreateInput{TenantUUID: tenant, MemberUUID: member.UUID.String(), Title: "fixture_title", Content: "fixture_content"})
	require.NoError(t, err)
	input := ReadReceiptInput{MemberUUID: member.UUID.String(), NotificationUUIDs: []string{notice.UUID.String()}}
	_, err = s.Acknowledge(keyCtx, input)
	require.Error(t, err)
	readGrant := gm.IntegrationGatewayAPIKeyPermission{APIKeyUUID: key.UUID, Scope: "_scope.notifications.read_receipts", Action: "read_receipts", ResourceType: "capability", ResourcePattern: "notifications", Effect: "allow"}
	require.NoError(t, db.Create(&readGrant).Error)
	_, err = s.Acknowledge(keyCtx, input)
	require.NoError(t, err)
	require.NoError(t, db.Delete(&grant).Error)
	require.Error(t, s.AuthorizeCreate(keyCtx))
}
