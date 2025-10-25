package workflow

import (
	"fmt"
	"strings"
	"sync"
)

// StepResult 描述单个步骤执行后的结果，用于决定路由与后续步骤。
type StepResult struct {
	Status           string
	Decision         string
	Output           map[string]any
	SelectedBranches []string
	Approved         bool
	HasApproval      bool
}

// StepExecutor 定义不同类型步骤的行为约束。
type StepExecutor interface {
	Type() string
	SubjectType() string
	Validate(step StepDefinition) error
	Next(step StepDefinition, result StepResult) ([]string, error)
}

// ExecutorRouter 注册并分发步骤执行器。
type ExecutorRouter struct {
	mu        sync.RWMutex
	executors map[string]StepExecutor
}

// NewExecutorRouter 创建包含内建执行器的新路由器。
func NewExecutorRouter() *ExecutorRouter {
	router := &ExecutorRouter{
		executors: map[string]StepExecutor{},
	}
	for _, exec := range builtinExecutors() {
		router.Register(exec)
	}
	return router
}

// DefaultExecutorRouter 返回全局内建执行器路由实例。
func DefaultExecutorRouter() *ExecutorRouter {
	defaultRouterOnce.Do(func() {
		defaultRouter = NewExecutorRouter()
	})
	return defaultRouter
}

// Register 将执行器注册到路由器。
func (r *ExecutorRouter) Register(exec StepExecutor) {
	if exec == nil {
		return
	}
	key := strings.ToLower(strings.TrimSpace(exec.Type()))
	if key == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.executors[key] = exec
}

// Executor 返回指定类型的执行器。
func (r *ExecutorRouter) Executor(stepType string) (StepExecutor, error) {
	key := strings.ToLower(strings.TrimSpace(stepType))
	if key == "" {
		return nil, fmt.Errorf("workflow executor: step type is empty")
	}
	r.mu.RLock()
	exec, ok := r.executors[key]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("workflow executor: unsupported step type %s", stepType)
	}
	return exec, nil
}

// Validate 使用对应执行器验证步骤配置。
func (r *ExecutorRouter) Validate(step StepDefinition) error {
	exec, err := r.Executor(step.Type)
	if err != nil {
		return err
	}
	return exec.Validate(step)
}

// NextSteps 根据执行结果计算后续步骤 ID。
func (r *ExecutorRouter) NextSteps(step StepDefinition, result StepResult) ([]string, error) {
	exec, err := r.Executor(step.Type)
	if err != nil {
		return nil, err
	}
	return exec.Next(step, result)
}

var (
	defaultRouter     *ExecutorRouter
	defaultRouterOnce sync.Once
)

func builtinExecutors() []StepExecutor {
	return []StepExecutor{
		&agentExecutor{},
		&systemExecutor{},
		&decisionExecutor{},
		&parallelExecutor{},
		&humanApprovalExecutor{},
		&compensationExecutor{},
	}
}

func cloneStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, len(values))
	copy(out, values)
	return out
}

func normalizeStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, v := range values {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			if _, ok := set[trimmed]; ok {
				continue
			}
			set[trimmed] = struct{}{}
			out = append(out, trimmed)
		}
	}
	return out
}
