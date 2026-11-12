package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"time"

	cap "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	nm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/notification"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"gorm.io/gorm"
)

const CreationReceiptReadCapabilityID = "com.corex.notifications.creation_receipts.read"
const CreationReceiptLookupContract = "notification_create_receipt.v1"

// CreationReceiptLookupInput 只指定原键及预期成员，租户与服务所有者来自认证身份。
type CreationReceiptLookupInput struct {
	IdempotencyKey string `json:"idempotency_key"`
	MemberUUID     string `json:"member_uuid"`
}

// CreationReceiptLookup 返回原创建凭据；notification 是冻结创建结果，不是当前已读状态。
type CreationReceiptLookup struct {
	Contract          string         `json:"contract"`
	TenantUUID        string         `json:"tenant_uuid"`
	ServiceOwner      string         `json:"service_owner"`
	IdempotencyKey    string         `json:"idempotency_key"`
	MemberUUID        string         `json:"member_uuid"`
	Found             bool           `json:"found"`
	NotificationState string         `json:"notification_state"`
	CheckedAt         time.Time      `json:"checked_at"`
	Notification      map[string]any `json:"notification,omitempty"`
}

// LookupCreationReceipt 只读持久凭据，不要求历史接收人仍可收信，不创建/复活/已读/发布通知。
func (s *Service) LookupCreationReceipt(ctx context.Context, input CreationReceiptLookupInput) (*CreationReceiptLookup, error) {
	owner, err := s.authorizedOwner(ctx, CreationReceiptReadCapabilityID)
	if err != nil {
		return nil, err
	}
	tenant := reqctx.GetTenantUUID(ctx)
	if !canonicalUUID(tenant) || !canonicalUUID(input.MemberUUID) || len(input.IdempotencyKey) < 1 || len(input.IdempotencyKey) > 256 {
		return nil, receiptError(400, "NOTIFICATION_CREATION_LOOKUP_INVALID")
	}
	for _, r := range input.IdempotencyKey {
		if r < 33 || r > 126 {
			return nil, receiptError(400, "NOTIFICATION_CREATION_LOOKUP_INVALID")
		}
	}
	result := &CreationReceiptLookup{Contract: CreationReceiptLookupContract, TenantUUID: tenant, ServiceOwner: owner, IdempotencyKey: input.IdempotencyKey, MemberUUID: input.MemberUUID, NotificationState: "not_found", CheckedAt: s.now().UTC()}
	var row nm.CreationReceipt
	err = s.db.WithContext(ctx).Where("tenant_uuid = ? AND service_owner = ? AND key_hash = ?", tenant, owner, notificationHash([]byte(input.IdempotencyKey))).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return result, nil
	}
	if err != nil {
		return nil, receiptError(503, "NOTIFICATION_UNAVAILABLE")
	}
	var original CreateReceipt
	decoder := json.NewDecoder(bytes.NewReader(row.ResponseJSON))
	decoder.UseNumber()
	if decoder.Decode(&original) != nil || decoder.Decode(&struct{}{}) != io.EOF || original.Contract != CreateReceiptContract || original.IdempotencyKey != input.IdempotencyKey || !canonicalUUID(row.NotificationUUID.String()) || original.Notification["id"] != row.NotificationUUID.String() {
		return nil, receiptError(503, "NOTIFICATION_CREATE_RECEIPT_INVALID")
	}
	originalMember, validMember := original.Notification["userId"].(string)
	if !validMember || !canonicalUUID(originalMember) {
		return nil, receiptError(503, "NOTIFICATION_CREATE_RECEIPT_INVALID")
	}
	if originalMember != input.MemberUUID {
		return nil, receiptError(403, "NOTIFICATION_CREATION_LOOKUP_FORBIDDEN")
	}
	result.Found = true
	result.Notification = original.Notification
	result.NotificationState = "receipt_only"
	var current nm.Notification
	err = s.db.WithContext(ctx).Unscoped().Where("uuid = ?", row.NotificationUUID).Take(&current).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return result, nil
	}
	if err != nil {
		return nil, receiptError(503, "NOTIFICATION_UNAVAILABLE")
	}
	if current.TenantUUID != tenant || current.ServiceOwner != owner || current.MemberUUID != input.MemberUUID {
		return nil, receiptError(503, "NOTIFICATION_CREATE_RECEIPT_INVALID")
	}
	result.NotificationState = "present"
	if current.DeletedAt.Valid {
		result.NotificationState = "deleted"
	}
	return result, nil
}

func (i *CapabilityInvoker) invokeCreationReceiptLookup(ctx context.Context, in cap.CoreCapabilityInvokeInput) (map[string]interface{}, error) {
	bad := func() (map[string]interface{}, error) {
		return nil, receiptError(400, "NOTIFICATION_CREATION_LOOKUP_INVALID")
	}
	if in.Method != "INVOKE" || in.Endpoint != CapabilityEndpoint || len(in.Query) > 0 || len(in.Context) > 0 || in.TenantUUID != reqctx.GetTenantUUID(ctx) {
		return bad()
	}
	for key := range in.Payload {
		if key != "body" {
			return bad()
		}
	}
	raw, err := json.Marshal(in.Body)
	if err != nil {
		return bad()
	}
	var op struct {
		Operation string `json:"operation"`
		CreationReceiptLookupInput
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&op) != nil || decoder.Decode(&struct{}{}) != io.EOF || op.Operation != "lookup_creation_receipt" {
		return bad()
	}
	if i == nil || i.Service == nil {
		return nil, receiptError(503, "NOTIFICATION_UNAVAILABLE")
	}
	result, err := i.Service.LookupCreationReceipt(ctx, op.CreationReceiptLookupInput)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"result": result}, nil
}
