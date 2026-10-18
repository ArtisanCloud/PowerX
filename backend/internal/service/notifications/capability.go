package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	cap "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
)

const CreateCapabilityID = "com.corex.notifications.create"
const CapabilityEndpoint = "core://notifications"

// AuthorizeCreate enforces the actual service grant; authentication alone is
// insufficient. Internal system workers continue to use Create directly.
func (s *Service) AuthorizeCreate(ctx context.Context) error {
	if !cap.ServiceCredential(ctx) {
		return dto.NewErrorWithCode(http.StatusUnauthorized, "NOTIFICATION_SERVICE_REQUIRED", "service credential required", nil)
	}
	if s == nil || s.db == nil {
		return dto.NewErrorWithCode(http.StatusServiceUnavailable, "NOTIFICATION_UNAVAILABLE", "notification service unavailable", nil)
	}
	access, err := cap.NewGrantStatusService(s.db).CurrentAccess(ctx)
	if err == nil {
		err = access.Require(CreateCapabilityID)
	}
	if err != nil {
		status := http.StatusServiceUnavailable
		var grant *cap.DirectGrantError
		if errors.As(err, &grant) {
			status = grant.Status
		}
		return dto.NewErrorWithCode(status, "NOTIFICATION_SERVICE_FORBIDDEN", "notification service access denied", nil)
	}
	return nil
}

// CreateOperation is a fixed typed binding for Framework Host/service callers.
type CreateOperation struct {
	Navigation  *Navigation    `json:"navigation,omitempty"`
	Operation   string         `json:"operation"`
	Title       string         `json:"title"`
	Content     string         `json:"content"`
	Type        string         `json:"type,omitempty"`
	Category    string         `json:"category,omitempty"`
	IsImportant bool           `json:"is_important,omitempty"`
	MemberUUID  string         `json:"member_uuid,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type CapabilityInvoker struct {
	Service *Service
	Publish func(context.Context, string, any)
}

func (i *CapabilityInvoker) InvokeCoreCapability(ctx context.Context, in cap.CoreCapabilityInvokeInput) (map[string]interface{}, error) {
	if in.CapabilityID == ReadReceiptCapabilityID {
		return i.invokeReadReceipt(ctx, in)
	}
	if in.CapabilityID != CreateCapabilityID {
		return nil, cap.ErrCoreCapabilityNotHandled
	}
	bad := func() (map[string]interface{}, error) {
		return nil, dto.NewErrorWithCode(http.StatusBadRequest, "NOTIFICATION_INVALID_ARGUMENT", "typed create notification required", nil)
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
	var op CreateOperation
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&op) != nil || decoder.Decode(&struct{}{}) != io.EOF || op.Operation != "create" || strings.TrimSpace(op.Title) == "" || strings.TrimSpace(op.Content) == "" {
		return bad()
	}
	if i == nil || i.Service == nil || i.Publish == nil {
		return nil, dto.NewErrorWithCode(http.StatusServiceUnavailable, "NOTIFICATION_UNAVAILABLE", "notification delivery unavailable", nil)
	}
	if err = i.Service.AuthorizeCreate(ctx); err != nil {
		return nil, err
	}
	metadata, err := json.Marshal(op.Metadata)
	if err != nil {
		return bad()
	}
	item, err := i.Service.Create(ctx, CreateInput{TenantUUID: in.TenantUUID, MemberUUID: op.MemberUUID, Title: op.Title, Content: op.Content, Type: op.Type, Category: op.Category, IsImportant: op.IsImportant, Metadata: metadata, Navigation: op.Navigation})
	if err != nil {
		return nil, err
	}
	payload := map[string]interface{}{"id": item.UUID.String(), "title": item.Title, "content": item.Content, "type": item.Type, "category": item.Category, "isRead": item.IsRead, "isImportant": item.IsImportant, "createdAt": item.CreatedAt, "updatedAt": item.UpdatedAt, "userId": item.MemberUUID, "metadata": op.Metadata}
	if len(item.Actions) > 0 {
		var actions []Action
		if err := json.Unmarshal(item.Actions, &actions); err != nil {
			return nil, err
		}
		payload["actions"] = actions
	}
	i.Publish(ctx, in.TenantUUID, payload)
	return map[string]interface{}{"result": payload}, nil
}

func (i *CapabilityInvoker) invokeReadReceipt(ctx context.Context, in cap.CoreCapabilityInvokeInput) (map[string]interface{}, error) {
	bad := func() (map[string]interface{}, error) {
		return nil, receiptError(400, "NOTIFICATION_READ_RECEIPT_INVALID")
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
		ReadReceiptInput
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&op) != nil || decoder.Decode(&struct{}{}) != io.EOF || op.Operation != "acknowledge_read" {
		return bad()
	}
	if i == nil || i.Service == nil || i.Publish == nil {
		return nil, receiptError(503, "NOTIFICATION_UNAVAILABLE")
	}
	result, err := i.Service.Acknowledge(ctx, op.ReadReceiptInput)
	if err != nil {
		return nil, err
	}
	i.Publish(ctx, in.TenantUUID, ReadEvent(in.TenantUUID, result.MemberUUID, result.NotificationUUIDs))
	return map[string]interface{}{"result": result}, nil
}
