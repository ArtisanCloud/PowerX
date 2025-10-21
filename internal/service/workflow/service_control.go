package workflow

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	modelworkflow "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/workflow"
)

// StepFailureInput 描述步骤失败的上下文。
type StepFailureInput struct {
	TenantID     uint64
	InstanceUUID uuid.UUID
	StepRecordID uint64
	StepID       string
	Reason       string
}

// StepFailureResult 返回失败处理后的结果。
type StepFailureResult struct {
	RetryScheduled        bool
	CompensationTriggered bool
	NextAttempt           int
}

// ControlInstance 执行暂停、恢复、取消与步骤控制等操作，返回更新后的实例。
func (s *Service) ControlInstance(ctx context.Context, input ControlInstanceInput) (*modelworkflow.WorkflowInstance, error) {
	if s == nil {
		return nil, errors.New("workflow service unavailable")
	}
	if input.TenantID == 0 {
		return nil, errors.New("tenant_id is required")
	}
	if input.InstanceUUID == uuid.Nil {
		return nil, errors.New("instance uuid is required")
	}

	instance, err := s.instances.GetByUUID(ctx, input.TenantID, input.InstanceUUID)
	if err != nil {
		return nil, err
	}

	action := strings.ToLower(strings.TrimSpace(input.Action))
	switch action {
	case "pause":
		if err := s.instances.UpdateState(ctx, instance.TenantID, instance.UUID, "suspended", map[string]interface{}{"last_error": strings.TrimSpace(input.Reason)}); err != nil {
			return nil, err
		}
	case "resume":
		if err := s.instances.UpdateState(ctx, instance.TenantID, instance.UUID, "running", map[string]interface{}{"last_error": ""}); err != nil {
			return nil, err
		}
	case "cancel":
		if err := s.instances.UpdateState(ctx, instance.TenantID, instance.UUID, "canceled", map[string]interface{}{"last_error": strings.TrimSpace(input.Reason)}); err != nil {
			return nil, err
		}
	case "retry_step":
		if input.StepID == "" {
			return nil, errors.New("step_id is required for retry_step")
		}
		if err := s.manualRetryStep(ctx, instance, input.StepID, input.AssignmentID, input.Payload); err != nil {
			return nil, err
		}
	case "trigger_compensation":
		if input.StepID == "" {
			return nil, errors.New("step_id is required for trigger_compensation")
		}
		if err := s.manualTriggerCompensation(ctx, instance, input.StepID, input.Operator, input.Reason); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported control action %s", input.Action)
	}

	updated, err := s.instances.GetByUUID(ctx, input.TenantID, input.InstanceUUID)
	if err != nil {
		return nil, err
	}
	return updated, nil
}

// HandleStepFailure 处理执行失败的步骤，决定重试或进入补偿。
func (s *Service) HandleStepFailure(ctx context.Context, input StepFailureInput) (StepFailureResult, error) {
	result := StepFailureResult{}
	if s == nil {
		return result, errors.New("workflow service unavailable")
	}
	if input.TenantID == 0 || input.InstanceUUID == uuid.Nil || input.StepRecordID == 0 || strings.TrimSpace(input.StepID) == "" {
		return result, errors.New("invalid failure input")
	}

	instance, err := s.instances.GetByUUID(ctx, input.TenantID, input.InstanceUUID)
	if err != nil {
		return result, err
	}

	record, err := s.steps.GetByID(ctx, input.StepRecordID)
	if err != nil {
		return result, err
	}
	if record.InstanceUUID != instance.UUID {
		return result, fmt.Errorf("step %d does not belong to instance %s", record.ID, instance.UUID)
	}

	definition, validation, err := s.loadDefinitionContext(ctx, instance)
	if err != nil {
		return result, err
	}
	stepDef, ok := validation.StepByID(input.StepID)
	if !ok {
		return result, fmt.Errorf("step %s not found in definition", input.StepID)
	}

	attempt := int(record.Attempt)
	if attempt < 0 {
		attempt = 0
	}
	attempt++

	updates := map[string]interface{}{
		"attempt":        attempt,
		"failure_reason": strings.TrimSpace(input.Reason),
		"completed_at":   time.Now().UTC(),
	}
	if err := s.steps.UpdateState(ctx, record.ID, "failed", updates); err != nil {
		return result, err
	}

	policy := decodeRetryPolicy(definition.DefaultRetryPolicy)
	maxAttempts := policy.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 3
	}

	if attempt < maxAttempts {
		if s.scheduler == nil {
			return result, errors.New("scheduler unavailable for retry")
		}
		nextAttempt := attempt + 1
		delay := s.scheduler.NextDelay(policy, attempt)
		metadata := map[string]string{
			"step_id": input.StepID,
			"attempt": strconv.Itoa(nextAttempt),
		}
		_, err := s.scheduler.ScheduleRetry(ctx, RetryScheduleOptions{
			TenantKey:    tenantKey(instance.TenantID),
			WorkerID:     "workflow-runtime",
			InstanceUUID: instance.UUID,
			StepRecordID: record.ID,
			Attempt:      nextAttempt,
			Delay:        delay,
			Metadata:     metadata,
		})
		if err != nil {
			return result, err
		}
		if err := s.instances.UpdateState(ctx, instance.TenantID, instance.UUID, "waiting", map[string]interface{}{"last_error": strings.TrimSpace(input.Reason)}); err != nil {
			return result, err
		}
		if s.metrics != nil {
			s.metrics.ObserveRetryScheduled(ctx, instance.TenantID, stepDef.Type)
		}
		result.RetryScheduled = true
		result.NextAttempt = nextAttempt
		return result, nil
	}

	// 尝试补偿
	if !stepDef.Compensatable {
		return result, fmt.Errorf("step %s exhausted retries and is not compensatable", stepDef.ID)
	}
	if err := s.triggerCompensation(ctx, instance, stepDef, record, strings.TrimSpace(input.Reason)); err != nil {
		return result, err
	}
	if s.metrics != nil {
		s.metrics.ObserveCompensationTriggered(ctx, instance.TenantID, stepDef.Type)
	}
	result.CompensationTriggered = true
	return result, nil
}

func (s *Service) manualRetryStep(ctx context.Context, instance *modelworkflow.WorkflowInstance, stepID string, assignmentID uint64, payload map[string]any) error {
	_ = assignmentID
	_, validation, err := s.loadDefinitionContext(ctx, instance)
	if err != nil {
		return err
	}
	stepDef, ok := validation.StepByID(stepID)
	if !ok {
		return fmt.Errorf("step %s not found", stepID)
	}

	now := s.now().UTC()
	retryRecord := &modelworkflow.WorkflowStepRecord{
		InstanceUUID:   instance.UUID,
		StepID:         stepDef.ID,
		Type:           stepDef.Type,
		State:          "queued",
		SubjectType:    strings.ToLower(stepDef.Type),
		PayloadIn:      toJSONOrEmpty(payload),
		ScheduledAt:    now,
		LastTransition: now,
	}
	if retryRecord.SubjectType != "agent" && retryRecord.SubjectType != "human" {
		retryRecord.SubjectType = "system"
	}
	if step, err := s.steps.AppendRecord(ctx, retryRecord); err != nil {
		return err
	} else if retryRecord.SubjectType == "agent" && s.tracker != nil {
		capability := ""
		if stepDef.Config != nil {
			if v, ok := stepDef.Config["capability"].(string); ok {
				capability = v
			}
		}
		agentID := uuid.Nil
		if stepDef.Config != nil {
			if v, ok := stepDef.Config["agent_id"].(string); ok {
				if parsed, err := uuid.Parse(strings.TrimSpace(v)); err == nil {
					agentID = parsed
				}
			}
		}
		if agentID != uuid.Nil {
			_, err := s.tracker.Dispatch(ctx, AssignmentDispatchInput{
				TenantID:     instance.TenantID,
				InstanceUUID: instance.UUID,
				StepRecordID: step.ID,
				StepID:       step.StepID,
				AgentUUID:    agentID,
				Capability:   capability,
			})
			if err != nil {
				return err
			}
		}
	}

	return s.instances.UpdateState(ctx, instance.TenantID, instance.UUID, "running", map[string]interface{}{"current_step_id": stepID})
}

func (s *Service) manualTriggerCompensation(ctx context.Context, instance *modelworkflow.WorkflowInstance, stepID string, operator uuid.UUID, reason string) error {
	_, validation, err := s.loadDefinitionContext(ctx, instance)
	if err != nil {
		return err
	}
	stepDef, ok := validation.StepByID(stepID)
	if !ok {
		return fmt.Errorf("step %s not found", stepID)
	}
	records, err := s.steps.ListByInstance(ctx, instance.UUID)
	if err != nil {
		return err
	}
	var target *modelworkflow.WorkflowStepRecord
	for i := len(records) - 1; i >= 0; i-- {
		if records[i].StepID == stepID {
			target = &records[i]
			break
		}
	}
	if target == nil {
		return fmt.Errorf("step %s has no execution record", stepID)
	}
	_, err = s.createManualCompensation(ctx, instance, stepDef, target, operator, reason)
	return err
}

func (s *Service) loadDefinitionContext(ctx context.Context, instance *modelworkflow.WorkflowInstance) (*modelworkflow.WorkflowDefinition, *ValidationResult, error) {
	definition, err := s.definitions.GetByUUID(ctx, instance.TenantID, instance.DefinitionUUID, &instance.DefinitionVersion)
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

func tenantKey(tenantID uint64) string {
	return fmt.Sprintf("tenant:%d", tenantID)
}
