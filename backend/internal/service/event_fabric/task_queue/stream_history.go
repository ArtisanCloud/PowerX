// Package taskqueue 为正式 Stream TaskBus 复用统一 event_task_histories。
package taskqueue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	r "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/event_fabric"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const recordIDKey = "taskbus_record_id"
const recordTenantKey = "taskbus_record_tenant"
const activeDeliveryKey = "taskbus_active_delivery"
const DefaultMaxAttempts = 3

var ErrTaskTerminal = errors.New("taskbus task already terminal")

// StreamDriver 的重试次数、投递 fencing 和恢复归统一驱动管理，业务消费者只提交结果。
type StreamDriver struct {
	base       *event_bus.RedisStreamTaskDriver
	repo       *r.TaskHistoryRepository
	retryDelay time.Duration
}

func NewStreamDriver(db *gorm.DB, base *event_bus.RedisStreamTaskDriver) *StreamDriver {
	return NewStreamDriverWithRetryDelay(db, base, 5*time.Second)
}
func NewStreamDriverWithRetryDelay(db *gorm.DB, base *event_bus.RedisStreamTaskDriver, delay time.Duration) *StreamDriver {
	if delay < time.Millisecond {
		delay = time.Millisecond
	}
	return &StreamDriver{base: base, repo: r.NewTaskHistoryRepository(db), retryDelay: delay}
}
func metadata(row *m.TaskHistory) map[string]string {
	out := map[string]string{}
	_ = json.Unmarshal(row.Metadata, &out)
	return out
}
func setMetadata(row *m.TaskHistory, meta map[string]string) {
	raw, _ := json.Marshal(meta)
	row.Metadata = datatypes.JSON(raw)
}
func recordKey(msg event_bus.TaskMessage) (string, string) {
	tenant, id := msg.Metadata[recordTenantKey], msg.Metadata[recordIDKey]
	if tenant == "" {
		tenant = msg.TenantKey
	}
	if id == "" {
		id = msg.ID
	}
	return tenant, id
}
func deliveryKey(d event_bus.StreamTaskDelivery) string {
	return d.Message.ID + ":" + strconv.FormatUint(d.Lease.Token, 10)
}
func terminal(status string) bool {
	return status == m.TaskHistoryStatusCompleted || status == m.TaskHistoryStatusFailed || status == m.TaskHistoryStatusCancelled
}
func attempts(msg event_bus.TaskMessage) int {
	n, _ := strconv.Atoi(msg.Metadata["max_attempts"])
	if n < 1 || n > 100 {
		return DefaultMaxAttempts
	}
	return n
}

// Register 可复用业务事务，解决业务提交成功但尚未入队时的恢复窗口。
func (d *StreamDriver) Register(ctx context.Context, tx *gorm.DB, msg event_bus.TaskMessage) error {
	tenant, id := recordKey(msg)
	raw, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	row := &m.TaskHistory{TaskID: id, TenantKey: tenant, SubscriberID: msg.SubscriberID, Topic: msg.Topic, Kind: msg.Metadata["kind"], Source: "task_driver", TraceID: msg.TraceID, Status: m.TaskHistoryStatusPending, Payload: string(raw), Metadata: datatypes.JSON(`{}`), SubmittedAt: &now, LastSeenAt: now}
	return d.repo.Register(ctx, tx, row)
}
func (d *StreamDriver) Status(ctx context.Context, tenant, subscriber, id string) (*m.TaskHistory, error) {
	return d.repo.FindByKey(ctx, tenant, subscriber, id)
}
func (d *StreamDriver) StatusWithTx(ctx context.Context, tx *gorm.DB, tenant, subscriber, id string) (*m.TaskHistory, error) {
	if tx == nil {
		return d.Status(ctx, tenant, subscriber, id)
	}
	return r.NewTaskHistoryRepository(tx).FindByKey(ctx, tenant, subscriber, id)
}

func (d *StreamDriver) Enqueue(ctx context.Context, msg event_bus.TaskMessage) error {
	if err := d.Register(ctx, nil, msg); err != nil {
		return err
	}
	tenant, id := recordKey(msg)
	row, err := d.Status(ctx, tenant, msg.SubscriberID, id)
	if err != nil {
		return err
	}
	if row != nil && terminal(row.Status) {
		return nil
	}
	err = d.base.Enqueue(ctx, msg)
	update := d.repo.WithRecord(context.WithoutCancel(ctx), tenant, msg.SubscriberID, id, func(_ *gorm.DB, row *m.TaskHistory) error {
		if row.Status != m.TaskHistoryStatusPending {
			return nil
		}
		row.ErrorMessage = ""
		if err != nil {
			row.ErrorMessage = "TASKBUS_ENQUEUE_FAILED"
		}
		return nil
	})
	if err != nil {
		return err
	}
	return update
}

func (d *StreamDriver) Dequeue(ctx context.Context, tenant, subscriber, group, worker string, max int64, wait time.Duration) ([]event_bus.StreamTaskDelivery, error) {
	items, err := d.base.Dequeue(ctx, tenant, subscriber, group, worker, max, wait)
	if err != nil {
		return nil, err
	}
	out := make([]event_bus.StreamTaskDelivery, 0, len(items))
	for _, item := range items {
		exhausted := false
		busy := false
		tenant, id := recordKey(item.Message)
		err = d.repo.WithRecord(ctx, tenant, item.Message.SubscriberID, id, func(_ *gorm.DB, row *m.TaskHistory) error {
			if terminal(row.Status) {
				return ErrTaskTerminal
			}
			meta := metadata(row)
			if meta[activeDeliveryKey] != deliveryKey(item) {
				if row.Status == m.TaskHistoryStatusRunning {
					var active event_bus.TaskLease
					if json.Unmarshal([]byte(meta["taskbus_lease"]), &active) == nil {
						err := d.base.ValidateLease(ctx, active)
						if err == nil {
							busy = true
							return nil
						}
						if !errors.Is(err, event_bus.ErrLeaseStale) {
							return err
						}
					}
				}
				if row.Attempt >= attempts(item.Message) {
					row.Status = m.TaskHistoryStatusFailed
					row.ErrorMessage = "TASKBUS_RETRIES_EXHAUSTED"
					now := time.Now().UTC()
					row.CompletedAt = &now
					exhausted = true
					return nil
				}
				row.Attempt++
				meta[activeDeliveryKey] = deliveryKey(item)
				raw, _ := json.Marshal(item.Lease)
				meta["taskbus_lease"] = string(raw)
			}
			row.Status = m.TaskHistoryStatusRunning
			row.ErrorMessage = ""
			now := time.Now().UTC()
			row.StartedAt = &now
			setMetadata(row, meta)
			return nil
		})
		if errors.Is(err, ErrTaskTerminal) {
			if e := d.base.Ack(ctx, item); e != nil {
				return out, e
			}
			continue
		}
		if err != nil {
			return out, err
		}
		if busy {
			if e := d.base.Defer(ctx, item, time.Now().Add(3*time.Second), "task_already_running"); e != nil {
				return out, e
			}
			continue
		}
		if exhausted {
			if _, e := d.base.Nack(ctx, item, time.Time{}, "TASKBUS_RETRIES_EXHAUSTED", item.Message.Attempt+1); e != nil {
				return out, e
			}
			continue
		}
		out = append(out, item)
	}
	return out, nil
}

func (d *StreamDriver) withDelivery(ctx context.Context, item event_bus.StreamTaskDelivery, apply func(*gorm.DB, *m.TaskHistory) error) error {
	if _, err := d.base.Renew(ctx, item); err != nil {
		return err
	}
	tenant, id := recordKey(item.Message)
	return d.repo.WithRecord(ctx, tenant, item.Message.SubscriberID, id, func(tx *gorm.DB, row *m.TaskHistory) error {
		if terminal(row.Status) {
			return ErrTaskTerminal
		}
		if metadata(row)[activeDeliveryKey] != deliveryKey(item) {
			return event_bus.ErrLeaseStale
		}
		return apply(tx, row)
	})
}

// Complete 把业务对象更新与共享任务完成放到同一个事务；旧投递不能接纳结果。
func (d *StreamDriver) Complete(ctx context.Context, item event_bus.StreamTaskDelivery, apply func(*gorm.DB) error) error {
	return d.withDelivery(ctx, item, func(tx *gorm.DB, row *m.TaskHistory) error {
		if err := apply(tx); err != nil {
			return err
		}
		now := time.Now().UTC()
		row.Status = m.TaskHistoryStatusCompleted
		row.ErrorMessage = ""
		row.CompletedAt = &now
		return nil
	})
}
func (d *StreamDriver) Ack(ctx context.Context, item event_bus.StreamTaskDelivery) error {
	return d.base.Ack(ctx, item)
}
func (d *StreamDriver) Cancel(ctx context.Context, item event_bus.StreamTaskDelivery, reason string) error {
	if err := d.withDelivery(ctx, item, func(_ *gorm.DB, row *m.TaskHistory) error {
		now := time.Now().UTC()
		row.Status = m.TaskHistoryStatusCancelled
		row.ErrorMessage = reason
		row.CompletedAt = &now
		return nil
	}); err != nil {
		return err
	}
	return d.base.Ack(ctx, item)
}

func (d *StreamDriver) Defer(ctx context.Context, item event_bus.StreamTaskDelivery, when time.Time, reason string) error {
	if err := d.withDelivery(ctx, item, func(_ *gorm.DB, row *m.TaskHistory) error {
		row.Status = m.TaskHistoryStatusDeferred
		row.Attempt = max(0, row.Attempt-1)
		row.ErrorMessage = reason
		return nil
	}); err != nil {
		return err
	}
	return d.base.Defer(ctx, item, when, reason)
}

// Reject 的上限与 DLQ 统一归 TaskBus。永久错误和达到上限的投递进入失败终态。
func (d *StreamDriver) Reject(ctx context.Context, item event_bus.StreamTaskDelivery, reason string, permanent bool) error {
	dead := false
	err := d.withDelivery(ctx, item, func(_ *gorm.DB, row *m.TaskHistory) error {
		dead = permanent || row.Attempt >= attempts(item.Message)
		row.Status = m.TaskHistoryStatusDeferred
		row.ErrorMessage = reason
		if dead {
			now := time.Now().UTC()
			row.Status = m.TaskHistoryStatusFailed
			row.CompletedAt = &now
		}
		return nil
	})
	if err != nil {
		return err
	}
	limit := attempts(item.Message)
	if dead {
		limit = item.Message.Attempt + 1
	}
	_, err = d.base.Nack(ctx, item, time.Now().Add(d.retryDelay), reason, limit)
	return err
}

// Recover 仅在消费者启动时使用共享历史补投；常驻消费依然由正式 TaskBus 阻塞驱动。
func (d *StreamDriver) Recover(ctx context.Context, queueTenant, subscriber string) error {
	var after uint64
	for {
		rows, err := d.repo.Recoverable(ctx, subscriber, after, 100)
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		for _, row := range rows {
			after = row.ID
			var msg event_bus.TaskMessage
			if json.Unmarshal([]byte(row.Payload), &msg) != nil || msg.TenantKey != queueTenant || msg.SubscriberID != subscriber {
				continue
			}
			if msg.Metadata == nil {
				msg.Metadata = map[string]string{}
			}
			msg.Metadata[recordIDKey] = row.TaskID
			msg.Metadata[recordTenantKey] = row.TenantKey
			msg.ID = fmt.Sprintf("%s:recover:%s", row.TaskID, uuid.NewString())
			msg.Attempt = max(0, row.Attempt-1)
			if err := d.base.Enqueue(ctx, msg); err != nil {
				return err
			}
		}
	}
}
