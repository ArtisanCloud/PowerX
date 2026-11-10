package taskqueue

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	r "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/event_fabric"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const runtimeReceiptKey = "runtime_host_receipt_uuid"
const runtimeDeliveryKey = "runtime_host_delivery"
const runtimeSubscriberKey = "runtime_host_subscriber"

type RuntimeService struct{ driver *StreamDriver }

func NewRuntimeService(driver *StreamDriver) *RuntimeService { return &RuntimeService{driver: driver} }
func runtimeError(status int, code string) error {
	return dto.NewErrorWithCode(status, code, code, nil)
}
func runtimeSubscriber(scope RuntimeScope, subscriber string) string {
	hash := sha256.Sum256([]byte(scope.EnvironmentID + "\x00" + scope.SourceService + "\x00" + subscriber))
	return "hostq2:" + hex.EncodeToString(hash[:])
}
func runtimeValidPart(value string, limit int) bool {
	return value != "" && value == strings.TrimSpace(value) && len(value) <= limit
}

func (s *RuntimeService) Apply(ctx context.Context, scope RuntimeScope, in RuntimeRequest) (map[string]any, error) {
	if s == nil || s.driver == nil || s.driver.base == nil || s.driver.repo == nil {
		return nil, runtimeError(503, "TASK_QUEUE_UNAVAILABLE")
	}
	scope.EnvironmentID = s.driver.base.EnvironmentNamespace()
	if scope.EnvironmentID == "" {
		return nil, runtimeError(503, "TASK_QUEUE_ENVIRONMENT_REQUIRED")
	}
	if in.ContractVersion != RuntimeContract || !runtimeValidPart(scope.SourceService, 256) || scope.TenantUUID == "" || in.TenantKey != scope.TenantUUID {
		return nil, runtimeError(400, "TASK_QUEUE_INVALID_ARGUMENT")
	}
	if err := validateRuntimeOperation(in); err != nil {
		return nil, err
	}
	var result map[string]any
	var err error
	switch in.Operation {
	case "enqueue":
		result, err = s.enqueue(ctx, scope, in)
	case "dequeue":
		result, err = s.dequeue(ctx, scope, in)
	case "ack", "nack", "renew":
		result, err = s.settle(ctx, scope, in)
	default:
		return nil, runtimeError(400, "TASK_QUEUE_INVALID_ARGUMENT")
	}
	if err != nil {
		if errors.Is(err, event_bus.ErrTaskDeliveryStateInvalid) {
			return nil, runtimeError(503, "TASK_QUEUE_DELIVERY_STATE_INVALID")
		}
		if errors.Is(err, r.ErrRuntimeQueueConflict) || errors.Is(err, event_bus.ErrTaskIdempotencyConflict) {
			return nil, runtimeError(409, "TASK_QUEUE_IDEMPOTENCY_CONFLICT")
		}
		if errors.Is(err, event_bus.ErrLeaseStale) || errors.Is(err, ErrTaskTerminal) {
			return nil, runtimeError(409, "TASK_QUEUE_RECEIPT_STALE")
		}
		if dto.CodeOf(err) != "" {
			return nil, err
		}
		return nil, runtimeError(503, "TASK_QUEUE_UNAVAILABLE")
	}
	result["contract_version"] = RuntimeContract
	return result, nil
}
func (s *RuntimeService) enqueue(ctx context.Context, scope RuntimeScope, in RuntimeRequest) (map[string]any, error) {
	v := in.Message
	if v == nil || v.TenantKey != scope.TenantUUID || !runtimeValidPart(v.ID, 128) || !runtimeValidPart(v.SubscriberID, 128) || !runtimeValidPart(v.IdempotencyKey, 255) || !runtimeValidPart(v.Topic, 255) || v.Attempt != 0 || v.Lease != nil {
		return nil, runtimeError(400, "TASK_QUEUE_INVALID_ARGUMENT")
	}
	payload, err := base64.StdEncoding.Strict().DecodeString(v.PayloadBase64)
	if err != nil || len(payload) > 1<<20 || base64.StdEncoding.EncodeToString(payload) != v.PayloadBase64 {
		return nil, runtimeError(400, "TASK_QUEUE_INVALID_ARGUMENT")
	}
	meta := map[string]string{}
	for key, value := range v.Metadata {
		if strings.HasPrefix(key, "runtime_host_") || strings.HasPrefix(key, "taskbus_") || key == "run_id" || len(key) > 128 || len(value) > 2048 {
			return nil, runtimeError(400, "TASK_QUEUE_INVALID_ARGUMENT")
		}
		meta[key] = value
	}
	meta[runtimeSubscriberKey] = v.SubscriberID
	msg := event_bus.TaskMessage{ID: v.ID, IdempotencyKey: v.IdempotencyKey, TenantKey: scope.TenantUUID, SubscriberID: runtimeSubscriber(scope, v.SubscriberID), Topic: v.Topic, Payload: payload, Headers: v.Headers, TraceID: v.TraceID, Metadata: meta}
	if v.VisibleAt != nil {
		msg.VisibleAt = v.VisibleAt.UTC()
	}
	raw, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	row := &m.TaskHistory{TaskID: msg.ID, TenantKey: msg.TenantKey, SubscriberID: msg.SubscriberID, IdempotencyKey: &v.IdempotencyKey, Topic: msg.Topic, Source: "runtime_host_v2", Status: m.TaskHistoryStatusPending, Payload: string(raw), Metadata: datatypes.JSON(`{}`), SubmittedAt: &now, LastSeenAt: now}
	if _, err = s.driver.repo.RegisterRuntime(ctx, row); err != nil {
		return nil, err
	}
	if err = s.driver.Enqueue(ctx, msg); err != nil {
		return nil, err
	}
	return map[string]any{"accepted": true, "message_id": v.ID, "idempotency_key": v.IdempotencyKey}, nil
}
func (s *RuntimeService) dequeue(ctx context.Context, scope RuntimeScope, in RuntimeRequest) (map[string]any, error) {
	if !runtimeValidPart(in.SubscriberID, 128) || !runtimeValidPart(in.WorkerID, 128) || in.MaxItems < 1 || in.MaxItems > 100 || in.WaitTimeoutMS < 1 || in.WaitTimeoutMS > 30000 {
		return nil, runtimeError(400, "TASK_QUEUE_INVALID_ARGUMENT")
	}
	items, err := s.driver.Dequeue(ctx, scope.TenantUUID, runtimeSubscriber(scope, in.SubscriberID), "runtime_host_v2", in.WorkerID, int64(in.MaxItems), time.Duration(in.WaitTimeoutMS)*time.Millisecond)
	if err != nil {
		return nil, err
	}
	out := []RuntimeMessage{}
	for _, item := range items {
		receipt := uuid.NewString()
		deliveryAttempt := 0
		err = s.driver.withDelivery(ctx, item, func(_ *gorm.DB, row *m.TaskHistory) error {
			meta := metadata(row)
			deliveryAttempt = row.Attempt
			raw, e := json.Marshal(item)
			if e != nil {
				return e
			}
			meta[runtimeReceiptKey] = receipt
			meta[runtimeDeliveryKey] = string(raw)
			setMetadata(row, meta)
			return nil
		})
		if err != nil {
			return nil, err
		}
		meta := map[string]string{}
		for key, value := range item.Message.Metadata {
			if !strings.HasPrefix(key, "runtime_host_") {
				meta[key] = value
			}
		}
		v := RuntimeMessage{ID: item.Message.ID, TenantKey: scope.TenantUUID, SubscriberID: in.SubscriberID, Topic: item.Message.Topic, PayloadBase64: base64.StdEncoding.EncodeToString(item.Message.Payload), IdempotencyKey: item.Message.IdempotencyKey, Headers: item.Message.Headers, TraceID: item.Message.TraceID, Metadata: meta, Attempt: deliveryAttempt, Lease: &RuntimeLease{ReceiptUUID: receipt, Fence: item.Lease.Token, ExpiresAt: item.Lease.ExpiresAt}}
		if !item.Message.VisibleAt.IsZero() {
			at := item.Message.VisibleAt
			v.VisibleAt = &at
		}
		out = append(out, v)
	}
	return map[string]any{"messages": out}, nil
}
func (s *RuntimeService) settle(ctx context.Context, scope RuntimeScope, in RuntimeRequest) (map[string]any, error) {
	if !runtimeValidPart(in.SubscriberID, 128) || !runtimeValidPart(in.MessageID, 128) {
		return nil, runtimeError(400, "TASK_QUEUE_INVALID_ARGUMENT")
	}
	if id, e := uuid.Parse(in.ReceiptUUID); e != nil || id == uuid.Nil {
		return nil, runtimeError(400, "TASK_QUEUE_INVALID_ARGUMENT")
	}
	row, err := s.driver.Status(ctx, scope.TenantUUID, runtimeSubscriber(scope, in.SubscriberID), in.MessageID)
	if err != nil {
		return nil, err
	}
	if row == nil || metadata(row)[runtimeReceiptKey] != in.ReceiptUUID {
		return nil, event_bus.ErrLeaseStale
	}
	var item event_bus.StreamTaskDelivery
	if json.Unmarshal([]byte(metadata(row)[runtimeDeliveryKey]), &item) != nil || item.Message.ID != in.MessageID || item.Message.TenantKey != scope.TenantUUID || item.Message.SubscriberID != runtimeSubscriber(scope, in.SubscriberID) {
		return nil, event_bus.ErrLeaseStale
	}
	if in.Operation == "ack" && terminal(row.Status) {
		return map[string]any{"confirmed": true, "receipt_uuid": in.ReceiptUUID, "already_settled": true}, nil
	}
	switch in.Operation {
	case "renew":
		renewed, e := s.driver.base.Renew(ctx, item)
		if e != nil {
			return nil, e
		}
		err = s.driver.repo.WithRecord(ctx, scope.TenantUUID, item.Message.SubscriberID, item.Message.ID, func(_ *gorm.DB, row *m.TaskHistory) error {
			if metadata(row)[runtimeReceiptKey] != in.ReceiptUUID || terminal(row.Status) {
				return event_bus.ErrLeaseStale
			}
			meta := metadata(row)
			raw, e := json.Marshal(renewed)
			if e != nil {
				return e
			}
			meta[runtimeDeliveryKey] = string(raw)
			setMetadata(row, meta)
			return nil
		})
		if err != nil {
			return nil, err
		}
		return map[string]any{"lease": RuntimeLease{ReceiptUUID: in.ReceiptUUID, Fence: renewed.Lease.Token, ExpiresAt: renewed.Lease.ExpiresAt}}, nil
	case "ack":
		if in.Outcome != "completed" && in.Outcome != "failed" {
			return nil, runtimeError(400, "TASK_QUEUE_RESULT_REQUIRED")
		}
		err = s.driver.withDelivery(ctx, item, func(_ *gorm.DB, row *m.TaskHistory) error {
			now := time.Now().UTC()
			row.Status = m.TaskHistoryStatusCompleted
			if in.Outcome == "failed" {
				row.Status = m.TaskHistoryStatusFailed
				row.ErrorMessage = "TASK_QUEUE_HANDLER_FAILED"
			}
			row.CompletedAt = &now
			meta := metadata(row)
			raw, err := json.Marshal(in.Metadata)
			if err != nil {
				return err
			}
			meta["runtime_host_result"] = string(raw)
			setMetadata(row, meta)
			return nil
		})
		if err != nil {
			return nil, err
		}
		if err = s.driver.Ack(ctx, item); err != nil {
			return nil, err
		}
	case "nack":
		if !runtimeValidPart(in.Reason, 128) {
			return nil, runtimeError(400, "TASK_QUEUE_INVALID_ARGUMENT")
		}
		when := time.Time{}
		if in.RetryAt != nil {
			when = in.RetryAt.UTC()
		}
		err = s.driver.RejectAt(ctx, item, in.Reason, false, when)
		if err != nil {
			return nil, err
		}
	}
	return map[string]any{"confirmed": true, "receipt_uuid": in.ReceiptUUID}, nil
}
