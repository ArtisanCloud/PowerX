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
	for _, key := range []string{"review_type", "approver_policy", "review_payload_path", "approved_route", "rejected_route"} {
		if _, ok := step.Config[key]; !ok {
			return fmt.Errorf("human_approval step %s requires config.%s", step.ID, key)
		}
	}
	if configString(step.Config, "review_type") == "" {
		return fmt.Errorf("human_approval step %s requires non-empty review_type", step.ID)
	}
	if configString(step.Config, "review_payload_path") == "" {
		return fmt.Errorf("human_approval step %s requires non-empty review_payload_path", step.ID)
	}
	if _, err := singleRoute(step.Config["approved_route"]); err != nil {
		return fmt.Errorf("human_approval step %s approved_route: %w", step.ID, err)
	}
	if _, err := singleRoute(step.Config["rejected_route"]); err != nil {
		return fmt.Errorf("human_approval step %s rejected_route: %w", step.ID, err)
	}
	return nil
}

func (h *humanApprovalExecutor) Next(step StepDefinition, result StepResult) ([]string, error) {
	if !result.HasApproval {
		return nil, fmt.Errorf("human_approval step %s missing approval result", step.ID)
	}
	if result.Approved {
		target, err := singleRoute(step.Config["approved_route"])
		if err != nil {
			return nil, err
		}
		return []string{target}, nil
	}

	if step.Config != nil {
		target, err := singleRoute(step.Config["rejected_route"])
		if err != nil {
			return nil, err
		}
		return []string{target}, nil
	}
	return nil, fmt.Errorf("human_approval step %s rejected without rejected_route", step.ID)
}

func singleRoute(value any) (string, error) {
	switch typed := value.(type) {
	case string:
		if route := configString(map[string]any{"route": typed}, "route"); route != "" {
			return route, nil
		}
	case []string:
		if len(typed) == 1 && typed[0] != "" {
			return typed[0], nil
		}
	case []any:
		if len(typed) == 1 {
			if route, ok := typed[0].(string); ok && route != "" {
				return route, nil
			}
		}
	}
	return "", fmt.Errorf("route must contain exactly one target")
}
