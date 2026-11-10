package workers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	queue "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/task_queue"
	scheduler "github.com/ArtisanCloud/PowerX/internal/service/runtime_scheduler"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/runtime_scheduler"
	setting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"testing"
)

type schedulerQueueProbe struct {
	scope   queue.RuntimeScope
	request queue.RuntimeRequest
	calls   int
}

func (p *schedulerQueueProbe) Apply(_ context.Context, s queue.RuntimeScope, r queue.RuntimeRequest) (map[string]any, error) {
	p.scope = s
	p.request = r
	p.calls++
	return map[string]any{"accepted": true}, nil
}
func TestRuntimeSchedulerPublisherPinsTenantOwnerAndLiveCredential(t *testing.T) {
	previous := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = previous })
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&setting.PluginInstanceConfig{}))
	tenant := "61ecb9c0-3e89-4b02-b907-c371fbda4eac"
	owner := "com.powerx.plugins.ecommerce"
	raw, _ := json.Marshal(map[string]any{"client_id": "client-1", "allowed_capabilities": []string{queue.ConsumeCapability}})
	row := setting.PluginInstanceConfig{TenantUUID: tenant, PluginID: owner, Key: "auth.credentials", Enabled: true, Status: setting.PluginInstanceStatusEnabled, ValueJSON: datatypes.JSON(raw)}
	require.NoError(t, db.Create(&row).Error)
	probe := &schedulerQueueProbe{}
	p := &RuntimeSchedulerTaskPublisher{db: db, queue: probe}
	run := &models.SchedulerJobRun{TenantUUID: tenant, OwnerType: models.OwnerTypePlugin, OwnerID: owner, EventID: "event-1"}
	run.UUID = uuid.New()
	run.JobUUID = uuid.New()
	dispatch := scheduler.TaskDispatch{Contract: scheduler.TaskDispatchContract, SubscriberID: "runtime.scheduler.jobs", Payload: json.RawMessage(`{"business_action":"worker"}`)}
	require.NoError(t, p.PublishScheduledTask(context.Background(), run, dispatch))
	require.Equal(t, "sts:"+owner+":client:client-1", probe.scope.SourceService)
	require.Equal(t, tenant, probe.scope.TenantUUID)
	require.Equal(t, tenant, probe.request.Message.TenantKey)
	require.Equal(t, "scheduler:"+run.UUID.String(), probe.request.Message.IdempotencyKey)
	decoded, err := base64.StdEncoding.DecodeString(probe.request.Message.PayloadBase64)
	require.NoError(t, err)
	require.JSONEq(t, string(dispatch.Payload), string(decoded))
	// Foreign tenant, drain state, and revoked capability never enqueue.
	run.TenantUUID = uuid.NewString()
	require.Error(t, p.PublishScheduledTask(context.Background(), run, dispatch))
	run.TenantUUID = tenant
	require.NoError(t, db.Model(&row).Updates(map[string]any{"status": setting.PluginInstanceStatusDrainingRequested}).Error)
	require.Error(t, p.PublishScheduledTask(context.Background(), run, dispatch))
	require.NoError(t, db.Model(&row).Updates(map[string]any{"status": setting.PluginInstanceStatusEnabled, "value_json": `{"client_id":"client-1","allowed_capabilities":[]}`}).Error)
	require.Error(t, p.PublishScheduledTask(context.Background(), run, dispatch))
	require.Equal(t, 1, probe.calls)
}
