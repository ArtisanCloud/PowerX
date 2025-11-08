package workflow

import (
	"fmt"
)

type humanApprovalExecutor struct{}

func (h *humanApprovalExecutor) Type() string {
	return "human_approval"
}

func (h *humanApprovalExecutor) SubjectType() string {
	return "human"
}

func (h *humanApprovalExecutor) Validate(step StepDefinition) error {
	if step.Config == nil {
		return fmt.Errorf("human_approval step %s requires config", step.ID)
	}
	approvers, err := asStringSlice(step.Config["approvers"])
	if err != nil || len(approvers) == 0 {
		return fmt.Errorf("human_approval step %s requires non-empty approvers", step.ID)
	}
	if reject, ok := step.Config["on_reject"]; ok && reject != nil {
		if _, err := asStringSlice(reject); err != nil {
			return fmt.Errorf("human_approval step %s on_reject: %w", step.ID, err)
		}
	}
	return nil
}

func (h *humanApprovalExecutor) Next(step StepDefinition, result StepResult) ([]string, error) {
	if !result.HasApproval {
		return nil, fmt.Errorf("human_approval step %s missing approval result", step.ID)
	}
	if result.Approved {
		return cloneStrings(step.NextStepIDs), nil
	}

	if step.Config != nil {
		if reject, ok := step.Config["on_reject"]; ok {
			targets, err := asStringSlice(reject)
			if err != nil {
				return nil, err
			}
			targets = normalizeStrings(targets)
			if len(targets) > 0 {
				return targets, nil
			}
		}
	}
	if len(step.NextStepIDs) > 0 {
		return cloneStrings(step.NextStepIDs), nil
	}
	return nil, fmt.Errorf("human_approval step %s rejected without fallback route", step.ID)
}
