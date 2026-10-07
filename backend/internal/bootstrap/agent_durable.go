package bootstrap

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/ArtisanCloud/PowerX/internal/infra/media/driver"
	"github.com/ArtisanCloud/PowerX/internal/server/agent"
	runtime "github.com/ArtisanCloud/PowerX/internal/server/agent/runtime"
	"github.com/ArtisanCloud/PowerX/internal/service/agent_run"
	sessions "github.com/ArtisanCloud/PowerX/internal/service/agent_session"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/agent"
	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Bootstrap only assembles the Worker; cmd/app owns its process lifecycle.
func configureDurableSessions(ctx context.Context, cfg *config.Config, deps *shared.Deps, db *gorm.DB) error {
	c := cfg.AI.Runtime.DurableSessions.WithLifecycleDefaults()
	if err := c.Validate(); err != nil {
		return err
	}
	if !c.Enabled {
		return nil
	}
	if deps == nil || db == nil || deps.EventFabric == nil || deps.EventFabric.RedisClient == nil || deps.MediaMgr == nil {
		return fmt.Errorf("Redis, database and media dependencies are required")
	}
	if cfg.AI.Runtime.MaxSteps < 1 || cfg.AI.Runtime.MaxSteps > 1000 || cfg.AI.Runtime.MaxCapabilityCalls < 1 || cfg.AI.Runtime.MaxCapabilityCalls > cfg.AI.Runtime.MaxSteps {
		return fmt.Errorf("invalid durable plan step/capability budget")
	}
	if err := config.ValidateDeploymentEnv(cfg.Deployment.Env); err != nil {
		return err
	}
	gateCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := agent_run.ValidateProductionRedis(gateCtx, deps.EventFabric.RedisClient); err != nil {
		return err
	}
	capacity, err := agent_run.NewExecutionCapacity(gateCtx, deps.EventFabric.RedisClient, cfg.Deployment.Env,
		agent_run.ExecutionCapacityPolicy{TenantLimit: c.TenantConcurrency, RunLimit: cfg.AI.Runtime.MaxConcurrentTasks, LeaseTTL: c.LeaseTTL})
	if err != nil {
		return fmt.Errorf("Agent execution capacity: %w", err)
	}
	for _, column := range []string{"run_env", "admission_state", "response_envelope", "archive_key", "archived_at", "hot_expires_at"} {
		if !db.Migrator().HasColumn(&model.ServiceInvocation{}, column) {
			return fmt.Errorf("Agent Session migration required: %s", column)
		}
	}
	if !db.Migrator().HasColumn(&model.ServiceMessage{}, "response_envelope") {
		return fmt.Errorf("Agent message migration required: response_envelope")
	}
	if err := deps.MediaMgr.EnsureDriver("s3"); err != nil {
		return err
	}
	objects := agent_run.MediaReportObjects{Manager: deps.MediaMgr, Bucket: c.ReportBucket}
	owner := "agent-worker-" + uuid.NewString()
	// Verify the exact archive bucket before accepting a Run; never fall back to local files.
	probe := "agent-runtime/probes/" + owner + ".json"
	payload := []byte(`{"schema":"powerx.agent.storage-probe/v1"}`)
	if err := objects.Put(gateCtx, probe, payload); err != nil {
		return fmt.Errorf("report bucket write: %w", err)
	}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cleanupCancel()
		if err := deps.MediaMgr.Delete(cleanupCtx, "s3", driver.DeleteObjectInput{Bucket: c.ReportBucket, ObjectKey: probe}); err != nil {
			logger.WarnF(cleanupCtx, "Agent storage probe cleanup: %v", err)
		}
	}()
	actual, err := objects.Get(gateCtx, probe)
	if err != nil {
		return fmt.Errorf("report bucket read: %w", err)
	}
	if !bytes.Equal(actual, payload) {
		return fmt.Errorf("report bucket readback mismatch")
	}
	store, err := agent_run.NewRedisStoreWithQueueWaitTimeout(deps.EventFabric.RedisClient, cfg.AI.Runtime.QueueWaitTimeout)
	if err != nil {
		return err
	}
	queue, err := event_bus.NewRedisStreamTaskDriver(deps.EventFabric.RedisClient, c.LeaseTTL)
	if err != nil {
		return err
	}
	service := sessions.NewService(db)
	if err := service.ConfigureDurableAdmission(store, queue, cfg.Deployment.Env, cfg.AI.Runtime.RunDeadline); err != nil {
		return err
	}
	if err := service.ConfigureDurableOutput(&runtime.DurableFinalOutputReader{Runs: store, Objects: objects}); err != nil {
		return err
	}
	var admin *runtime.AdminRunService
	if c.AdminChatEnabled {
		if !db.Migrator().HasTable(&model.AdminRunAdmission{}) || !db.Migrator().HasColumn(&model.AdminRunAdmission{}, "archive_key") || !db.Migrator().HasColumn(&model.AdminRunAdmission{}, "archived_at") || !db.Migrator().HasColumn(&model.AdminRunAdmission{}, "hot_expires_at") {
			return fmt.Errorf("Agent admin run migration required")
		}
		admin = runtime.NewAdminRunService(db, store, queue, objects, cfg.Deployment.Env, cfg.AI.Runtime.RunDeadline)
		admin.ConfigureMedia(deps.MediaSvc)
	}
	loader := runtime.NewDurableConsumers(service, runtime.NewDBPlanningInputLoader(db), admin)
	loader.ConfigureArchiveQueue(queue)
	if err := loader.ConfigureArchive(store, objects); err != nil {
		return err
	}
	if err := loader.ConfigureArchiveLifecycle(cfg.Deployment.Env, agent_run.ArchivePolicy{HotRetention: c.HotRetention, MaxPending: c.ArchiveMaxPending, MaxAge: c.ArchiveMaxAge, HealthTTL: 3 * c.ScanInterval}); err != nil {
		return err
	}
	if err := loader.MaintainRuns(gateCtx); err != nil {
		return fmt.Errorf("archive health initialization: %w", err)
	}
	planner := &runtime.ServiceSessionPlanBuilder{Loader: loader, Engine: runtime.NewEngine(), Objects: objects,
		Budget: &agent_run.PlanBudget{MaxSteps: cfg.AI.Runtime.MaxSteps, MaxCapabilityCalls: cfg.AI.Runtime.MaxCapabilityCalls},
		// This is the task-execution pool. The LLM adapter independently resolves and leases the physical deployment pool.
		Pool: func(flowschema.PlanTask) (string, error) { return "agent-task-workers", nil },
	}
	executor := &runtime.ServiceSessionTaskExecutor{Runs: store, Objects: objects, Loader: loader, Invoker: &runtime.ManagerTaskInvoker{Manager: agent.GetAgentManager()}}
	worker := &agent_run.WorkerService{Store: store, Queue: queue, Capacity: capacity, Planner: planner, Executor: executor, Locator: loader, Concurrency: c.WorkerConcurrency, ScanInterval: c.ScanInterval, Owner: owner,
		OnError: func(err error) { logger.ErrorF(ctx, "Agent durable worker: %v", err) },
	}
	deps.AgentSessionSvc, deps.AgentRunWorker, deps.AgentAdminRun = service, worker, admin
	return nil
}
