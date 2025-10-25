package workflow

type compensationExecutor struct{}

func (c *compensationExecutor) Type() string {
	return "compensation"
}

func (c *compensationExecutor) SubjectType() string {
	return "system"
}

func (c *compensationExecutor) Validate(step StepDefinition) error {
	return nil
}

func (c *compensationExecutor) Next(step StepDefinition, _ StepResult) ([]string, error) {
	return cloneStrings(step.NextStepIDs), nil
}
