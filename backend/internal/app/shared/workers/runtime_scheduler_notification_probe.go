package workers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	eventbus "github.com/ArtisanCloud/PowerX/internal/event_bus"
	notificationssvc "github.com/ArtisanCloud/PowerX/internal/service/notifications"
	wsbus "github.com/ArtisanCloud/PowerX/internal/transport/websocket/bus"
	notificationmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/notification"
	runtimeschedulermodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/runtime_scheduler"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"gorm.io/datatypes"
)

const frameworkLabSchedulerProbeAction = "framework_lab_scheduler_probe"

type RuntimeSchedulerNotificationProbeOptions struct {
	EventBus      event_bus.EventBus
	Notifications *notificationssvc.Service
	Logger        *logger.Logger
	Clock         func() time.Time
}

type RuntimeSchedulerNotificationProbe struct {
	eventBus      event_bus.EventBus
	notifications *notificationssvc.Service
	logger        *logger.Logger
	clock         func() time.Time
	unsubscribe   func()
}

func NewRuntimeSchedulerNotificationProbe(opts RuntimeSchedulerNotificationProbeOptions) *RuntimeSchedulerNotificationProbe {
	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}
	log := opts.Logger
	if log == nil {
		log = logger.GetGlobalLogger()
	}
	return &RuntimeSchedulerNotificationProbe{
		eventBus:      opts.EventBus,
		notifications: opts.Notifications,
		logger:        log,
		clock:         clock,
	}
}

func (w *RuntimeSchedulerNotificationProbe) Start(ctx context.Context) error {
	if w == nil || w.eventBus == nil || w.notifications == nil {
		return errors.New("runtime scheduler notification probe requires event bus and notification service")
	}
	w.unsubscribe = w.eventBus.Subscribe(runtimeschedulermodel.TopicSchedulerTriggeredV1, func(evt event_bus.Event) error {
		return w.handleEvent(evt)
	})
	return nil
}

func (w *RuntimeSchedulerNotificationProbe) Stop() {
	if w != nil && w.unsubscribe != nil {
		w.unsubscribe()
		w.unsubscribe = nil
	}
}

func (w *RuntimeSchedulerNotificationProbe) handleEvent(evt event_bus.Event) error {
	payload, ok := eventPayloadMap(evt.Payload)
	if !ok {
		return nil
	}
	if strings.TrimSpace(stringValue(payload["business_action"])) != frameworkLabSchedulerProbeAction {
		return nil
	}

	tenantUUID := strings.TrimSpace(stringValue(payload["tenant_uuid"]))
	if tenantUUID == "" {
		tenantUUID = strings.TrimSpace(evt.TenantUUID)
	}
	if tenantUUID == "" {
		return errors.New("runtime scheduler notification probe missing tenant_uuid")
	}

	traceID := strings.TrimSpace(stringValue(payload["trace_id"]))
	if traceID == "" {
		traceID = strings.TrimSpace(evt.TraceID)
	}
	jobID := strings.TrimSpace(stringValue(payload["job_id"]))
	jobName := strings.TrimSpace(stringValue(payload["job_name"]))
	if jobName == "" {
		jobName = jobID
	}
	firedAt := strings.TrimSpace(stringValue(payload["fired_at"]))

	metadata, _ := json.Marshal(map[string]any{
		"kind":            "runtime_scheduler_framework_lab_probe",
		"job_id":          jobID,
		"job_name":        jobName,
		"owner_type":      stringValue(payload["owner_type"]),
		"owner_id":        stringValue(payload["owner_id"]),
		"trigger_source":  stringValue(payload["trigger_source"]),
		"business_action": frameworkLabSchedulerProbeAction,
		"trace_id":        traceID,
		"event_id":        stringValue(payload["event_id"]),
		"fired_at":        firedAt,
	})

	ctx := context.Background()
	if evt.Ctx != nil {
		ctx = context.WithoutCancel(evt.Ctx)
	}
	ctx = reqctx.WithTenantUUID(ctx, tenantUUID)
	if traceID != "" {
		ctx = reqctx.WithTraceID(ctx, traceID)
	}

	item, err := w.notifications.Create(ctx, notificationssvc.CreateInput{
		TenantUUID:  tenantUUID,
		Title:       "Runtime Scheduler 调试任务已触发",
		Content:     fmt.Sprintf("任务 %s 已在 %s 触发。", jobName, displayTime(firedAt, w.clock())),
		Type:        "success",
		Category:    "system",
		IsImportant: false,
		RelatedID:   jobID,
		RelatedType: "runtime_scheduler_job",
		Metadata:    datatypes.JSON(metadata),
	})
	if err != nil {
		return err
	}

	wsbus.DefaultHub.PublishWithContext(ctx, tenantUUID, eventbus.TopicSystemNotification, notificationView(item), traceID)
	return nil
}

func eventPayloadMap(payload any) (map[string]any, bool) {
	switch v := payload.(type) {
	case map[string]any:
		return v, true
	case []byte:
		var out map[string]any
		if err := json.Unmarshal(v, &out); err == nil {
			return out, true
		}
	case string:
		var out map[string]any
		if err := json.Unmarshal([]byte(v), &out); err == nil {
			return out, true
		}
	}
	return nil, false
}

func stringValue(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case fmt.Stringer:
		return val.String()
	case nil:
		return ""
	default:
		return fmt.Sprint(val)
	}
}

func displayTime(raw string, now time.Time) string {
	if strings.TrimSpace(raw) != "" {
		return raw
	}
	return now.Format(time.RFC3339)
}

func notificationView(item *notificationmodel.Notification) map[string]any {
	if item == nil {
		return map[string]any{}
	}
	view := map[string]any{
		"id":          item.UUID.String(),
		"title":       item.Title,
		"content":     item.Content,
		"type":        item.Type,
		"category":    item.Category,
		"isRead":      item.IsRead,
		"isImportant": item.IsImportant,
		"createdAt":   item.CreatedAt,
		"updatedAt":   item.UpdatedAt,
		"userId":      strings.TrimSpace(item.MemberUUID),
		"relatedId":   item.RelatedID,
		"relatedType": item.RelatedType,
	}
	if len(item.Metadata) > 0 {
		var metadata map[string]any
		if err := json.Unmarshal(item.Metadata, &metadata); err == nil {
			view["metadata"] = metadata
		}
	}
	return view
}
