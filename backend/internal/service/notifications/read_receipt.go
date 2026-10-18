package notifications

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"net/http"
	"strings"

	cap "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
)

const ReadReceiptCapabilityID = "com.corex.notifications.read_receipts"
const ReadReceiptContract = "notification_read_receipt.v1"

type ReadReceiptInput struct {
	MemberUUID        string   `json:"member_uuid"`
	NotificationUUIDs []string `json:"notification_uuids"`
}

type ReadReceipt struct {
	Contract          string   `json:"contract"`
	MemberUUID        string   `json:"member_uuid"`
	NotificationUUIDs []string `json:"notification_uuids"`
}

func receiptError(status int, code string) error {
	return dto.NewErrorWithCode(status, code, "notification."+strings.ToLower(code), nil)
}

func (s *Service) authorizedOwner(ctx context.Context, capability string) (string, error) {
	if !cap.ServiceCredential(ctx) {
		return "", receiptError(http.StatusUnauthorized, "NOTIFICATION_SERVICE_REQUIRED")
	}
	if s == nil || s.db == nil {
		return "", receiptError(503, "NOTIFICATION_UNAVAILABLE")
	}
	access, err := cap.NewGrantStatusService(s.db).CurrentAccess(ctx)
	if err == nil {
		err = access.Require(capability)
	}
	if err != nil {
		status := 503
		var grant *cap.DirectGrantError
		if errors.As(err, &grant) {
			status = grant.Status
		}
		return "", receiptError(status, "NOTIFICATION_SERVICE_FORBIDDEN")
	}
	claims := reqctx.GetClaims(ctx)
	if claims.Issuer == "powerx-sts" {
		return "plugin:" + claims.PluginID, nil
	}
	if !strings.HasPrefix(access.Subject, "api_key:") {
		return "", receiptError(403, "NOTIFICATION_SERVICE_FORBIDDEN")
	}
	return access.Subject, nil
}

func (s *Service) Acknowledge(ctx context.Context, input ReadReceiptInput) (*ReadReceipt, error) {
	owner, err := s.authorizedOwner(ctx, ReadReceiptCapabilityID)
	if err != nil {
		return nil, err
	}
	tenant := reqctx.GetTenantUUID(ctx)
	canonical := func(raw string) bool {
		id, err := uuid.Parse(raw)
		return err == nil && id != uuid.Nil && id.String() == raw
	}
	if !canonical(tenant) || !canonical(input.MemberUUID) || len(input.NotificationUUIDs) == 0 || len(input.NotificationUUIDs) > 100 {
		return nil, receiptError(400, "NOTIFICATION_READ_RECEIPT_INVALID")
	}
	seen := map[string]bool{}
	for _, id := range input.NotificationUUIDs {
		if !canonical(id) || seen[id] {
			return nil, receiptError(400, "NOTIFICATION_READ_RECEIPT_INVALID")
		}
		seen[id] = true
	}
	if err := NewRecipientAccess(s.db).Require(ctx, tenant, input.MemberUUID); err != nil {
		return nil, err
	}
	if err := s.repo.AcknowledgeOwned(ctx, tenant, owner, input.MemberUUID, input.NotificationUUIDs, s.now()); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, receiptError(403, "NOTIFICATION_READ_RECEIPT_FORBIDDEN")
		}
		return nil, receiptError(503, "NOTIFICATION_UNAVAILABLE")
	}
	return &ReadReceipt{Contract: ReadReceiptContract, MemberUUID: input.MemberUUID, NotificationUUIDs: input.NotificationUUIDs}, nil
}

// ReadEvent shares the existing tenant/member guarded notification topic. It is
// an invalidation/receipt, never a second created notification.
func ReadEvent(tenant, member string, ids []string) map[string]any {
	return map[string]any{"eventType": "notification_read", "payload_version": 1, "contract": ReadReceiptContract, "tenant_uuid": tenant, "userId": member, "notification_uuids": ids}
}
