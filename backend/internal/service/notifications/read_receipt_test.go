package notifications

import (
	"context"
	"encoding/json"
	cap "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	host "github.com/ArtisanCloud/PowerX/internal/testutil/runtime_host"
	cm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	im "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	nm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/notification"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"strings"
	"testing"
	"time"
)

func TestReadReceiptsAreOwnedMemberScopedAtomicAndIdempotent(t *testing.T) {
	t.Setenv("POWERX_RUNTIME_HOST_TEST_POSTGRES_DSN", "")
	db := host.Database(t)
	stmt := &gorm.Statement{DB: db}
	require.NoError(t, stmt.Parse(&nm.Notification{}))
	for _, f := range stmt.Schema.Fields {
		f.DefaultValue = strings.TrimSuffix(f.DefaultValue, "::jsonb")
	}
	require.NoError(t, db.AutoMigrate(&nm.Notification{}, &im.User{}, &im.Member{}))
	tenant := uuid.NewString()
	ctx, cred := host.Actor(t, db, tenant, "plugin.receipt.test")
	otherCtx, otherCred := host.Actor(t, db, tenant, "plugin.receipt.other")
	for _, id := range []string{CreateCapabilityID, ReadReceiptCapabilityID} {
		require.NoError(t, db.Create(&cm.CapabilityRecord{CapabilityID: id, PluginID: "core", PluginVersion: "1", Status: "published"}).Error)
		require.NoError(t, db.Create(&cm.CapabilityRegistration{CapabilityID: id, TenantUUID: tenant, Status: "published", Version: 1, RoutingPolicyID: uuid.New()}).Error)
	}
	host.Grant(t, db, cred, CreateCapabilityID, ReadReceiptCapabilityID)
	host.Grant(t, db, otherCred, CreateCapabilityID, ReadReceiptCapabilityID)
	members := []string{}
	for i := 0; i < 2; i++ {
		user := im.User{Email: uuid.NewString() + "@fixture.test", Status: 1}
		require.NoError(t, db.Create(&user).Error)
		m := im.Member{TenantUUID: tenant, UserID: user.ID, Username: uuid.NewString(), Status: 1}
		require.NoError(t, db.Create(&m).Error)
		members = append(members, m.UUID.String())
	}
	svc := NewService(db)
	a, err := svc.Create(ctx, CreateInput{TenantUUID: tenant, MemberUUID: members[0], Title: "fixture_title", Content: "fixture_content"})
	require.NoError(t, err)
	b, err := svc.Create(ctx, CreateInput{TenantUUID: tenant, MemberUUID: members[1], Title: "fixture_title", Content: "fixture_content"})
	require.NoError(t, err)
	foreign, err := svc.Create(otherCtx, CreateInput{TenantUUID: tenant, MemberUUID: members[0], Title: "fixture_title", Content: "fixture_content"})
	require.NoError(t, err)
	require.Equal(t, "plugin:plugin.receipt.test", a.ServiceOwner)
	_, err = svc.Acknowledge(ctx, ReadReceiptInput{MemberUUID: members[0], NotificationUUIDs: []string{a.UUID.String(), b.UUID.String()}})
	require.Error(t, err)
	_, err = svc.Acknowledge(ctx, ReadReceiptInput{MemberUUID: members[0], NotificationUUIDs: []string{foreign.UUID.String()}})
	require.Error(t, err)
	_, err = svc.Acknowledge(otherCtx, ReadReceiptInput{MemberUUID: members[0], NotificationUUIDs: []string{a.UUID.String()}})
	require.Error(t, err)
	require.NoError(t, db.First(a, "uuid = ?", a.UUID).Error)
	require.False(t, a.IsRead)
	input := ReadReceiptInput{MemberUUID: members[0], NotificationUUIDs: []string{a.UUID.String()}}
	var event any
	invoker := &CapabilityInvoker{Service: svc, Publish: func(_ context.Context, _ string, payload any) { event = payload }}
	raw, _ := json.Marshal(input)
	body := map[string]interface{}{}
	require.NoError(t, json.Unmarshal(raw, &body))
	body["operation"] = "acknowledge_read"
	_, err = invoker.InvokeCoreCapability(ctx, cap.CoreCapabilityInvokeInput{CapabilityID: ReadReceiptCapabilityID, TenantUUID: tenant, Method: "INVOKE", Endpoint: CapabilityEndpoint, Body: body})
	require.NoError(t, err)
	require.Equal(t, "notification_read", event.(map[string]any)["eventType"])
	require.NoError(t, db.First(a, "uuid = ?", a.UUID).Error)
	require.True(t, a.IsRead)
	first := *a.ReadAt
	svc.now = func() time.Time { return first.Add(time.Hour) }
	receipt, err := svc.Acknowledge(ctx, input)
	require.NoError(t, err)
	require.Equal(t, ReadReceiptContract, receipt.Contract)
	require.NoError(t, db.First(a, "uuid = ?", a.UUID).Error)
	require.Equal(t, first, *a.ReadAt)
	require.NoError(t, db.First(b, "uuid = ?", b.UUID).Error)
	require.False(t, b.IsRead)
	require.NoError(t, db.First(foreign, "uuid = ?", foreign.UUID).Error)
	require.False(t, foreign.IsRead)
	host.Grant(t, db, cred, CreateCapabilityID)
	_, err = svc.Acknowledge(ctx, input)
	require.Error(t, err)
	host.Grant(t, db, cred, CreateCapabilityID, ReadReceiptCapabilityID)
	require.NoError(t, db.Delete(a).Error)
	_, err = svc.Acknowledge(ctx, input)
	require.NoError(t, err)
}
