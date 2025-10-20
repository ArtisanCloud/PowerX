package workflow

type systemExecutor struct{}

func (s *systemExecutor) Type() string {
	return "system"
}

func (s *systemExecutor) SubjectType() string {
	return "system"
}

func (s *systemExecutor) Validate(step StepDefinition) error {
	// 当前阶段允许空配置，后续可在此扩展任务参数校验。
	return nil
}

func (s *systemExecutor) Next(step StepDefinition, _ StepResult) ([]string, error) {
	return cloneStrings(step.NextStepIDs), nil
}
