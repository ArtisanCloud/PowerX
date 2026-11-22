package taskqueue

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	history "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const DatasetPreviewSubscriber = "powerx.storage.dataset-preview.v1"
const DatasetPreviewTopic = "storage.dataset.preview.requested"
const DatasetExecutionSubscriber = "powerx.storage.dataset-execution.v1"
const DatasetExecutionTopic = "storage.dataset.execution.requested"

type datasetTaskChannel struct {
	subscriber, topic, source, prefix string
}

var datasetPreview = datasetTaskChannel{DatasetPreviewSubscriber, DatasetPreviewTopic, "core_storage_dataset_preview", "core-storage-preview:"}
var datasetExecution = datasetTaskChannel{DatasetExecutionSubscriber, DatasetExecutionTopic, "core_storage_dataset_execution", "core-storage-execution:"}

func datasetTaskIDReserved(id string) bool {
	return strings.HasPrefix(id, datasetPreview.prefix) || strings.HasPrefix(id, datasetExecution.prefix)
}

// RegisterDatasetPreview adds an authoritative delivery in the caller's
// transaction. Ordinary Host enqueue never sets this source or task ID.
func (s *RuntimeService) RegisterDatasetPreview(ctx context.Context, tx *gorm.DB, scope RuntimeScope, requestID string, notice []byte) (event_bus.TaskMessage, error) {
	return s.registerDatasetTask(ctx, tx, scope, datasetPreview, requestID, notice)
}

// RegisterDatasetExecution writes the dedicated execution source in the intent transaction.
func (s *RuntimeService) RegisterDatasetExecution(ctx context.Context, tx *gorm.DB, scope RuntimeScope, requestID string, notice []byte) (event_bus.TaskMessage, error) {
	return s.registerDatasetTask(ctx, tx, scope, datasetExecution, requestID, notice)
}

func (s *RuntimeService) registerDatasetTask(ctx context.Context, tx *gorm.DB, scope RuntimeScope, channel datasetTaskChannel, requestID string, notice []byte) (event_bus.TaskMessage, error) {
	msg := event_bus.TaskMessage{}
	if s == nil || s.driver == nil || s.driver.base == nil || tx == nil ||
		scope.EnvironmentID != s.driver.base.EnvironmentNamespace() || scope.EnvironmentID == "" ||
		!runtimeValidPart(scope.SourceService, 256) || !runtimeValidPart(scope.TenantUUID, 36) {
		return msg, runtimeError(503, "DATASET_TASKBUS_UNAVAILABLE")
	}
	msg = event_bus.TaskMessage{
		ID: channel.prefix + requestID, TenantKey: scope.TenantUUID,
		SubscriberID: runtimeSubscriber(scope, channel.subscriber),
		Topic:        channel.topic, Payload: notice, IdempotencyKey: channel.prefix + requestID,
	}
	raw, err := json.Marshal(msg)
	if err != nil {
		return msg, err
	}
	now := time.Now().UTC()
	row := history.TaskHistory{TaskID: msg.ID, TenantKey: msg.TenantKey, SubscriberID: msg.SubscriberID,
		Topic: msg.Topic, Source: channel.source, Status: history.TaskHistoryStatusPending,
		Payload: string(raw), Metadata: datatypes.JSON(`{}`), SubmittedAt: &now, LastSeenAt: now}
	return msg, s.driver.repo.Register(ctx, tx, &row)
}

// DeliverDatasetTask publishes an already committed task.
func (s *RuntimeService) DeliverDatasetTask(ctx context.Context, msg event_bus.TaskMessage) error {
	return s.driver.base.Enqueue(ctx, msg)
}

// recoverDatasetTasks repairs the commit/publish gap using the shared task
// history on the dedicated consumer's normal blocking receive path.
func (s *RuntimeService) recoverDatasetTasks(ctx context.Context, scope RuntimeScope, channel datasetTaskChannel) error {
	subscriber := runtimeSubscriber(scope, channel.subscriber)
	rows, err := s.driver.repo.PendingBySource(ctx, scope.TenantUUID, subscriber, channel.source, 100)
	if err != nil {
		return err
	}
	for _, row := range rows {
		var msg event_bus.TaskMessage
		if json.Unmarshal([]byte(row.Payload), &msg) != nil || msg.ID != row.TaskID ||
			msg.TenantKey != scope.TenantUUID || msg.SubscriberID != subscriber || msg.Topic != channel.topic {
			continue
		}
		if err := s.DeliverDatasetTask(ctx, msg); err != nil {
			return err
		}
	}
	return nil
}

// UseDatasetPreviewLease resolves the real Host delivery, checks the Core-only
// source under the task lock and applies the receipt update in that transaction.
func (s *RuntimeService) UseDatasetPreviewLease(ctx context.Context, scope RuntimeScope, requestID, receipt string, fence uint64, complete bool, apply func(*gorm.DB, time.Time) error) error {
	return s.useDatasetTaskLease(ctx, scope, datasetPreview, requestID, receipt, fence, complete, apply)
}

// UseDatasetExecutionLease gates an initial source-side grant on a current delivery.
func (s *RuntimeService) UseDatasetExecutionLease(ctx context.Context, scope RuntimeScope, requestID, receipt string, fence uint64, apply func(*gorm.DB, time.Time) error) error {
	return s.useDatasetTaskLease(ctx, scope, datasetExecution, requestID, receipt, fence, false, apply)
}

func (s *RuntimeService) useDatasetTaskLease(ctx context.Context, scope RuntimeScope, channel datasetTaskChannel, requestID, receipt string, fence uint64, complete bool, apply func(*gorm.DB, time.Time) error) error {
	if s == nil || s.driver == nil || s.driver.base == nil || scope.EnvironmentID != s.driver.base.EnvironmentNamespace() {
		return runtimeError(503, "DATASET_TASKBUS_UNAVAILABLE")
	}
	subscriber := runtimeSubscriber(scope, channel.subscriber)
	taskID := channel.prefix + requestID
	row, err := s.driver.Status(ctx, scope.TenantUUID, subscriber, taskID)
	if err != nil {
		return err
	}
	if row == nil || row.Source != channel.source || row.Topic != channel.topic || metadata(row)[runtimeReceiptKey] != receipt {
		return event_bus.ErrLeaseStale
	}
	var item event_bus.StreamTaskDelivery
	if json.Unmarshal([]byte(metadata(row)[runtimeDeliveryKey]), &item) != nil ||
		item.Message.ID != taskID || item.Message.TenantKey != scope.TenantUUID || item.Message.SubscriberID != subscriber ||
		item.Lease.TenantKey != scope.TenantUUID || item.Lease.SubscriberID != subscriber || item.Lease.MessageID != taskID ||
		item.Lease.Token != fence || !time.Now().Before(item.Lease.ExpiresAt) {
		return event_bus.ErrLeaseStale
	}
	return s.driver.withDelivery(ctx, item, func(tx *gorm.DB, row *history.TaskHistory) error {
		if row.Source != channel.source || row.Topic != channel.topic || metadata(row)[runtimeReceiptKey] != receipt ||
			!time.Now().Before(item.Lease.ExpiresAt) {
			return event_bus.ErrLeaseStale
		}
		if err := apply(tx, item.Lease.ExpiresAt); err != nil {
			return err
		}
		if complete {
			now := time.Now().UTC()
			row.Status = history.TaskHistoryStatusCompleted
			row.CompletedAt = &now
			row.ErrorMessage = ""
		}
		return nil
	})
}

// SettleDatasetExecution atomically records the durable source result and closes
// the original task. The callback must match the already-issued write grant
// even if Redis and the delivery lease were lost after the source commit.
func (s *RuntimeService) SettleDatasetExecution(ctx context.Context, scope RuntimeScope, requestID string, apply func(*gorm.DB) error) error {
	if s == nil || s.driver == nil || s.driver.base == nil || scope.EnvironmentID != s.driver.base.EnvironmentNamespace() {
		return runtimeError(503, "DATASET_TASKBUS_UNAVAILABLE")
	}
	channel := datasetExecution
	return s.driver.repo.WithRecord(ctx, scope.TenantUUID, runtimeSubscriber(scope, channel.subscriber), channel.prefix+requestID, func(tx *gorm.DB, row *history.TaskHistory) error {
		if row.Source != channel.source || row.Topic != channel.topic {
			return event_bus.ErrLeaseStale
		}
		if err := apply(tx); err != nil {
			return err
		}
		if row.Status != history.TaskHistoryStatusCompleted {
			now := time.Now().UTC()
			row.Status, row.CompletedAt, row.ErrorMessage = history.TaskHistoryStatusCompleted, &now, ""
		}
		return nil
	})
}
