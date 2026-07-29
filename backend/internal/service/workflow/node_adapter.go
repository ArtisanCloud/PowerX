package workflow

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/google/uuid"

	modelworkflow "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/workflow"
)

const (
	NodeResultStatusSucceeded    = "succeeded"
	NodeResultStatusFailed       = "failed"
	NodeResultStatusWaiting      = "waiting"
	NodeResultStatusCompensating = "compensating"
)

var (
	ErrNodeAdapterNil        = errors.New("workflow.node_adapter_nil")
	ErrNodeKindRequired      = errors.New("workflow.node_kind_required")
	ErrNodeAdapterDuplicated = errors.New("workflow.node_adapter_duplicated")
	ErrNodeAdapterNotFound   = errors.New("workflow.node_adapter_unavailable")
)

// NodeSchema describes machine-readable input or output shape for workflow builder.
type NodeSchema struct {
	Type       string         `json:"type,omitempty"`
	Required   []string       `json:"required,omitempty"`
	Properties map[string]any `json:"properties,omitempty"`
}

// NodeAdapterSpec is the catalog-facing description of a registered runtime node.
type NodeAdapterSpec struct {
	NodeKind     string     `json:"node_kind"`
	DisplayName  string     `json:"display_name,omitempty"`
	Category     string     `json:"category,omitempty"`
	Description  string     `json:"description,omitempty"`
	InputSchema  NodeSchema `json:"input_schema,omitempty"`
	OutputSchema NodeSchema `json:"output_schema,omitempty"`
}

// NodeExecutionContext carries the durable workflow state needed by a node adapter.
type NodeExecutionContext struct {
	TenantUUID string
	Instance   *modelworkflow.WorkflowInstance
	StepRecord *modelworkflow.WorkflowStepRecord
	Step       StepDefinition
	AgentUUID  uuid.UUID
	TraceID    string
	Payload    map[string]any
}

// NodeResult is the normalized result emitted by every adapter.
type NodeResult struct {
	Status           string
	Output           map[string]any
	Decision         string
	SelectedBranches []string
	ErrorCode        string
	ErrorMessage     string
	AwaitingHuman    bool
	ReviewTaskUUID   uuid.UUID
	Retryable        bool
}

// ToStepResult converts adapter output into the existing graph router contract.
func (r NodeResult) ToStepResult() StepResult {
	return StepResult{
		Status:           r.Status,
		Decision:         r.Decision,
		Output:           cloneMap(r.Output),
		SelectedBranches: cloneStrings(r.SelectedBranches),
		Approved:         r.Status == NodeResultStatusSucceeded,
		HasApproval:      r.AwaitingHuman || r.ReviewTaskUUID != uuid.Nil,
	}
}

// NodeAdapter executes one node_kind and exposes its catalog schema.
type NodeAdapter interface {
	Spec() NodeAdapterSpec
	Validate(step StepDefinition) error
	Execute(ctx context.Context, exec NodeExecutionContext) (NodeResult, error)
}

// NodeAdapterRegistry stores node adapters by normalized node_kind.
type NodeAdapterRegistry struct {
	mu       sync.RWMutex
	adapters map[string]NodeAdapter
}

func NewNodeAdapterRegistry() *NodeAdapterRegistry {
	return &NodeAdapterRegistry{adapters: map[string]NodeAdapter{}}
}

func (r *NodeAdapterRegistry) Register(adapter NodeAdapter) error {
	if adapter == nil {
		return ErrNodeAdapterNil
	}
	spec := adapter.Spec()
	nodeKind := normalizeNodeKind(spec.NodeKind)
	if nodeKind == "" {
		return ErrNodeKindRequired
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.adapters[nodeKind]; exists {
		return fmt.Errorf("%w: %s", ErrNodeAdapterDuplicated, nodeKind)
	}
	r.adapters[nodeKind] = adapter
	return nil
}

func (r *NodeAdapterRegistry) Adapter(nodeKind string) (NodeAdapter, error) {
	if r == nil {
		return nil, ErrNodeAdapterNotFound
	}
	normalized := normalizeNodeKind(nodeKind)
	if normalized == "" {
		return nil, ErrNodeKindRequired
	}
	r.mu.RLock()
	adapter, ok := r.adapters[normalized]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNodeAdapterNotFound, normalized)
	}
	return adapter, nil
}

func (r *NodeAdapterRegistry) ValidateDefinition(step StepDefinition) error {
	adapter, err := r.Adapter(step.NodeKind)
	if err != nil {
		return err
	}
	return adapter.Validate(step)
}

func (r *NodeAdapterRegistry) List() []NodeAdapterSpec {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	specs := make([]NodeAdapterSpec, 0, len(r.adapters))
	for nodeKind, adapter := range r.adapters {
		spec := adapter.Spec()
		spec.NodeKind = nodeKind
		specs = append(specs, spec)
	}
	r.mu.RUnlock()

	sort.Slice(specs, func(i, j int) bool {
		return specs[i].NodeKind < specs[j].NodeKind
	})
	return specs
}

func normalizeNodeKind(nodeKind string) string {
	return strings.ToLower(strings.TrimSpace(nodeKind))
}

func cloneMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func configString(config map[string]any, key string) string {
	if config == nil {
		return ""
	}
	value, ok := config[key]
	if !ok {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return ""
	}
}

func requireConfigString(step StepDefinition, key string) error {
	if configString(step.Config, key) == "" {
		return fmt.Errorf("workflow.node_config_required: %s.%s", step.ID, key)
	}
	return nil
}

func requireConfigValue(step StepDefinition, key string) error {
	if step.Config == nil {
		return fmt.Errorf("workflow.node_config_required: %s.%s", step.ID, key)
	}
	if _, ok := step.Config[key]; !ok {
		return fmt.Errorf("workflow.node_config_required: %s.%s", step.ID, key)
	}
	return nil
}

func objectSchema() NodeSchema {
	return NodeSchema{
		Type:       "object",
		Properties: map[string]any{},
	}
}

func requiredObjectSchema(required ...string) NodeSchema {
	return NodeSchema{
		Type:       "object",
		Required:   cloneStrings(required),
		Properties: map[string]any{},
	}
}
