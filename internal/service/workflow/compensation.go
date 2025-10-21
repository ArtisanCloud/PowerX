package workflow

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	modelworkflow "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/workflow"
)

// triggerCompensation 为失败步骤创建补偿记录并更新实例状态。
func (s *Service) triggerCompensation(ctx context.Context, instance *modelworkflow.WorkflowInstance, step StepDefinition, record *modelworkflow.WorkflowStepRecord, reason string) error {
	if s == nil || s.compensations == nil {
		return errors.New("compensation store unavailable")
	}
	if instance == nil || record == nil {
		return errors.New("invalid compensation context")
	}
	if !step.Compensatable {
		return fmt.Errorf("step %s is not marked compensatable", step.ID)
	}

	comp := &modelworkflow.WorkflowStepCompensation{
		StepRecordID: record.ID,
		State:        "pending",
		Handler:      step.ID,
		InitiatedBy:  "auto",
		Notes:        reason,
	}
	created, err := s.compensations.CreateCompensation(ctx, comp)
	if err != nil {
		return err
	}

	updates := map[string]interface{}{
		"last_error": reason,
	}
	if err := s.instances.UpdateState(ctx, instance.TenantID, instance.UUID, "compensating", updates); err != nil {
		return err
	}

	instanceEvent := newWorkflowEvent(
		instance.TenantID,
		instance.UUID,
		"workflow.instance.compensating",
		fmt.Sprintf("workflow instance %s entering compensation", instance.UUID),
		map[string]any{
			"step_id": step.ID,
			"reason":  reason,
			"mode":    "automatic",
		},
	)
	instanceEvent.RelatedStepRecord = record.ID
	instanceEvent.ActorType = "system"
	s.em.emit(ctx, instanceEvent)

	stepEvent := newWorkflowEvent(
		instance.TenantID,
		instance.UUID,
		"workflow.step.compensation_triggered",
		fmt.Sprintf("compensation triggered for step %s", step.ID),
		map[string]any{
			"step_id":         step.ID,
			"reason":          reason,
			"compensation_id": created.ID,
		},
	)
	stepEvent.RelatedStepRecord = record.ID
	stepEvent.ActorType = "system"
	s.em.emit(ctx, stepEvent)

	return nil
}

// createManualCompensation 允许人工触发指定步骤的补偿。
func (s *Service) createManualCompensation(ctx context.Context, instance *modelworkflow.WorkflowInstance, step StepDefinition, record *modelworkflow.WorkflowStepRecord, operator uuid.UUID, reason string) (*modelworkflow.WorkflowStepCompensation, error) {
	if s == nil || s.compensations == nil {
		return nil, errors.New("compensation store unavailable")
	}
	comp := &modelworkflow.WorkflowStepCompensation{
		StepRecordID: record.ID,
		State:        "pending",
		Handler:      step.ID,
		InitiatedBy:  "operator",
		Notes:        reason,
	}
	if operator != uuid.Nil {
		comp.Notes = fmt.Sprintf("%s (operator=%s)", reason, operator.String())
	}
	created, err := s.compensations.CreateCompensation(ctx, comp)
	if err != nil {
		return nil, err
	}
	if err := s.instances.UpdateState(ctx, instance.TenantID, instance.UUID, "compensating", map[string]interface{}{"last_error": reason}); err != nil {
		return nil, err
	}

	instanceEvent := newWorkflowEvent(
		instance.TenantID,
		instance.UUID,
		"workflow.instance.compensating",
		fmt.Sprintf("workflow instance %s entering compensation", instance.UUID),
		map[string]any{
			"step_id": step.ID,
			"reason":  reason,
			"mode":    "manual",
		},
	)
	instanceEvent.RelatedStepRecord = record.ID
	if operator != uuid.Nil {
		instanceEvent.ActorType = "operator"
		instanceEvent.ActorID = operator.String()
	} else {
		instanceEvent.ActorType = "system"
	}
	s.em.emit(ctx, instanceEvent)

	stepEvent := newWorkflowEvent(
		instance.TenantID,
		instance.UUID,
		"workflow.step.compensation_requested",
		fmt.Sprintf("manual compensation requested for step %s", step.ID),
		map[string]any{
			"step_id":         step.ID,
			"reason":          reason,
			"compensation_id": created.ID,
		},
	)
	stepEvent.RelatedStepRecord = record.ID
	if operator != uuid.Nil {
		stepEvent.ActorType = "operator"
		stepEvent.ActorID = operator.String()
	} else {
		stepEvent.ActorType = "system"
	}
	s.em.emit(ctx, stepEvent)

	return created, nil
}

// completeCompensation 标记补偿完成。
func (s *Service) completeCompensation(ctx context.Context, instance *modelworkflow.WorkflowInstance, comp *modelworkflow.WorkflowStepCompensation, success bool, notes string) error {
	if s == nil || s.compensations == nil || comp == nil {
		return errors.New("compensation store unavailable")
	}
	state := "completed"
	if !success {
		state = "failed"
	}
	updates := map[string]interface{}{
		"notes": notes,
	}
	if success {
		updates["completed_at"] = time.Now().UTC()
	}
	if err := s.compensations.UpdateState(ctx, comp.ID, state, updates); err != nil {
		return err
	}
	if success {
		if err := s.instances.UpdateState(ctx, instance.TenantID, instance.UUID, "compensated", nil); err != nil {
			return err
		}
	} else {
		if err := s.instances.UpdateState(ctx, instance.TenantID, instance.UUID, "compensation_failed", map[string]interface{}{"last_error": notes}); err != nil {
			return err
		}
	}
	if s.metrics != nil {
		s.metrics.ObserveCompensationResult(ctx, instance.TenantID, comp.Handler, success)
	}

	eventType := "workflow.step.compensation_completed"
	if !success {
		eventType = "workflow.step.compensation_failed"
	}
	s.emitStepCompensationResult(ctx, instance, comp, eventType, notes, success)
	return nil
}

func (s *Service) emitStepCompensationResult(ctx context.Context, instance *modelworkflow.WorkflowInstance, comp *modelworkflow.WorkflowStepCompensation, eventType, notes string, success bool) {
	if s == nil || s.em == nil || instance == nil || comp == nil {
		return
	}
	payload := map[string]any{
		"compensation_id": comp.ID,
		"step_record_id":  comp.StepRecordID,
		"success":         success,
	}
	if notes != "" {
		payload["notes"] = notes
	}
	event := newWorkflowEvent(
		instance.TenantID,
		instance.UUID,
		eventType,
		fmt.Sprintf("compensation %s for step record %d", ternary(success, "completed", "failed"), comp.StepRecordID),
		payload,
	)
	event.RelatedStepRecord = comp.StepRecordID
	event.ActorType = "system"
	s.em.emit(ctx, event)
}

func ternary[T any](cond bool, a, b T) T {
	if cond {
		return a
	}
	return b
}
