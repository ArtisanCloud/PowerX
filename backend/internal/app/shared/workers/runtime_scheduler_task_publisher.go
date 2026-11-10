package workers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	queue "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/task_queue"
	scheduler "github.com/ArtisanCloud/PowerX/internal/service/runtime_scheduler"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/runtime_scheduler"
	setting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	"gorm.io/gorm"
)

type schedulerTaskQueue interface {
	Apply(context.Context, queue.RuntimeScope, queue.RuntimeRequest) (map[string]any, error)
}

type RuntimeSchedulerTaskPublisher struct {
	db    *gorm.DB
	queue schedulerTaskQueue
}

func NewRuntimeSchedulerTaskPublisher(db *gorm.DB, service *queue.RuntimeService) *RuntimeSchedulerTaskPublisher {
	return &RuntimeSchedulerTaskPublisher{db: db, queue: service}
}

// Core owns the persisted scheduler run. Resolve its exact enabled plugin
// credential namespace; never use the caller/payload to choose another queue.
func (p *RuntimeSchedulerTaskPublisher) PublishScheduledTask(ctx context.Context, run *models.SchedulerJobRun, dispatch scheduler.TaskDispatch) error {
	if p == nil || p.db == nil || p.queue == nil || run == nil || run.OwnerType != models.OwnerTypePlugin || run.TenantUUID == "" || run.OwnerID == "" || dispatch.Contract != scheduler.TaskDispatchContract {
		return errors.New("SCHEDULER_TASK_QUEUE_UNAVAILABLE")
	}
	var record setting.PluginInstanceConfig
	if err := p.db.WithContext(ctx).Where("tenant_uuid = ? AND plugin_id = ? AND key = ? AND enabled = ?", run.TenantUUID, run.OwnerID, "auth.credentials", true).Where("status = ?", setting.PluginInstanceStatusEnabled).First(&record).Error; err != nil {
		return errors.New("SCHEDULER_PLUGIN_NOT_ENABLED")
	}
	var credential struct {
		ClientID  string   `json:"client_id"`
		Allowed   []string `json:"allowed_capabilities"`
		ExpiresAt *int64   `json:"expires_at"`
	}
	if json.Unmarshal(record.ValueJSON, &credential) != nil || strings.TrimSpace(credential.ClientID) == "" || credential.ExpiresAt != nil && *credential.ExpiresAt <= time.Now().Unix() {
		return errors.New("SCHEDULER_PLUGIN_CREDENTIAL_INVALID")
	}
	consumeAllowed := false
	for _, id := range credential.Allowed {
		if id == queue.ConsumeCapability {
			consumeAllowed = true
		}
	}
	if !consumeAllowed {
		return errors.New("SCHEDULER_TASK_CONSUME_GRANT_REQUIRED")
	}
	result, err := p.queue.Apply(ctx, queue.RuntimeScope{TenantUUID: run.TenantUUID, SourceService: "sts:" + run.OwnerID + ":client:" + credential.ClientID}, queue.RuntimeRequest{
		ContractVersion: queue.RuntimeContract, Operation: "enqueue", TenantKey: run.TenantUUID,
		Message: &queue.RuntimeMessage{ID: run.EventID, IdempotencyKey: "scheduler:" + run.UUID.String(), TenantKey: run.TenantUUID, SubscriberID: dispatch.SubscriberID, Topic: models.TopicSchedulerTriggeredV1, PayloadBase64: base64.StdEncoding.EncodeToString(dispatch.Payload), TraceID: run.TraceID, Metadata: map[string]string{"plugin_id": run.OwnerID, "scheduler_job_id": run.JobUUID.String(), "scheduler_run_id": run.UUID.String()}},
	})
	if err != nil {
		return err
	}
	if accepted, ok := result["accepted"].(bool); !ok || !accepted {
		return errors.New("SCHEDULER_TASK_QUEUE_NOT_ACCEPTED")
	}
	return nil
}
