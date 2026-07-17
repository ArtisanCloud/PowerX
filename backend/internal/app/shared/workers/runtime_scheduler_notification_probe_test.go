package workers

import (
	"context"
	"testing"
	"time"

	notificationssvc "github.com/ArtisanCloud/PowerX/internal/service/notifications"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	notificationmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/notification"
	runtimeschedulermodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/runtime_scheduler"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestRuntimeSchedulerNotificationProbeIgnoresCanceledEventContext(t *testing.T) {
	prevSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = prevSchema })

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&parseTime=true&_loc=UTC"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&notificationmodel.Notification{}); err != nil {
		t.Fatalf("migrate notification: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	probe := NewRuntimeSchedulerNotificationProbe(RuntimeSchedulerNotificationProbeOptions{
		Notifications: notificationssvc.NewService(db),
		Clock: func() time.Time {
			return time.Date(2026, 7, 16, 15, 30, 0, 0, time.UTC)
		},
	})
	err = probe.handleEvent(event_bus.Event{
		Name: runtimeschedulermodel.TopicSchedulerTriggeredV1,
		Ctx:  ctx,
		Payload: map[string]any{
			"business_action": "framework_lab_scheduler_probe",
			"tenant_uuid":     "6b5d0240-9920-46da-b707-88200e0f51ea",
			"job_id":          "ad0f3015-39e6-49c3-a619-6bd329cee18b",
			"job_name":        "framework_lab_once_1784215878913",
			"fired_at":        "2026-07-16T15:32:18Z",
			"trace_id":        "trace-runtime-scheduler",
		},
	})
	if err != nil {
		t.Fatalf("handleEvent() error = %v", err)
	}

	var count int64
	if err := db.Model(&notificationmodel.Notification{}).
		Where("tenant_uuid = ? AND related_id = ?", "6b5d0240-9920-46da-b707-88200e0f51ea", "ad0f3015-39e6-49c3-a619-6bd329cee18b").
		Count(&count).Error; err != nil {
		t.Fatalf("count notification: %v", err)
	}
	if count != 1 {
		t.Fatalf("notification count = %d, want 1", count)
	}
}
