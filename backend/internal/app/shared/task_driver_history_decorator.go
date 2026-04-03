package shared

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	eventfabricmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	eventfabricrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/event_fabric"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"gorm.io/datatypes"
)

type taskHistoryDriver struct {
	base  event_bus.TaskDriver
	repo  *eventfabricrepo.TaskHistoryRepository
	clock func() time.Time
}

func newTaskHistoryDriver(base event_bus.TaskDriver, repo *eventfabricrepo.TaskHistoryRepository, clock func() time.Time) event_bus.TaskDriver {
	if base == nil || repo == nil {
		return base
	}
	if clock == nil {
		clock = time.Now
	}
	return &taskHistoryDriver{base: base, repo: repo, clock: clock}
}

func (d *taskHistoryDriver) Type() event_bus.QueueDriverType {
	return d.base.Type()
}

func (d *taskHistoryDriver) Capability() event_bus.QueueDriverCapability {
	return d.base.Capability()
}

func (d *taskHistoryDriver) Enqueue(ctx context.Context, message event_bus.TaskMessage) error {
	if err := d.base.Enqueue(ctx, message); err != nil {
		return err
	}
	now := d.clock().UTC()
	status := eventfabricmodel.TaskHistoryStatusPending
	if !message.VisibleAt.IsZero() && message.VisibleAt.After(now) {
		status = eventfabricmodel.TaskHistoryStatusDeferred
	}
	d.save(ctx, message.TenantKey, message.SubscriberID, message.ID, func(item *eventfabricmodel.TaskHistory) {
		item.Topic = strings.TrimSpace(message.Topic)
		item.TraceID = strings.TrimSpace(message.TraceID)
		item.Kind = normalizeKind(message)
		item.Source = "task_driver"
		item.Status = status
		item.Attempt = message.Attempt
		item.Payload = payloadPreview(message.Payload)
		item.Metadata = toJSON(message.Metadata)
		submitted := now
		if item.SubmittedAt == nil {
			item.SubmittedAt = &submitted
		}
		item.LastSeenAt = now
	})
	return nil
}

func (d *taskHistoryDriver) Dequeue(ctx context.Context, request event_bus.DequeueRequest) ([]event_bus.TaskMessage, error) {
	messages, err := d.base.Dequeue(ctx, request)
	if err != nil {
		return nil, err
	}
	if len(messages) == 0 {
		return messages, nil
	}
	now := d.clock().UTC()
	for _, message := range messages {
		msg := message
		d.save(ctx, request.TenantKey, request.SubscriberID, msg.ID, func(item *eventfabricmodel.TaskHistory) {
			if item.Topic == "" {
				item.Topic = strings.TrimSpace(msg.Topic)
			}
			if item.TraceID == "" {
				item.TraceID = strings.TrimSpace(msg.TraceID)
			}
			if item.Kind == "" {
				item.Kind = normalizeKind(msg)
			}
			item.Source = "task_driver"
			item.Status = eventfabricmodel.TaskHistoryStatusRunning
			item.Attempt = msg.Attempt
			started := now
			if item.StartedAt == nil {
				item.StartedAt = &started
			}
			if item.SubmittedAt == nil {
				item.SubmittedAt = &started
			}
			item.LastSeenAt = now
		})
	}
	return messages, nil
}

func (d *taskHistoryDriver) Ack(ctx context.Context, request event_bus.AckRequest) error {
	if err := d.base.Ack(ctx, request); err != nil {
		return err
	}
	now := d.clock().UTC()
	d.save(ctx, request.TenantKey, request.SubscriberID, request.MessageID, func(item *eventfabricmodel.TaskHistory) {
		item.Source = "task_driver"
		item.Status = eventfabricmodel.TaskHistoryStatusCompleted
		item.ErrorMessage = ""
		completed := now
		item.CompletedAt = &completed
		if item.SubmittedAt == nil {
			item.SubmittedAt = &completed
		}
		item.LastSeenAt = now
	})
	return nil
}

func (d *taskHistoryDriver) Nack(ctx context.Context, request event_bus.NackRequest) error {
	if err := d.base.Nack(ctx, request); err != nil {
		return err
	}
	now := d.clock().UTC()
	status := eventfabricmodel.TaskHistoryStatusPending
	if !request.RetryAt.IsZero() && request.RetryAt.After(now) {
		status = eventfabricmodel.TaskHistoryStatusDeferred
	}
	d.save(ctx, request.TenantKey, request.SubscriberID, request.MessageID, func(item *eventfabricmodel.TaskHistory) {
		item.Source = "task_driver"
		item.Status = status
		item.ErrorMessage = strings.TrimSpace(request.Reason)
		item.Metadata = toJSON(request.Metadata)
		item.LastSeenAt = now
	})
	return nil
}

func (d *taskHistoryDriver) Retry(ctx context.Context, request event_bus.RetryRequest) error {
	if err := d.base.Retry(ctx, request); err != nil {
		return err
	}
	now := d.clock().UTC()
	msg := request.Message
	d.save(ctx, msg.TenantKey, msg.SubscriberID, msg.ID, func(item *eventfabricmodel.TaskHistory) {
		item.Topic = strings.TrimSpace(msg.Topic)
		item.TraceID = strings.TrimSpace(msg.TraceID)
		item.Kind = normalizeKind(msg)
		item.Source = "task_driver"
		item.Status = eventfabricmodel.TaskHistoryStatusDeferred
		item.Attempt = msg.Attempt
		item.ErrorMessage = strings.TrimSpace(request.Reason)
		item.LastSeenAt = now
	})
	return nil
}

func (d *taskHistoryDriver) save(ctx context.Context, tenantKey, subscriberID, taskID string, mutate func(item *eventfabricmodel.TaskHistory)) {
	if d == nil || d.repo == nil || mutate == nil {
		return
	}
	tenantKey = strings.TrimSpace(tenantKey)
	subscriberID = strings.TrimSpace(subscriberID)
	taskID = strings.TrimSpace(taskID)
	if tenantKey == "" || subscriberID == "" || taskID == "" {
		return
	}
	item, _ := d.repo.FindByKey(ctx, tenantKey, subscriberID, taskID)
	if item == nil {
		item = &eventfabricmodel.TaskHistory{
			TaskID:       taskID,
			TenantKey:    tenantKey,
			SubscriberID: subscriberID,
		}
	}
	mutate(item)
	_ = d.repo.Save(ctx, item)
}

func normalizeKind(message event_bus.TaskMessage) string {
	if message.Metadata != nil {
		if kind := strings.TrimSpace(message.Metadata["kind"]); kind != "" {
			return kind
		}
	}
	return ""
}

func payloadPreview(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	const max = 2048
	if len(raw) > max {
		return string(raw[:max]) + "...(truncated)"
	}
	return string(raw)
}

func toJSON(m map[string]string) datatypes.JSON {
	if len(m) == 0 {
		return datatypes.JSON([]byte("{}"))
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return datatypes.JSON([]byte("{}"))
	}
	return datatypes.JSON(raw)
}
