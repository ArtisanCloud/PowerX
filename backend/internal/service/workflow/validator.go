package workflow

import (
	"errors"
	"fmt"
	"strings"
)

// Allowed step types for orchestration engine.
var allowedStepTypes = map[string]struct{}{
	"agent":          {},
	"system":         {},
	"decision":       {},
	"parallel":       {},
	"human_approval": {},
	"compensation":   {},
}

// StepDefinition 描述工作流定义中的单个步骤。
type StepDefinition struct {
	ID            string                 `json:"id" yaml:"id"`
	DisplayName   string                 `json:"display_name,omitempty" yaml:"display_name,omitempty"`
	Type          string                 `json:"type" yaml:"type"`
	NodeKind      string                 `json:"node_kind,omitempty" yaml:"node_kind,omitempty"`
	NodeRef       string                 `json:"node_ref,omitempty" yaml:"node_ref,omitempty"`
	InputMapping  map[string]any         `json:"input_mapping,omitempty" yaml:"input_mapping,omitempty"`
	OutputMapping map[string]any         `json:"output_mapping,omitempty" yaml:"output_mapping,omitempty"`
	Config        map[string]any         `json:"config,omitempty" yaml:"config,omitempty"`
	NextStepIDs   []string               `json:"next_step_ids,omitempty" yaml:"next_step_ids,omitempty"`
	DependsOn     []string               `json:"depends_on,omitempty" yaml:"depends_on,omitempty"`
	Compensatable bool                   `json:"compensatable,omitempty" yaml:"compensatable,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

// ValidationResult 返回校验后便于执行所需的派生数据。
type ValidationResult struct {
	Steps            []StepDefinition
	StartStepIDs     []string
	AllStepIDs       map[string]struct{}
	Adjacency        map[string][]string
	ReverseAdjacency map[string][]string
	HasCompensatable bool
	StepIndex        map[string]StepDefinition
}

var (
	errEmptySteps      = errors.New("step graph must contain at least one step")
	errMissingStepID   = errors.New("step id is required")
	errDuplicateStepID = errors.New("step id must be unique")
	errInvalidStepType = errors.New("invalid step type")
	errNoEntryStep     = errors.New("no entry step detected (steps without depends_on)")
	errCycleDetected   = errors.New("step graph contains cycle")
)

// ValidateStepDefinitions 检查步骤配置是否满足编排执行要求，并返回标准化结果。
func ValidateStepDefinitions(rawSteps []StepDefinition) (*ValidationResult, error) {
	if len(rawSteps) == 0 {
		return nil, errEmptySteps
	}

	router := DefaultExecutorRouter()

	steps := make([]StepDefinition, 0, len(rawSteps))
	stepMap := make(map[string]StepDefinition, len(rawSteps))
	idSet := make(map[string]struct{}, len(rawSteps))

	for _, step := range rawSteps {
		step.ID = strings.TrimSpace(step.ID)
		if step.ID == "" {
			return nil, errMissingStepID
		}
		if _, exists := idSet[step.ID]; exists {
			return nil, fmt.Errorf("%w: %s", errDuplicateStepID, step.ID)
		}

		step.Type = strings.TrimSpace(strings.ToLower(step.Type))
		if _, ok := allowedStepTypes[step.Type]; !ok {
			return nil, fmt.Errorf("%w: %s", errInvalidStepType, step.Type)
		}

		idSet[step.ID] = struct{}{}
		normalized := normalizeStep(step)
		if err := router.Validate(normalized); err != nil {
			return nil, fmt.Errorf("step %s: %w", step.ID, err)
		}
		stepMap[step.ID] = normalized
		steps = append(steps, normalized)
	}

	adj := make(map[string][]string, len(steps))
	reverse := make(map[string][]string, len(steps))

	for _, step := range steps {
		for _, nextID := range step.NextStepIDs {
			if _, ok := idSet[nextID]; !ok {
				return nil, fmt.Errorf("next_step_ids references unknown step: %s -> %s", step.ID, nextID)
			}
			adj[step.ID] = append(adj[step.ID], nextID)
			reverse[nextID] = append(reverse[nextID], step.ID)
		}
		for _, depID := range step.DependsOn {
			if _, ok := idSet[depID]; !ok {
				return nil, fmt.Errorf("depends_on references unknown step: %s -> %s", step.ID, depID)
			}
			reverse[step.ID] = appendUnique(reverse[step.ID], depID)
			adj[depID] = appendUnique(adj[depID], step.ID)
		}
	}

	if hasCycle(adj, idSet) {
		return nil, errCycleDetected
	}

	startSteps := make([]string, 0)
	for _, step := range steps {
		if len(reverse[step.ID]) == 0 {
			startSteps = append(startSteps, step.ID)
		}
	}
	if len(startSteps) == 0 {
		return nil, errNoEntryStep
	}

	hasComp := false
	for _, step := range steps {
		if step.Compensatable {
			hasComp = true
			break
		}
	}

	return &ValidationResult{
		Steps:            steps,
		StartStepIDs:     startSteps,
		Adjacency:        adj,
		ReverseAdjacency: reverse,
		AllStepIDs:       idSet,
		HasCompensatable: hasComp,
		StepIndex:        stepMap,
	}, nil
}

// StepByID 根据步骤 ID 返回定义及其存在性标记。
func (r *ValidationResult) StepByID(stepID string) (StepDefinition, bool) {
	if r == nil || r.StepIndex == nil {
		return StepDefinition{}, false
	}
	step, ok := r.StepIndex[stepID]
	return step, ok
}

func normalizeStep(step StepDefinition) StepDefinition {
	// Copy slices to avoid unexpected mutation
	next := append([]string{}, step.NextStepIDs...)
	deps := append([]string{}, step.DependsOn...)
	step.NextStepIDs = uniqueStrings(trimStrings(next))
	step.DependsOn = uniqueStrings(trimStrings(deps))
	step.NodeKind = strings.TrimSpace(strings.ToLower(step.NodeKind))
	step.NodeRef = strings.TrimSpace(step.NodeRef)
	return step
}

func trimStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func uniqueStrings(values []string) []string {
	set := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, v := range values {
		if _, ok := set[v]; ok {
			continue
		}
		set[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func appendUnique(in []string, value string) []string {
	for _, existing := range in {
		if existing == value {
			return in
		}
	}
	return append(in, value)
}

func hasCycle(adj map[string][]string, nodes map[string]struct{}) bool {
	visited := make(map[string]int, len(nodes)) // 0:unvisited 1:visiting 2:visited

	var dfs func(node string) bool
	dfs = func(node string) bool {
		if state, ok := visited[node]; ok {
			if state == 1 {
				return true
			}
			if state == 2 {
				return false
			}
		}
		visited[node] = 1
		for _, next := range adj[node] {
			if dfs(next) {
				return true
			}
		}
		visited[node] = 2
		return false
	}

	for node := range nodes {
		if visited[node] == 0 {
			if dfs(node) {
				return true
			}
		}
	}
	return false
}
