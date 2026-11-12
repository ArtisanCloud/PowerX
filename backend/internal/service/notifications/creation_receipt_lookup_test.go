package notifications

import (
	"context"
	"encoding/json"
	"testing"

	cap "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	host "github.com/ArtisanCloud/PowerX/internal/testutil/runtime_host"
	cm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	im "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	nm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/notification"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

func lookupRegistration(t *testing.T, s *Service, tenant string) {
	t.Helper()
	require.NoError(t, s.db.FirstOrCreate(&cm.CapabilityRecord{CapabilityID: CreationReceiptReadCapabilityID, PluginID: "core", PluginVersion: "1", Status: "published"}, "capability_id = ?", CreationReceiptReadCapabilityID).Error)
	require.NoError(t, s.db.Create(&cm.CapabilityRegistration{CapabilityID: CreationReceiptReadCapabilityID, TenantUUID: tenant, Status: "published", ContractRef: "v1", Version: 1, RoutingPolicyID: uuid.New()}).Error)
}

func TestCreationLookupPreservesOriginalProofAfterReadMemberDisableAndDeletionWithoutWrites(t *testing.T) {
	s, ctx, cred, input := creationFixture(t)
	original, err := s.CreateIdempotent(ctx, input)
	require.NoError(t, err)
	lookupRegistration(t, s, reqctx.GetTenantUUID(ctx))
	host.Grant(t, s.db, cred, CreateCapabilityID, CreationReceiptReadCapabilityID)
	id := original.Notification["id"].(string)
	require.NoError(t, s.MarkRead(ctx, reqctx.GetTenantUUID(ctx), input.MemberUUID, id))
	require.NoError(t, s.db.Model(&im.Member{}).Where("uuid = ?", input.MemberUUID).Update("status", 0).Error)
	var before nm.CreationReceipt
	require.NoError(t, s.db.First(&before).Error)
	for _, state := range []string{"present", "deleted", "receipt_only"} {
		if state == "deleted" {
			require.NoError(t, s.db.Where("uuid = ?", id).Delete(&nm.Notification{}).Error)
		}
		if state == "receipt_only" {
			require.NoError(t, s.db.Unscoped().Where("uuid = ?", id).Delete(&nm.Notification{}).Error)
		}
		result, err := s.LookupCreationReceipt(ctx, CreationReceiptLookupInput{input.IdempotencyKey, input.MemberUUID})
		require.NoError(t, err)
		require.True(t, result.Found)
		require.Equal(t, state, result.NotificationState)
		require.Equal(t, id, result.Notification["id"])
		require.Equal(t, false, result.Notification["isRead"])
		raw, _ := json.Marshal(result.Notification)
		require.Contains(t, string(raw), "9999999999999999.99")
		var after nm.CreationReceipt
		require.NoError(t, s.db.First(&after).Error)
		require.JSONEq(t, string(before.ResponseJSON), string(after.ResponseJSON))
	}
	var count int64
	require.NoError(t, s.db.Unscoped().Model(&nm.Notification{}).Count(&count).Error)
	require.Zero(t, count)
	missing, err := s.LookupCreationReceipt(ctx, CreationReceiptLookupInput{"fixture.missing", input.MemberUUID})
	require.NoError(t, err)
	require.False(t, missing.Found)
	require.Nil(t, missing.Notification)
}

func TestCreationLookupRequiresIndependentLiveGrantAndExactOriginalMemberOwnerTenant(t *testing.T) {
	s, ctx, cred, input := creationFixture(t)
	_, err := s.CreateIdempotent(ctx, input)
	require.NoError(t, err)
	lookupRegistration(t, s, reqctx.GetTenantUUID(ctx))
	lookup := CreationReceiptLookupInput{input.IdempotencyKey, input.MemberUUID}
	_, err = s.LookupCreationReceipt(ctx, lookup)
	require.Error(t, err) // create grant is not a read grant
	host.Grant(t, s.db, cred, CreationReceiptReadCapabilityID)
	_, err = s.LookupCreationReceipt(ctx, CreationReceiptLookupInput{input.IdempotencyKey, uuid.NewString()})
	var coded *dto.AppError
	require.ErrorAs(t, err, &coded)
	require.Equal(t, "NOTIFICATION_CREATION_LOOKUP_FORBIDDEN", coded.Code)
	_, err = s.LookupCreationReceipt(context.Background(), lookup)
	require.Error(t, err)
	other, otherCred := host.Actor(t, s.db, reqctx.GetTenantUUID(ctx), "third.party.notification.reader")
	host.Grant(t, s.db, otherCred, CreationReceiptReadCapabilityID)
	result, err := s.LookupCreationReceipt(other, lookup)
	require.NoError(t, err)
	require.False(t, result.Found)
	foreign, foreignCred := host.Actor(t, s.db, uuid.NewString(), cred.PluginID)
	lookupRegistration(t, s, reqctx.GetTenantUUID(foreign))
	host.Grant(t, s.db, foreignCred, CreationReceiptReadCapabilityID)
	result, err = s.LookupCreationReceipt(foreign, lookup)
	require.NoError(t, err)
	require.False(t, result.Found)
	host.Grant(t, s.db, cred)
	_, err = s.LookupCreationReceipt(ctx, lookup)
	require.Error(t, err)
}

func TestCreationLookupMalformedStoredProofAndInvalidSelectorsNeverBecomeNotFound(t *testing.T) {
	s, ctx, cred, input := creationFixture(t)
	_, err := s.CreateIdempotent(ctx, input)
	require.NoError(t, err)
	lookupRegistration(t, s, reqctx.GetTenantUUID(ctx))
	host.Grant(t, s.db, cred, CreationReceiptReadCapabilityID)
	for _, key := range []string{"", "contains space", "contains\nnewline"} {
		_, err := s.LookupCreationReceipt(ctx, CreationReceiptLookupInput{key, input.MemberUUID})
		require.Error(t, err)
	}
	require.NoError(t, s.db.Model(&nm.CreationReceipt{}).Where("tenant_uuid = ?", reqctx.GetTenantUUID(ctx)).Update("response_json", datatypes.JSON(`{}`)).Error)
	_, err = s.LookupCreationReceipt(ctx, CreationReceiptLookupInput{input.IdempotencyKey, input.MemberUUID})
	require.Error(t, err)
	var row nm.CreationReceipt
	require.NoError(t, s.db.First(&row).Error)
	missingMember, err := json.Marshal(CreateReceipt{Contract: CreateReceiptContract, IdempotencyKey: input.IdempotencyKey, Notification: map[string]any{"id": row.NotificationUUID.String()}})
	require.NoError(t, err)
	require.NoError(t, s.db.Model(&nm.CreationReceipt{}).Where("tenant_uuid = ?", reqctx.GetTenantUUID(ctx)).Update("response_json", datatypes.JSON(missingMember)).Error)
	_, err = s.LookupCreationReceipt(ctx, CreationReceiptLookupInput{input.IdempotencyKey, input.MemberUUID})
	var coded *dto.AppError
	require.ErrorAs(t, err, &coded)
	require.Equal(t, "NOTIFICATION_CREATE_RECEIPT_INVALID", coded.Code)
}

func TestCreationLookupFixedCapabilityNeedsNoPublisherAndRejectsRawGatewaySelectors(t *testing.T) {
	s, ctx, cred, input := creationFixture(t)
	_, err := s.CreateIdempotent(ctx, input)
	require.NoError(t, err)
	lookupRegistration(t, s, reqctx.GetTenantUUID(ctx))
	host.Grant(t, s.db, cred, CreationReceiptReadCapabilityID)
	invoker := &CapabilityInvoker{Service: s} // no publisher: lookup must not need or emit an event
	body := map[string]any{"operation": "lookup_creation_receipt", "idempotency_key": input.IdempotencyKey, "member_uuid": input.MemberUUID}
	in := cap.CoreCapabilityInvokeInput{CapabilityID: CreationReceiptReadCapabilityID, TenantUUID: reqctx.GetTenantUUID(ctx), Method: "INVOKE", Endpoint: CapabilityEndpoint, Body: body}
	_, err = invoker.InvokeCoreCapability(ctx, in)
	require.NoError(t, err)
	for _, field := range []string{"tenant_uuid", "plugin_id", "headers", "endpoint", "method", "notification_uuid"} {
		body[field] = "forbidden"
		_, err := invoker.InvokeCoreCapability(ctx, in)
		require.Error(t, err)
		delete(body, field)
	}
	in.Method = "POST"
	_, err = invoker.InvokeCoreCapability(ctx, in)
	require.Error(t, err)
	var count int64
	require.NoError(t, s.db.Model(&nm.CreationReceipt{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
}
