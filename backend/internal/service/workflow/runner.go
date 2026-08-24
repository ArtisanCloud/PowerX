package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"

	modelworkflow "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/workflow"
)

const (
	defaultRunnerLeaseDuration = 30 * time.Second
	defaultRunnerBatchSize     = 10
	defaultRunnerDrainLimit    = 50
)

var (
	ErrWorkflowRunnerUnavailable = errors.New("workflow.runner_unavailable")
	ErrWorkflowStepStaleAttempt  = errors.New("workflow.step_stale_attempt")
	ErrWorkflowStepDefinitionNil = errors.New("workflow.step_definition_unavailable")
)

type RunnerOptions struct {
	DefinitionStore DefinitionStore
	InstanceStore   InstanceStore
	StepStore       StepRecordStore
	AdapterRegistry *NodeAdapterRegistry
	ExecutorRouter  *ExecutorRouter
	Clock           func() time.Time
	TargetInstance  uuid.UUID
	LeaseOwner      string
	LeaseDuration   time.Duration
	BatchSize       int
	EventPublisher  runtimeEventPublisher
}

type Runner struct {
	definitions DefinitionStore
	instances   InstanceStore
	steps       StepRecordStore
	adapters    *NodeAdapterRegistry
	router      *ExecutorRouter
	now         func() time.Time
	targetInst  uuid.UUID
	leaseOwner  string
	leaseTTL    time.Duration
	batchSize   int
	publish     runtimeEventPublisher
}

type RunDueStepsResult struct {
	Leased    int
	Completed int
	Waiting   int
	Failed    int
	Skipped   int
}

type DrainDueStepsOptions struct {
	InstanceUUID  uuid.UUID
	LeaseOwner    string
	LeaseDuration time.Duration
	BatchSize     int
	MaxIterations int
}

func NewRunner(opts RunnerOptions) (*Runner, error) {
	if opts.DefinitionStore == nil || opts.InstanceStore == nil || opts.StepStore == nil || opts.AdapterRegistry == nil {
		return nil, ErrWorkflowRunnerUnavailable
	}
	router := opts.ExecutorRouter
	if router == nil {
		router = DefaultExecutorRouter()
	}
	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}
	leaseOwner := strings.TrimSpace(opts.LeaseOwner)
	if leaseOwner == "" {
		return nil, errors.New("workflow.runner_lease_owner_required")
	}
	leaseTTL := opts.LeaseDuration
	if leaseTTL <= 0 {
		leaseTTL = defaultRunnerLeaseDuration
	}
	batchSize := opts.BatchSize
	if batchSize <= 0 {
		batchSize = defaultRunnerBatchSize
	}
	return &Runner{
		definitions: opts.DefinitionStore,
		instances:   opts.InstanceStore,
		steps:       opts.StepStore,
		adapters:    opts.AdapterRegistry,
		router:      router,
		now:         clock,
		targetInst:  opts.TargetInstance,
		leaseOwner:  leaseOwner,
		leaseTTL:    leaseTTL,
		batchSize:   batchSize,
		publish:     opts.EventPublisher,
	}, nil
}

func (s *Service) NewRunner(opts RunnerOptions) (*Runner, error) {
	if s == nil {
		return nil, ErrWorkflowRunnerUnavailable
	}
	if opts.DefinitionStore == nil {
		opts.DefinitionStore = s.definitions
	}
	if opts.InstanceStore == nil {
		opts.InstanceStore = s.instances
	}
	if opts.StepStore == nil {
		opts.StepStore = s.steps
	}
	if opts.AdapterRegistry == nil {
		opts.AdapterRegistry = s.adapters
	}
	if opts.Clock == nil {
		opts.Clock = s.now
	}
	if opts.EventPublisher == nil {
		opts.EventPublisher = s.publishRuntimeEvent
	}
	return NewRunner(opts)
}

func (s *Service) DrainDueSteps(ctx context.Context, opts DrainDueStepsOptions) (RunDueStepsResult, error) {
	result := RunDueStepsResult{}
	if s == nil {
		return result, ErrWorkflowRunnerUnavailable
	}
	maxIterations := opts.MaxIterations
	if maxIterations <= 0 {
		maxIterations = defaultRunnerDrainLimit
	}
	runner, err := s.NewRunner(RunnerOptions{
		TargetInstance: opts.InstanceUUID,
		LeaseOwner:     opts.LeaseOwner,
		LeaseDuration:  opts.LeaseDuration,
		BatchSize:      opts.BatchSize,
	})
	if err != nil {
		return result, err
	}
	for i := 0; i < maxIterations; i++ {
		current, err := runner.RunDueSteps(ctx)
		result.Leased += current.Leased
		result.Completed += current.Completed
		result.Waiting += current.Waiting
		result.Failed += current.Failed
		result.Skipped += current.Skipped
		if err != nil {
			return result, err
		}
		if current.Leased == 0 || current.Waiting > 0 || current.Failed > 0 {
			return result, nil
		}
	}
	return result, nil
}

func (r *Runner) RunDueSteps(ctx context.Context) (RunDueStepsResult, error) {
	result := RunDueStepsResult{}
	if r == nil {
		return result, ErrWorkflowRunnerUnavailable
	}
	leaseUntil := r.now().UTC().Add(r.leaseTTL)
	var records []modelworkflow.WorkflowStepRecord
	var err error
	if r.targetInst != uuid.Nil {
		records, err = r.steps.LeaseQueuedStepsByInstance(ctx, r.targetInst, r.batchSize, r.leaseOwner, leaseUntil)
	} else {
		records, err = r.steps.LeaseQueuedSteps(ctx, r.batchSize, r.leaseOwner, leaseUntil)
	}
	if err != nil {
		return result, err
	}
	result.Leased = len(records)
	for i := range records {
		outcome, err := r.runLeasedStep(ctx, &records[i])
		switch outcome {
		case "completed":
			result.Completed++
		case "waiting":
			result.Waiting++
		case "failed":
			result.Failed++
		case "skipped":
			result.Skipped++
		}
		if err != nil {
			return result, err
		}
	}
	return result, nil
}

func (r *Runner) runLeasedStep(ctx context.Context, record *modelworkflow.WorkflowStepRecord) (string, error) {
	if record == nil {
		return "skipped", errors.New("workflow.step_record_required")
	}
	instance, err := r.instances.GetByUUIDAnyTenant(ctx, record.InstanceUUID)
	if err != nil {
		return "failed", err
	}
	if instance.State == "canceled" || instance.State == "suspended" {
		return "skipped", nil
	}
	r.publishRuntimeEvent(ctx, instance.TenantUUID, instance.UUID, "workflow.step.started", record.StepID, map[string]any{
		"step_record_id": record.ID,
		"attempt":        record.Attempt,
	})
	definition, validation, err := r.loadDefinition(ctx, instance)
	if err != nil {
		return "failed", err
	}
	_ = definition
	step, ok := validation.StepByID(record.StepID)
	if !ok {
		return "failed", fmt.Errorf("%w: %s", ErrWorkflowStepDefinitionNil, record.StepID)
	}
	adapter, err := r.adapters.Adapter(step.NodeKind)
	if err != nil {
		return "failed", r.failStep(ctx, record, instance, "workflow.node_adapter_unavailable", err.Error())
	}
	if err := adapter.Validate(step); err != nil {
		return "failed", r.failStep(ctx, record, instance, "workflow.node_validation_failed", err.Error())
	}

	payload, err := applyWorkflowInputMapping(jsonMap(record.PayloadIn), step.InputMapping)
	if err != nil {
		return "failed", r.failStep(ctx, record, instance, "workflow.input_mapping_failed", err.Error())
	}
	nodeResult, execErr := adapter.Execute(ctx, NodeExecutionContext{
		TenantUUID: strings.TrimSpace(instance.TenantUUID),
		Instance:   instance,
		StepRecord: record,
		Step:       step,
		AgentUUID:  instance.AgentUUID,
		TraceID:    instance.TraceID,
		Payload:    payload,
	})
	if execErr != nil {
		return "failed", r.failStep(ctx, record, instance, firstNonEmpty(nodeResult.ErrorCode, "workflow.node_execution_failed"), execErr.Error())
	}

	status := strings.TrimSpace(strings.ToLower(nodeResult.Status))
	if status == "" {
		return "failed", r.failStep(ctx, record, instance, "workflow.node_result_status_required", "workflow.node_result_status_required")
	}
	switch status {
	case NodeResultStatusWaiting:
		return "waiting", r.markStepWaiting(ctx, record, instance, nodeResult)
	case NodeResultStatusCompensating:
		return "failed", r.markInstanceCompensating(ctx, record, instance, nodeResult)
	case NodeResultStatusFailed:
		return "failed", r.failStep(ctx, record, instance, firstNonEmpty(nodeResult.ErrorCode, "workflow.node_failed"), firstNonEmpty(nodeResult.ErrorMessage, "workflow.node_failed"))
	case NodeResultStatusSucceeded:
		mappedOutput, err := applyWorkflowOutputMapping(nodeResult.Output, step.OutputMapping)
		if err != nil {
			return "failed", r.failStep(ctx, record, instance, "workflow.output_mapping_failed", err.Error())
		}
		nodeResult.Output = mappedOutput
		if err := r.completeStep(ctx, record, instance, nodeResult); err != nil {
			return "failed", err
		}
		nextStepIDs, err := r.router.NextSteps(step, nodeResult.ToStepResult())
		if err != nil {
			return "failed", r.failStep(ctx, record, instance, "workflow.next_step_resolution_failed", err.Error())
		}
		if err := r.enqueueNextSteps(ctx, instance, validation, nextStepIDs, nodeResult.Output); err != nil {
			return "failed", err
		}
		if err := r.convergeInstanceState(ctx, instance); err != nil {
			return "failed", err
		}
		return "completed", nil
	default:
		return "failed", r.failStep(ctx, record, instance, "workflow.node_result_status_invalid", status)
	}
}

func (r *Runner) loadDefinition(ctx context.Context, instance *modelworkflow.WorkflowInstance) (*modelworkflow.WorkflowDefinition, *ValidationResult, error) {
	definition, err := r.definitions.GetByUUID(ctx, instance.TenantUUID, instance.DefinitionUUID, &instance.DefinitionVersion)
	if err != nil {
		return nil, nil, err
	}
	steps, err := loadStepGraph(definition.StepGraph)
	if err != nil {
		return nil, nil, err
	}
	validation, err := ValidateStepDefinitions(steps)
	if err != nil {
		return nil, nil, err
	}
	return definition, validation, nil
}

func (r *Runner) completeStep(ctx context.Context, record *modelworkflow.WorkflowStepRecord, instance *modelworkflow.WorkflowInstance, nodeResult NodeResult) error {
	ok, err := r.steps.UpdateStateForAttempt(ctx, record.ID, record.Attempt, "completed", map[string]interface{}{
		"payload_out":    toJSONOrEmpty(nodeResult.Output),
		"awaiting_human": false,
		"error_code":     "",
		"error_message":  "",
		"failure_reason": "",
	})
	if err != nil {
		return err
	}
	if !ok {
		return ErrWorkflowStepStaleAttempt
	}
	if instance != nil {
		r.publishRuntimeEvent(ctx, instance.TenantUUID, instance.UUID, "workflow.step.completed", record.StepID, map[string]any{
			"step_record_id": record.ID,
			"attempt":        record.Attempt,
		})
	}
	return nil
}

func (r *Runner) markStepWaiting(ctx context.Context, record *modelworkflow.WorkflowStepRecord, instance *modelworkflow.WorkflowInstance, nodeResult NodeResult) error {
	ok, err := r.steps.UpdateStateForAttempt(ctx, record.ID, record.Attempt, "waiting", map[string]interface{}{
		"payload_out":    toJSONOrEmpty(nodeResult.Output),
		"awaiting_human": nodeResult.AwaitingHuman || nodeResult.ReviewTaskUUID != uuid.Nil,
		"lease_owner":    "",
		"lease_until":    nil,
	})
	if err != nil {
		return err
	}
	if !ok {
		return ErrWorkflowStepStaleAttempt
	}
	if err := r.instances.UpdateState(ctx, instance.TenantUUID, instance.UUID, "waiting", map[string]interface{}{
		"current_step_id": record.StepID,
		"last_error":      "",
	}); err != nil {
		return err
	}
	r.publishRuntimeEvent(ctx, instance.TenantUUID, instance.UUID, "workflow.step.waiting", record.StepID, map[string]any{
		"step_record_id": record.ID,
		"attempt":        record.Attempt,
	})
	return nil
}

func (r *Runner) markInstanceCompensating(ctx context.Context, record *modelworkflow.WorkflowStepRecord, instance *modelworkflow.WorkflowInstance, nodeResult NodeResult) error {
	if err := r.failStep(ctx, record, instance, firstNonEmpty(nodeResult.ErrorCode, "workflow.node_compensating"), firstNonEmpty(nodeResult.ErrorMessage, "workflow.node_compensating")); err != nil {
		return err
	}
	return r.instances.UpdateState(ctx, instance.TenantUUID, instance.UUID, "compensating", map[string]interface{}{
		"current_step_id": record.StepID,
		"last_error":      firstNonEmpty(nodeResult.ErrorMessage, "workflow.node_compensating"),
	})
}

func (r *Runner) failStep(ctx context.Context, record *modelworkflow.WorkflowStepRecord, instance *modelworkflow.WorkflowInstance, code string, message string) error {
	if code == "" {
		code = "workflow.node_failed"
	}
	if message == "" {
		message = code
	}
	ok, err := r.steps.UpdateStateForAttempt(ctx, record.ID, record.Attempt, "failed", map[string]interface{}{
		"failure_reason": message,
		"error_code":     code,
		"error_message":  message,
		"lease_owner":    "",
		"lease_until":    nil,
	})
	if err != nil {
		return err
	}
	if !ok {
		return ErrWorkflowStepStaleAttempt
	}
	if err := r.instances.UpdateState(ctx, instance.TenantUUID, instance.UUID, "failed", map[string]interface{}{
		"current_step_id": record.StepID,
		"last_error":      message,
		"completed_at":    r.now().UTC(),
	}); err != nil {
		return err
	}
	r.publishRuntimeEvent(ctx, instance.TenantUUID, instance.UUID, "workflow.step.failed", record.StepID, map[string]any{
		"step_record_id": record.ID,
		"attempt":        record.Attempt,
		"error_code":     code,
		"error_message":  message,
	})
	return nil
}

func (r *Runner) enqueueNextSteps(ctx context.Context, instance *modelworkflow.WorkflowInstance, validation *ValidationResult, nextStepIDs []string, payload map[string]any) error {
	now := r.now().UTC()
	for _, stepID := range normalizeStrings(nextStepIDs) {
		step, ok := validation.StepByID(stepID)
		if !ok {
			return fmt.Errorf("%w: %s", ErrWorkflowStepDefinitionNil, stepID)
		}
		if !r.dependenciesCompleted(ctx, instance.UUID, step.DependsOn) {
			continue
		}
		exec, err := r.router.Executor(step.Type)
		if err != nil {
			return err
		}
		subjectType := exec.SubjectType()
		if subjectType == "" {
			subjectType = "system"
		}
		record := &modelworkflow.WorkflowStepRecord{
			InstanceUUID:   instance.UUID,
			StepID:         step.ID,
			Type:           step.Type,
			NodeKind:       step.NodeKind,
			NodeRef:        step.NodeRef,
			SubjectType:    subjectType,
			State:          "queued",
			InputMapping:   toJSONOrEmpty(step.InputMapping),
			OutputMapping:  toJSONOrEmpty(step.OutputMapping),
			PayloadIn:      toJSONOrEmpty(payload),
			ScheduledAt:    now,
			LastTransition: now,
		}
		created, err := r.steps.AppendRecord(ctx, record)
		if err != nil {
			return err
		}
		r.publishRuntimeEvent(ctx, instance.TenantUUID, instance.UUID, "workflow.step.queued", step.ID, map[string]any{
			"step_record_id": created.ID,
		})
	}
	return nil
}

func (r *Runner) dependenciesCompleted(ctx context.Context, instanceUUID uuid.UUID, dependsOn []string) bool {
	if len(dependsOn) == 0 {
		return true
	}
	for _, depID := range normalizeStrings(dependsOn) {
		record, err := r.steps.FindLatestByStep(ctx, instanceUUID, depID)
		if err != nil || record.State != "completed" {
			return false
		}
	}
	return true
}

func (r *Runner) convergeInstanceState(ctx context.Context, instance *modelworkflow.WorkflowInstance) error {
	records, err := r.steps.ListByInstance(ctx, instance.UUID)
	if err != nil {
		return err
	}
	hasActive := false
	hasFailed := false
	currentStepID := ""
	for _, record := range records {
		switch record.State {
		case "queued", "in_progress", "waiting":
			hasActive = true
			if currentStepID == "" {
				currentStepID = record.StepID
			}
		case "failed":
			hasFailed = true
			if currentStepID == "" {
				currentStepID = record.StepID
			}
		}
	}
	if hasFailed {
		if err := r.instances.UpdateState(ctx, instance.TenantUUID, instance.UUID, "failed", map[string]interface{}{
			"current_step_id": currentStepID,
			"completed_at":    r.now().UTC(),
		}); err != nil {
			return err
		}
		r.publishRuntimeEvent(ctx, instance.TenantUUID, instance.UUID, "workflow.instance.failed", currentStepID, nil)
		return nil
	}
	if hasActive {
		if err := r.instances.UpdateState(ctx, instance.TenantUUID, instance.UUID, "running", map[string]interface{}{
			"current_step_id": currentStepID,
			"last_error":      "",
		}); err != nil {
			return err
		}
		r.publishRuntimeEvent(ctx, instance.TenantUUID, instance.UUID, "workflow.instance.running", currentStepID, nil)
		return nil
	}
	if err := r.instances.UpdateState(ctx, instance.TenantUUID, instance.UUID, "succeeded", map[string]interface{}{
		"current_step_id": "",
		"last_error":      "",
		"completed_at":    r.now().UTC(),
	}); err != nil {
		return err
	}
	r.publishRuntimeEvent(ctx, instance.TenantUUID, instance.UUID, "workflow.instance.succeeded", "", nil)
	return nil
}

func (r *Runner) publishRuntimeEvent(ctx context.Context, tenantUUID string, instanceUUID uuid.UUID, eventType string, stepID string, details map[string]any) {
	if r == nil || r.publish == nil {
		return
	}
	r.publish(ctx, tenantUUID, instanceUUID, eventType, stepID, details)
}

func jsonMap(data datatypes.JSON) map[string]any {
	if len(data) == 0 {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil
	}
	return out
}

func applyWorkflowInputMapping(payload map[string]any, mapping map[string]any) (map[string]any, error) {
	if len(mapping) == 0 {
		return cloneMap(payload), nil
	}
	out := make(map[string]any, len(mapping))
	for targetField, sourceSpec := range mapping {
		target := strings.TrimSpace(targetField)
		source := strings.TrimSpace(fmt.Sprint(sourceSpec))
		if target == "" || source == "" {
			return nil, fmt.Errorf("workflow.input_mapping_invalid: %s", targetField)
		}
		value, ok := workflowFieldValue(payload, source)
		if !ok {
			return nil, fmt.Errorf("workflow.input_field_missing: %s", source)
		}
		workflowSetFieldValue(out, target, value)
	}
	return out, nil
}

func applyWorkflowOutputMapping(payload map[string]any, mapping map[string]any) (map[string]any, error) {
	if len(mapping) == 0 {
		return cloneMap(payload), nil
	}
	out := make(map[string]any, len(mapping))
	for sourceField, targetSpec := range mapping {
		source := strings.TrimSpace(sourceField)
		target := strings.TrimSpace(fmt.Sprint(targetSpec))
		if source == "" || target == "" {
			return nil, fmt.Errorf("workflow.output_mapping_invalid: %s", sourceField)
		}
		value, ok := workflowFieldValue(payload, source)
		if !ok {
			return nil, fmt.Errorf("workflow.output_field_missing: %s", source)
		}
		workflowSetFieldValue(out, target, value)
	}
	return out, nil
}

func workflowFieldValue(payload map[string]any, field string) (any, bool) {
	if len(payload) == 0 {
		return nil, false
	}
	parts := strings.Split(field, ".")
	var current any = payload
	for _, part := range parts {
		key := strings.TrimSpace(part)
		if key == "" {
			return nil, false
		}
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = object[key]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func workflowSetFieldValue(payload map[string]any, field string, value any) {
	parts := strings.Split(field, ".")
	current := payload
	for i, raw := range parts {
		key := strings.TrimSpace(raw)
		if key == "" {
			return
		}
		if i == len(parts)-1 {
			current[key] = value
			return
		}
		next, ok := current[key].(map[string]any)
		if !ok {
			next = map[string]any{}
			current[key] = next
		}
		current = next
	}
}
