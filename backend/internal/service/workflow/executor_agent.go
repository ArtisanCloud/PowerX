package workflow

type agentExecutor struct{}

func (a *agentExecutor) Type() string {
	return "agent"
}

func (a *agentExecutor) SubjectType() string {
	return "agent"
}

func (a *agentExecutor) Validate(step StepDefinition) error {
	return nil
}

func (a *agentExecutor) Next(step StepDefinition, _ StepResult) ([]string, error) {
	return cloneStrings(step.NextStepIDs), nil
}
