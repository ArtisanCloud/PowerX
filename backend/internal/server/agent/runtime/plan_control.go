package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/google/uuid"
)

var ErrRuntimeBudgetExhausted = errors.New("runtime budget exhausted")

type RuntimeBudget struct {
	MaxPlanRevisions     int
	MaxObservations      int
	MaxSteps             int
	MaxCapabilityCalls   int
	MaxConcurrentTasks   int
	MaxExecutionDuration time.Duration
	PlanRevisions        int
	Observations         int
	Steps                int
	CapabilityCalls      int
	mu                   sync.Mutex
}

func (b *RuntimeBudget) ConsumeRevision() error {
	if b == nil {
		return ErrRuntimeBudgetExhausted
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.MaxPlanRevisions <= 0 || b.PlanRevisions >= b.MaxPlanRevisions {
		return fmt.Errorf("%w: plan revisions", ErrRuntimeBudgetExhausted)
	}
	b.PlanRevisions++
	return nil
}

func (b *RuntimeBudget) ConsumeObservation() error {
	if b == nil {
		return ErrRuntimeBudgetExhausted
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.MaxObservations <= 0 || b.Observations >= b.MaxObservations {
		return fmt.Errorf("%w: observations", ErrRuntimeBudgetExhausted)
	}
	b.Observations++
	return nil
}

func (b *RuntimeBudget) ConsumeTask(nodeKind string) error {
	if b == nil {
		return ErrRuntimeBudgetExhausted
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.MaxSteps <= 0 || b.Steps >= b.MaxSteps {
		return fmt.Errorf("%w: steps", ErrRuntimeBudgetExhausted)
	}
	if strings.EqualFold(strings.TrimSpace(nodeKind), "tooling") && (b.MaxCapabilityCalls <= 0 || b.CapabilityCalls >= b.MaxCapabilityCalls) {
		return fmt.Errorf("%w: capability calls", ErrRuntimeBudgetExhausted)
	}
	b.Steps++
	if strings.EqualFold(strings.TrimSpace(nodeKind), "tooling") {
		b.CapabilityCalls++
	}
	return nil
}

func (b *RuntimeBudget) ValidatePlanConcurrency(plan flowschema.ExecutionPlan) error {
	if b == nil || b.MaxConcurrentTasks <= 0 {
		return ErrRuntimeBudgetExhausted
	}
	if len(plan.Tasks) == 0 {
		return fmt.Errorf("%w: empty execution plan", ErrRuntimeBudgetExhausted)
	}
	return nil
}

type PlanRevision struct {
	RevisionUUID            uuid.UUID                `json:"revision_uuid"`
	ParentRevisionUUID      *uuid.UUID               `json:"parent_revision_uuid,omitempty"`
	SnapshotUUID            uuid.UUID                `json:"snapshot_uuid"`
	ReasonCode              string                   `json:"reason_code"`
	Plan                    flowschema.ExecutionPlan `json:"plan"`
	SupersededTaskRefs      []string                 `json:"superseded_task_refs"`
	NewTaskRefs             []string                 `json:"new_task_refs"`
	TriggerObservationUUIDs []uuid.UUID              `json:"trigger_observation_uuids,omitempty"`
	CreatedAt               time.Time                `json:"created_at"`
}

// PlanController permits revisions only inside the immutable snapshot that
// existed before planning; it cannot discover new resources or grants.
type PlanController struct {
	snapshot     *ResourceSnapshot
	budget       *RuntimeBudget
	current      *PlanRevision
	observations []ResourceObservation
}

func (c *PlanController) Observe(ctx context.Context, env string, resourceUUID uuid.UUID, purpose string) (ResourceObservation, error) {
	if c == nil || c.snapshot == nil {
		return ResourceObservation{}, fmt.Errorf("plan controller is not initialized")
	}
	if _, err := c.snapshot.RequireReadable(resourceUUID); err != nil {
		return ResourceObservation{}, err
	}
	observation, err := ObserveSnapshotResource(ctx, c.budget, env, resourceUUID, purpose)
	if err != nil {
		return ResourceObservation{}, err
	}
	c.observations = append(c.observations, observation)
	return observation, nil
}

func (c *PlanController) Observations() []ResourceObservation {
	if c == nil {
		return nil
	}
	out := make([]ResourceObservation, len(c.observations))
	copy(out, c.observations)
	return out
}

func NewPlanController(snapshot *ResourceSnapshot, budget *RuntimeBudget, initial flowschema.ExecutionPlan) (*PlanController, error) {
	if snapshot == nil || snapshot.SnapshotUUID == uuid.Nil {
		return nil, fmt.Errorf("resource snapshot is required")
	}
	if budget == nil {
		return nil, fmt.Errorf("runtime budget is required")
	}
	if initial.PlanID == "" {
		return nil, fmt.Errorf("initial plan_id is required")
	}
	current := &PlanRevision{RevisionUUID: uuid.New(), SnapshotUUID: snapshot.SnapshotUUID, ReasonCode: "initial_plan", Plan: initial, CreatedAt: time.Now().UTC()}
	return &PlanController{snapshot: snapshot, budget: budget, current: current}, nil
}

func (c *PlanController) Revise(reasonCode string, next flowschema.ExecutionPlan) (*PlanRevision, error) {
	if c == nil || c.current == nil {
		return nil, fmt.Errorf("plan controller is not initialized")
	}
	if reasonCode == "" || next.PlanID == "" {
		return nil, fmt.Errorf("revision reason_code and plan_id are required")
	}
	if err := c.budget.ConsumeRevision(); err != nil {
		return nil, err
	}
	parent := c.current.RevisionUUID
	revision := &PlanRevision{RevisionUUID: uuid.New(), ParentRevisionUUID: &parent, SnapshotUUID: c.snapshot.SnapshotUUID, ReasonCode: reasonCode, Plan: next, SupersededTaskRefs: taskRefs(c.current.Plan), NewTaskRefs: taskRefs(next), CreatedAt: time.Now().UTC()}
	c.current = revision
	return revision, nil
}

func taskRefs(plan flowschema.ExecutionPlan) []string {
	refs := make([]string, 0, len(plan.Tasks))
	for _, task := range plan.Tasks {
		if task.TaskID != "" {
			refs = append(refs, task.TaskID)
		}
	}
	return refs
}

func (c *PlanController) Current() *PlanRevision {
	if c == nil {
		return nil
	}
	return c.current
}
