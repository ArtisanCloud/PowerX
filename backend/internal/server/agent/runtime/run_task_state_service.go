package runtime

import (
	"context"
	"fmt"
	"strings"
	"time"

	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/repository"
	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type RunTaskStateService struct {
	repo *repository.RunTaskStateRepository
}
type runTaskStateServiceKey struct{}

const runTaskStatePersistTimeout = 5 * time.Second

func NewRunTaskStateService(db *gorm.DB) *RunTaskStateService {
	return &RunTaskStateService{repo: repository.NewRunTaskStateRepository(db)}
}
func ContextWithRunTaskStateService(ctx context.Context, s *RunTaskStateService) context.Context {
	if ctx == nil || s == nil {
		return ctx
	}
	return context.WithValue(ctx, runTaskStateServiceKey{}, s)
}
func runTaskStateServiceFromContext(ctx context.Context) (*RunTaskStateService, bool) {
	s, ok := ctx.Value(runTaskStateServiceKey{}).(*RunTaskStateService)
	return s, ok && s != nil
}
func (s *RunTaskStateService) Persist(ctx context.Context, task flowschema.PlanTask, status string) error {
	if s == nil || s.repo == nil {
		return fmt.Errorf("run task state service is not configured")
	}
	run, er := uuid.Parse(contextString(ctx, "runtime_run_uuid"))
	snap, es := uuid.Parse(contextString(ctx, "runtime_snapshot_uuid"))
	rev, ev := uuid.Parse(contextString(ctx, "runtime_plan_revision_uuid"))
	tenant := firstNonEmpty(strings.TrimSpace(contextString(ctx, "tenant_uuid")), strings.TrimSpace(reqctx.GetTenantUUID(ctx)))
	env := firstNonEmpty(strings.TrimSpace(contextString(ctx, "env")), strings.TrimSpace(reqctx.GetEnv(ctx)))
	if er != nil || es != nil || ev != nil || tenant == "" || env == "" {
		return fmt.Errorf("run task state references are required")
	}
	// A terminal task-state update must survive cancellation of the execution
	// context that caused it. Its complete scope is already captured above, so
	// persist it with an independent bounded context.
	persistCtx, cancel := context.WithTimeout(context.Background(), runTaskStatePersistTimeout)
	defer cancel()
	return s.repo.Upsert(persistCtx, &dbmodel.AgentRunTaskState{Env: env, TenantUUID: tenant, RunUUID: run, SnapshotUUID: snap, PlanRevisionUUID: rev, TaskID: task.TaskID, Status: status})
}

func (s *RunTaskStateService) BuildResumePlan(ctx context.Context, env, tenantUUID string, runUUID, revisionUUID uuid.UUID, plan flowschema.ExecutionPlan) (flowschema.ExecutionPlan, error) {
	if s == nil || s.repo == nil || strings.TrimSpace(plan.PlanID) == "" {
		return flowschema.ExecutionPlan{}, fmt.Errorf("resume plan input is incomplete")
	}
	rows, err := s.repo.List(ctx, env, tenantUUID, runUUID, revisionUUID)
	if err != nil {
		return flowschema.ExecutionPlan{}, err
	}
	states := make(map[string]string, len(rows))
	for _, row := range rows {
		if _, exists := states[row.TaskID]; exists {
			return flowschema.ExecutionPlan{}, fmt.Errorf("duplicate persisted task state")
		}
		states[row.TaskID] = row.Status
	}
	all := make(map[string]flowschema.PlanTask, len(plan.Tasks))
	for _, task := range plan.Tasks {
		if strings.TrimSpace(task.TaskID) == "" {
			return flowschema.ExecutionPlan{}, fmt.Errorf("resume plan task_id is required")
		}
		all[task.TaskID] = task
	}
	next := flowschema.ExecutionPlan{PlanID: plan.PlanID + "_approved_resume"}
	for _, source := range plan.Tasks {
		if states[source.TaskID] == "completed" {
			continue
		}
		task := cloneExecutionPlan(flowschema.ExecutionPlan{Tasks: []flowschema.PlanTask{source}}).Tasks[0]
		dependencies := task.DependsOn[:0]
		for _, dependency := range task.DependsOn {
			if _, exists := all[dependency]; !exists {
				return flowschema.ExecutionPlan{}, fmt.Errorf("resume plan dependency is outside plan")
			}
			if states[dependency] == "completed" {
				continue
			}
			dependencies = append(dependencies, dependency)
		}
		task.DependsOn = dependencies
		next.Tasks = append(next.Tasks, task)
	}
	if len(next.Tasks) == 0 {
		return flowschema.ExecutionPlan{}, fmt.Errorf("approved plan has no remaining tasks")
	}
	return next, nil
}
