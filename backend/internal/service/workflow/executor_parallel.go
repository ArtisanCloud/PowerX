package workflow

import "fmt"

type parallelExecutor struct{}

func (p *parallelExecutor) Type() string {
	return "parallel"
}

func (p *parallelExecutor) SubjectType() string {
	return "system"
}

func (p *parallelExecutor) Validate(step StepDefinition) error {
	if len(step.NextStepIDs) < 2 {
		return fmt.Errorf("parallel step %s requires at least two next_step_ids", step.ID)
	}
	return nil
}

func (p *parallelExecutor) Next(step StepDefinition, result StepResult) ([]string, error) {
	if len(result.SelectedBranches) > 0 {
		selected := normalizeStrings(result.SelectedBranches)
		if len(selected) == 0 {
			return nil, fmt.Errorf("parallel step %s selected branches empty", step.ID)
		}
		allowed := make(map[string]struct{}, len(step.NextStepIDs))
		for _, branch := range step.NextStepIDs {
			allowed[branch] = struct{}{}
		}
		for _, branch := range selected {
			if _, ok := allowed[branch]; !ok {
				return nil, fmt.Errorf("parallel step %s selected unknown branch %s", step.ID, branch)
			}
		}
		return selected, nil
	}
	return cloneStrings(step.NextStepIDs), nil
}
