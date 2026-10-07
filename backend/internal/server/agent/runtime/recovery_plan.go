package runtime

import (
	"context"
	"fmt"
	"strings"

	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/google/uuid"
)

// semanticReplacementPlanFromVerdict turns a verifier-selected alternative
// into a persisted observation before it changes executable work. The reader
// verifies the frozen primary contract; SemanticReplanner verifies the frozen
// replacement grant. Neither layer reads provider text or expands the plan.
func semanticReplacementPlanFromVerdict(ctx context.Context, controller *PlanController, planner PlanRevisionPlanner, snapshot *ResourceSnapshot, current flowschema.ExecutionPlan, verdict VerificationVerdict, env string) (string, flowschema.ExecutionPlan, ResourceObservation, error) {
	if controller == nil || planner == nil || snapshot == nil || verdict.Class != VerificationReplaceable || strings.TrimSpace(env) == "" {
		return "", flowschema.ExecutionPlan{}, ResourceObservation{}, fmt.Errorf("semantic replacement input is incomplete")
	}
	recoveryPlan, err := recoverySubplan(current, verdict.TaskRefs)
	if err != nil {
		return "", flowschema.ExecutionPlan{}, ResourceObservation{}, err
	}
	var primaryUUID uuid.UUID
	found := false
	for _, task := range recoveryPlan.Tasks {
		if task.TaskID != verdict.ReplacementTaskRef {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(task.NodeKind), "tooling") || task.Params == nil {
			return "", flowschema.ExecutionPlan{}, ResourceObservation{}, fmt.Errorf("replacement task must be a tooling task with capability_uuid")
		}
		primaryUUID, err = uuid.Parse(strings.TrimSpace(anyToString(task.Params["capability_uuid"])))
		if err != nil || primaryUUID == uuid.Nil {
			return "", flowschema.ExecutionPlan{}, ResourceObservation{}, fmt.Errorf("replacement task capability_uuid is required")
		}
		found = true
		break
	}
	if !found {
		return "", flowschema.ExecutionPlan{}, ResourceObservation{}, fmt.Errorf("replacement task is outside recovery sub-plan")
	}
	observeCtx := ContextWithReplacementDirective(ctx, verdict.ReplacementTaskRef, verdict.ReplacementCapabilityUUID)
	observation, err := controller.Observe(observeCtx, env, primaryUUID, capabilityReplacementObservationPurpose)
	if err != nil {
		return "", flowschema.ExecutionPlan{}, ResourceObservation{}, fmt.Errorf("observe replacement contract: %w", err)
	}
	if observation.ReplanDirective == nil || observation.ReplanDirective.Action != "replace_authorized_capability" {
		return "", flowschema.ExecutionPlan{}, ResourceObservation{}, fmt.Errorf("replacement observation did not produce a semantic directive")
	}
	reasonCode, next, err := planner.Replan(observeCtx, PlanRevisionRequest{Snapshot: snapshot, Observations: []ResourceObservation{observation}, CurrentPlan: recoveryPlan})
	if err != nil {
		return "", flowschema.ExecutionPlan{}, ResourceObservation{}, fmt.Errorf("semantic replacement replan: %w", err)
	}
	return reasonCode, next, observation, nil
}

// retryPlanFromVerdict produces a closed sub-plan from task IDs explicitly
// named by a verification contract. It never retries an implicit task or adds
// a new capability to the frozen plan.
func retryPlanFromVerdict(snapshot *ResourceSnapshot, current flowschema.ExecutionPlan, verdict VerificationVerdict) (flowschema.ExecutionPlan, error) {
	if snapshot == nil || verdict.Class != VerificationRetryable || strings.TrimSpace(current.PlanID) == "" || len(verdict.TaskRefs) == 0 {
		return flowschema.ExecutionPlan{}, fmt.Errorf("retry plan input is incomplete")
	}
	next, err := recoverySubplan(current, verdict.TaskRefs)
	if err != nil {
		return flowschema.ExecutionPlan{}, err
	}
	for _, task := range next.Tasks {
		if err := requireRetryContract(snapshot, task); err != nil {
			return flowschema.ExecutionPlan{}, err
		}
	}
	next.PlanID = current.PlanID + "_retry"
	return next, nil
}

func replacementPlanFromVerdict(snapshot *ResourceSnapshot, current flowschema.ExecutionPlan, verdict VerificationVerdict) (flowschema.ExecutionPlan, error) {
	if snapshot == nil || verdict.Class != VerificationReplaceable || verdict.ReplacementCapabilityUUID == uuid.Nil || strings.TrimSpace(verdict.ReplacementTaskRef) == "" {
		return flowschema.ExecutionPlan{}, fmt.Errorf("replacement plan input is incomplete")
	}
	replacement, err := snapshot.RequireInvocable(verdict.ReplacementCapabilityUUID)
	if err != nil || replacement.Kind != ResourceKindCapability || strings.TrimSpace(replacement.CapabilityID) == "" {
		return flowschema.ExecutionPlan{}, fmt.Errorf("replacement capability is not invocable in frozen snapshot")
	}
	next, err := recoverySubplan(current, verdict.TaskRefs)
	if err != nil {
		return flowschema.ExecutionPlan{}, err
	}
	for index := range next.Tasks {
		task := &next.Tasks[index]
		if task.TaskID != verdict.ReplacementTaskRef {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(task.NodeKind), "tooling") || task.Params == nil {
			return flowschema.ExecutionPlan{}, fmt.Errorf("replacement task must be a tooling task with capability_uuid")
		}
		currentUUID, parseErr := uuid.Parse(strings.TrimSpace(anyToString(task.Params["capability_uuid"])))
		if parseErr != nil || currentUUID == uuid.Nil {
			return flowschema.ExecutionPlan{}, fmt.Errorf("replacement task capability_uuid is required")
		}
		currentCapability, err := snapshot.RequireInvocable(currentUUID)
		if err != nil {
			return flowschema.ExecutionPlan{}, fmt.Errorf("replacement task capability is not invocable in frozen snapshot")
		}
		allowed := false
		for _, alternativeUUID := range currentCapability.RuntimeContract.AlternativeCapabilityUUIDs {
			if alternativeUUID == replacement.CapabilityUUID {
				allowed = true
				break
			}
		}
		if !allowed {
			return flowschema.ExecutionPlan{}, fmt.Errorf("replacement capability is not declared by the frozen capability contract")
		}
		task.Params["capability_uuid"] = replacement.CapabilityUUID.String()
		task.Params["capability_id"] = replacement.CapabilityID
		task.NodeRef = replacement.CapabilityID
		next.PlanID = current.PlanID + "_replacement"
		return next, nil
	}
	return flowschema.ExecutionPlan{}, fmt.Errorf("replacement task is outside recovery sub-plan")
}

func requireRetryContract(snapshot *ResourceSnapshot, task flowschema.PlanTask) error {
	if !strings.EqualFold(strings.TrimSpace(task.NodeKind), "tooling") || task.Params == nil {
		return fmt.Errorf("retry task must be a tooling task with capability_uuid")
	}
	capabilityUUID, err := uuid.Parse(strings.TrimSpace(anyToString(task.Params["capability_uuid"])))
	if err != nil || capabilityUUID == uuid.Nil {
		return fmt.Errorf("retry task capability_uuid is required")
	}
	capability, err := snapshot.RequireInvocable(capabilityUUID)
	if err != nil || capability.RuntimeContract.RetryMaxAttempts < 1 {
		return fmt.Errorf("retry is not declared by the frozen capability contract")
	}
	return nil
}

func recoverySubplan(current flowschema.ExecutionPlan, taskRefs []string) (flowschema.ExecutionPlan, error) {
	if strings.TrimSpace(current.PlanID) == "" || len(taskRefs) == 0 {
		return flowschema.ExecutionPlan{}, fmt.Errorf("recovery plan input is incomplete")
	}
	allowed := make(map[string]flowschema.PlanTask, len(current.Tasks))
	for _, task := range current.Tasks {
		if strings.TrimSpace(task.TaskID) == "" {
			return flowschema.ExecutionPlan{}, fmt.Errorf("current plan task_id is required")
		}
		allowed[task.TaskID] = task
	}
	selected := make(map[string]struct{}, len(taskRefs))
	for _, ref := range taskRefs {
		if _, ok := allowed[ref]; !ok {
			return flowschema.ExecutionPlan{}, fmt.Errorf("recovery task is outside current plan")
		}
		selected[ref] = struct{}{}
	}
	for ref := range selected {
		for _, dependency := range allowed[ref].DependsOn {
			if _, ok := selected[dependency]; !ok {
				return flowschema.ExecutionPlan{}, fmt.Errorf("recovery task dependencies must be declared in recovery_task_refs")
			}
		}
	}
	next := flowschema.ExecutionPlan{PlanID: current.PlanID, Tasks: make([]flowschema.PlanTask, 0, len(selected))}
	for _, task := range current.Tasks {
		if _, ok := selected[task.TaskID]; ok {
			next.Tasks = append(next.Tasks, cloneExecutionPlan(flowschema.ExecutionPlan{Tasks: []flowschema.PlanTask{task}}).Tasks[0])
		}
	}
	return next, nil
}
