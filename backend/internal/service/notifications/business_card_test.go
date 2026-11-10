package notifications

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	cap "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	host "github.com/ArtisanCloud/PowerX/internal/testutil/runtime_host"
	cm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	im "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	nm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/notification"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestBusinessCardsKeepOneOwnedMemberRecordAndRejectStaleReceipts(t *testing.T) {
	db := host.Database(t)
	stmt := &gorm.Statement{DB: db}
	require.NoError(t, stmt.Parse(&nm.Notification{}))
	for _, f := range stmt.Schema.Fields {
		f.DefaultValue = strings.TrimSuffix(f.DefaultValue, "::jsonb")
	}
	require.NoError(t, db.AutoMigrate(&nm.Notification{}, &im.User{}, &im.Member{}))
	tenant := uuid.NewString()
	ctx, cred := host.Actor(t, db, tenant, "plugin.card.test")
	other, otherCred := host.Actor(t, db, tenant, "plugin.card.other")
	capabilities := []string{CreateCapabilityID, ReadReceiptCapabilityID, InvalidateCardCapabilityID}
	for _, id := range capabilities {
		require.NoError(t, db.Create(&cm.CapabilityRecord{CapabilityID: id, PluginID: "core", PluginVersion: "1", Status: "published"}).Error)
		require.NoError(t, db.Create(&cm.CapabilityRegistration{CapabilityID: id, TenantUUID: tenant, Status: "published", Version: 1, RoutingPolicyID: uuid.New()}).Error)
	}
	host.Grant(t, db, cred, capabilities...)
	host.Grant(t, db, otherCred, capabilities...)
	user := im.User{Email: "card@example.test", Status: 1}
	require.NoError(t, db.Create(&user).Error)
	member := im.Member{TenantUUID: tenant, UserID: user.ID, Username: "card", Status: 1}
	require.NoError(t, db.Create(&member).Error)
	svc := NewService(db)
	enabled := true
	svc.LookupPresentationPlugin = func(_ context.Context, id string) (plugin_mgr.Plugin, error) {
		state := plugin_mgr.StateEnabled
		if !enabled {
			state = plugin_mgr.StateDisabled
		}
		return plugin_mgr.Plugin{ID: id, State: state, Endpoints: plugin_mgr.EndpointSpec{HTTPBasePath: "/api/v1"}, NotificationPresentations: []plugin_mgr.NotificationPresentationSpec{{BusinessType: "sample_track", Contract: BusinessCardContract, PresentationPath: "/admin/operations/notices/{delivery_key}/presentation", PreviewPath: "/admin/operations/notices/{delivery_key}/images/{image_key}"}, {BusinessType: "bulk_order", Contract: BusinessCardContract, PresentationPath: "/admin/operations/bulk/{delivery_key}/presentation", PreviewPath: "/admin/operations/bulk/{delivery_key}/images/{image_key}"}}}, nil
	}
	originalLookup := svc.LookupPresentationPlugin
	svc.LookupPresentationPlugin = func(ctx context.Context, id string) (plugin_mgr.Plugin, error) {
		p, err := originalLookup(ctx, id)
		for _, b := range append([]plugin_mgr.NotificationPresentationSpec{}, p.NotificationPresentations...) {
			b.Contract = BusinessCardContractV2
			b.PresentationPath += "-v2"
			p.NotificationPresentations = append(p.NotificationPresentations, b)
		}
		return p, err
	}
	metadata := func(kind string) []byte {
		raw, _ := json.Marshal(map[string]any{"business_card": BusinessCardReference{Contract: BusinessCardContract, SourcePlugin: "plugin.card.test", BusinessType: kind, DeliveryKey: "opaque-key", SourceRevision: 1}})
		return raw
	}
	var published []any
	invoker := &CapabilityInvoker{Service: svc, Publish: func(_ context.Context, _ string, payload any) { published = append(published, payload) }}
	for _, kind := range []string{"sample_track", "bulk_order"} {
		item, err := svc.Create(ctx, CreateInput{TenantUUID: tenant, MemberUUID: member.UUID.String(), Title: "generic", Content: "generic", Metadata: metadata(kind)})
		require.NoError(t, err)
		require.EqualValues(t, 1, item.Revision)
		require.EqualValues(t, 1, item.SourceRevision)
		event := CreatedEvent(item)
		require.Equal(t, "opaque-key", event["metadata"].(map[string]any)["business_card"].(map[string]any)["delivery_key"])
		source, err := svc.PresentationSource(ctx, tenant, member.UUID.String(), item.UUID.String())
		require.NoError(t, err)
		require.Contains(t, source.PresentationURL, "/_p/plugin.card.test/api/v1/admin/")
		require.NotContains(t, source.PresentationURL, "token")
		_, err = svc.PresentationSource(ctx, uuid.NewString(), member.UUID.String(), item.UUID.String())
		require.Error(t, err)
		_, err = svc.PresentationSource(ctx, tenant, uuid.NewString(), item.UUID.String())
		require.Error(t, err)
		enabled = false
		_, err = svc.PresentationSource(ctx, tenant, member.UUID.String(), item.UUID.String())
		require.Error(t, err)
		enabled = true
		require.Error(t, svc.MarkRead(ctx, tenant, member.UUID.String(), item.UUID.String()))
		require.NoError(t, svc.MarkReadVersion(ctx, tenant, member.UUID.String(), item.UUID.String(), 1))
		in := InvalidateCardInput{MemberUUID: member.UUID.String(), NotificationUUID: item.UUID.String(), SourceRevision: 2}
		_, _, err = svc.InvalidateCard(other, in)
		require.Error(t, err)
		wrongTenant := reqctx.WithTenantUUID(ctx, uuid.NewString())
		_, _, err = svc.InvalidateCard(wrongTenant, in)
		require.Error(t, err)
		host.Grant(t, db, cred, CreateCapabilityID, ReadReceiptCapabilityID)
		_, _, err = svc.InvalidateCard(ctx, in)
		require.Error(t, err)
		host.Grant(t, db, cred, capabilities...)
		invoke := cap.CoreCapabilityInvokeInput{CapabilityID: InvalidateCardCapabilityID, TenantUUID: tenant, Method: "INVOKE", Endpoint: CapabilityEndpoint, Body: map[string]interface{}{"operation": "invalidate_card", "member_uuid": in.MemberUUID, "notification_uuid": in.NotificationUUID, "source_revision": 2}}
		before := len(published)
		_, err = invoker.InvokeCoreCapability(ctx, invoke)
		require.NoError(t, err)
		require.Len(t, published, before+1)
		update := published[len(published)-1].(map[string]any)
		require.EqualValues(t, 2, update["revision"])
		require.False(t, update["isRead"].(bool))
		require.Equal(t, "notification_updated", update["eventType"])
		for j := 0; j < 2; j++ {
			_, err = invoker.InvokeCoreCapability(ctx, invoke)
			require.NoError(t, err)
		}
		require.Len(t, published, before+1)
		in.SourceRevision = 1
		current, changed, err := svc.InvalidateCard(ctx, in)
		require.NoError(t, err)
		require.False(t, changed)
		require.EqualValues(t, 2, current.Revision)
		require.Error(t, svc.MarkReadVersion(ctx, tenant, member.UUID.String(), item.UUID.String(), 1))
		_, err = svc.Acknowledge(ctx, ReadReceiptInput{MemberUUID: member.UUID.String(), NotificationUUIDs: []string{item.UUID.String()}, Revisions: map[string]int64{item.UUID.String(): 1}})
		require.Error(t, err)
		receipt, err := svc.Acknowledge(ctx, ReadReceiptInput{MemberUUID: member.UUID.String(), NotificationUUIDs: []string{item.UUID.String()}, Revisions: map[string]int64{item.UUID.String(): 2}})
		require.NoError(t, err)
		require.EqualValues(t, 2, receipt.Revisions[item.UUID.String()])
		in.SourceRevision = 3
		in.Reason = "access_changed"
		accessUpdated, changed, err := svc.InvalidateCard(ctx, in)
		require.NoError(t, err)
		require.True(t, changed)
		require.True(t, accessUpdated.IsRead)
		require.EqualValues(t, 3, accessUpdated.Revision)
		// Refresh and a fresh Core service both read the same durable record.
		recovered, err := NewService(db).Get(ctx, tenant, member.UUID.String(), item.UUID.String())
		require.NoError(t, err)
		require.EqualValues(t, 3, recovered.Revision)
		require.True(t, recovered.IsRead)
		invoke.Body["endpoint"] = "https://bad.example"
		_, err = invoker.InvokeCoreCapability(ctx, invoke)
		require.Error(t, err)
	}
	_, total, err := svc.List(ctx, ListInput{TenantUUID: tenant, MemberUUID: member.UUID.String()})
	require.NoError(t, err)
	require.EqualValues(t, 2, total)
	// A v1 reference can be upgraded in-place only by its owner, with a new
	// source watermark and a presentation-only reason. No business read is reset.
	legacyCard, err := svc.Create(ctx, CreateInput{TenantUUID: tenant, MemberUUID: member.UUID.String(), Title: "upgrade", Content: "x", Metadata: metadata("sample_track")})
	require.NoError(t, err)
	require.NoError(t, svc.MarkReadVersion(ctx, tenant, member.UUID.String(), legacyCard.UUID.String(), 1))
	ref2 := &BusinessCardReference{Contract: BusinessCardContractV2, SourcePlugin: "plugin.card.test", BusinessType: "sample_track", DeliveryKey: "opaque-key", SourceRevision: 2}
	upgrade2 := InvalidateCardInput{MemberUUID: member.UUID.String(), NotificationUUID: legacyCard.UUID.String(), SourceRevision: 2, Reason: "presentation_changed", CardReference: ref2}
	upgraded, changed, err := svc.InvalidateCard(ctx, upgrade2)
	require.NoError(t, err)
	require.True(t, changed)
	require.True(t, upgraded.IsRead)
	require.Equal(t, legacyCard.UUID, upgraded.UUID)
	src, err := svc.PresentationSource(ctx, tenant, member.UUID.String(), upgraded.UUID.String())
	require.NoError(t, err)
	require.Equal(t, BusinessCardContractV2, src.Contract)
	require.Contains(t, src.PresentationURL, "presentation-v2")
	_, changed, err = svc.InvalidateCard(ctx, upgrade2)
	require.NoError(t, err)
	require.False(t, changed)
	resource := InvalidateCardInput{MemberUUID: member.UUID.String(), NotificationUUID: upgraded.UUID.String(), SourceRevision: 3, Reason: "presentation_changed"}
	refreshed, changed, err := svc.InvalidateCard(ctx, resource)
	require.NoError(t, err)
	require.True(t, changed)
	require.True(t, refreshed.IsRead)
	ref2.Contract = BusinessCardContract
	ref2.SourceRevision = 4
	upgrade2.SourceRevision = 4
	_, _, err = svc.InvalidateCard(ctx, upgrade2)
	require.Error(t, err)

	for _, raw := range []string{`{"business_card":null}`, `{"business_card":{"contract":"unknown"}}`, `{"business_card":{"contract":"business_notification_card.v1","source_plugin":"plugin.card.test","business_type":"sample_track","delivery_key":"../escape","source_revision":1}}`} {
		_, err = svc.Create(ctx, CreateInput{TenantUUID: tenant, MemberUUID: member.UUID.String(), Title: "generic", Metadata: []byte(raw)})
		require.Error(t, err)
	}
	// Upgrade an existing owned delivery in place; replay is harmless and binding is immutable.
	legacy, err := svc.Create(ctx, CreateInput{TenantUUID: tenant, MemberUUID: member.UUID.String(), Title: "legacy", Content: "generic"})
	require.NoError(t, err)
	_, err = svc.Acknowledge(ctx, ReadReceiptInput{MemberUUID: member.UUID.String(), NotificationUUIDs: []string{legacy.UUID.String()}, Revisions: map[string]int64{legacy.UUID.String(): 100}})
	require.Error(t, err)
	_, err = svc.Acknowledge(ctx, ReadReceiptInput{MemberUUID: member.UUID.String(), NotificationUUIDs: []string{legacy.UUID.String()}})
	require.NoError(t, err)
	ref := &BusinessCardReference{Contract: BusinessCardContract, SourcePlugin: "plugin.card.test", BusinessType: "sample_track", DeliveryKey: "old-delivery", SourceRevision: 1}
	upgrade := InvalidateCardInput{MemberUUID: member.UUID.String(), NotificationUUID: legacy.UUID.String(), SourceRevision: 1, Reason: "access_changed", CardReference: ref}
	_, _, err = svc.InvalidateCard(other, upgrade)
	require.Error(t, err)
	attached, changed, err := svc.InvalidateCard(ctx, upgrade)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, legacy.UUID, attached.UUID)
	require.True(t, attached.IsRead)
	require.EqualValues(t, 2, attached.Revision)
	_, changed, err = svc.InvalidateCard(ctx, upgrade)
	require.NoError(t, err)
	require.False(t, changed)
	badRef := *ref
	badRef.DeliveryKey = "different"
	upgrade.CardReference = &badRef
	_, _, err = svc.InvalidateCard(ctx, upgrade)
	require.Error(t, err)
	// Permission/revocation checks remain live, including inactive recipients.
	require.NoError(t, db.Model(&member).Update("status", 0).Error)
	_, _, err = svc.InvalidateCard(ctx, InvalidateCardInput{MemberUUID: member.UUID.String(), NotificationUUID: uuid.NewString(), SourceRevision: 3})
	require.Error(t, err)
}
