package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/repository"
	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type PlanRevisionService struct {
	repo *repository.PlanRevisionRepository
}

type planRevisionServiceContextKey struct{}

func NewPlanRevisionService(db *gorm.DB) *PlanRevisionService {
	return &PlanRevisionService{repo: repository.NewPlanRevisionRepository(db)}
}

func ContextWithPlanRevisionService(ctx context.Context, service *PlanRevisionService) context.Context {
	if ctx == nil || service == nil {
		return ctx
	}
	return context.WithValue(ctx, planRevisionServiceContextKey{}, service)
}

func PlanRevisionServiceFromContext(ctx context.Context) (*PlanRevisionService, bool) {
	if ctx == nil {
		return nil, false
	}
	service, ok := ctx.Value(planRevisionServiceContextKey{}).(*PlanRevisionService)
	return service, ok && service != nil
}

func (s *PlanRevisionService) Persist(ctx context.Context, env, tenantUUID string, runUUID uuid.UUID, revision *PlanRevision, budget RuntimeBudget, triggerObservations []ResourceObservation) error {
	if s == nil || s.repo == nil || revision == nil || strings.TrimSpace(env) == "" || strings.TrimSpace(tenantUUID) == "" || runUUID == uuid.Nil || revision.RevisionUUID == uuid.Nil || revision.SnapshotUUID == uuid.Nil {
		return fmt.Errorf("plan revision persistence input is incomplete")
	}
	plan, err := json.Marshal(revision.Plan)
	if err != nil {
		return fmt.Errorf("marshal plan revision: %w", err)
	}
	triggerIDs := make([]string, 0, len(triggerObservations))
	for _, observation := range triggerObservations {
		if observation.ObservationUUID == uuid.Nil {
			return fmt.Errorf("plan revision trigger observation_uuid is required")
		}
		triggerIDs = append(triggerIDs, observation.ObservationUUID.String())
	}
	revision.TriggerObservationUUIDs = make([]uuid.UUID, 0, len(triggerObservations))
	for _, observation := range triggerObservations {
		revision.TriggerObservationUUIDs = append(revision.TriggerObservationUUIDs, observation.ObservationUUID)
	}
	triggers, err := json.Marshal(triggerIDs)
	if err != nil {
		return fmt.Errorf("marshal trigger observations: %w", err)
	}
	superseded, err := json.Marshal(revision.SupersededTaskRefs)
	if err != nil {
		return fmt.Errorf("marshal superseded task refs: %w", err)
	}
	newTasks, err := json.Marshal(revision.NewTaskRefs)
	if err != nil {
		return fmt.Errorf("marshal new task refs: %w", err)
	}
	return s.repo.Create(ctx, &dbmodel.AgentPlanRevision{UUID: revision.RevisionUUID, Env: strings.TrimSpace(env), TenantUUID: strings.TrimSpace(tenantUUID), RunUUID: runUUID, SnapshotUUID: revision.SnapshotUUID, ParentRevisionUUID: revision.ParentRevisionUUID, ReasonCode: revision.ReasonCode, Plan: datatypes.JSON(plan), TriggerObservationUUIDs: datatypes.JSON(triggers), SupersededTaskRefs: datatypes.JSON(superseded), NewTaskRefs: datatypes.JSON(newTasks), PlanRevisionsConsumed: budget.PlanRevisions, ObservationsConsumed: budget.Observations})
}

func (s *PlanRevisionService) Restore(ctx context.Context, env, tenantUUID string, runUUID, snapshotUUID, revisionUUID uuid.UUID) (*PlanRevision, error) {
	if s == nil || s.repo == nil || snapshotUUID == uuid.Nil {
		return nil, fmt.Errorf("plan revision restore input is incomplete")
	}
	row, err := s.repo.GetScoped(ctx, env, tenantUUID, runUUID, revisionUUID)
	if err != nil {
		return nil, err
	}
	if row.SnapshotUUID != snapshotUUID {
		return nil, fmt.Errorf("plan revision snapshot mismatch")
	}
	var plan flowschema.ExecutionPlan
	if err := json.Unmarshal(row.Plan, &plan); err != nil {
		return nil, fmt.Errorf("decode persisted plan revision: %w", err)
	}
	var triggerIDs []string
	if err := json.Unmarshal(row.TriggerObservationUUIDs, &triggerIDs); err != nil {
		return nil, fmt.Errorf("decode plan revision trigger observations: %w", err)
	}
	triggers := make([]uuid.UUID, 0, len(triggerIDs))
	for _, triggerID := range triggerIDs {
		triggerUUID, err := uuid.Parse(triggerID)
		if err != nil || triggerUUID == uuid.Nil {
			return nil, fmt.Errorf("plan revision trigger observation_uuid is invalid")
		}
		triggers = append(triggers, triggerUUID)
	}
	return &PlanRevision{RevisionUUID: row.UUID, SnapshotUUID: row.SnapshotUUID, ParentRevisionUUID: row.ParentRevisionUUID, ReasonCode: row.ReasonCode, Plan: plan, TriggerObservationUUIDs: triggers}, nil
}

// ValidateSemanticReplay proves a persisted semantic revision can be rebuilt
// from its immutable parent, frozen snapshot, and exactly its recorded typed
// observations. It refuses missing/extra observations and any plan mismatch.
func (s *PlanRevisionService) ValidateSemanticReplay(ctx context.Context, env, tenantUUID string, runUUID uuid.UUID, snapshot *ResourceSnapshot, revision *PlanRevision, observations []ResourceObservation) error {
	if s == nil || s.repo == nil || snapshot == nil || revision == nil || revision.ReasonCode != "observation.semantic_continuation" || revision.ParentRevisionUUID == nil || revision.SnapshotUUID != snapshot.SnapshotUUID {
		return fmt.Errorf("semantic replay input is invalid")
	}
	if len(revision.TriggerObservationUUIDs) == 0 {
		return fmt.Errorf("semantic replay trigger observations are required")
	}
	byUUID := make(map[uuid.UUID]ResourceObservation, len(observations))
	for _, observation := range observations {
		byUUID[observation.ObservationUUID] = observation
	}
	triggers := make([]ResourceObservation, 0, len(revision.TriggerObservationUUIDs))
	for _, triggerUUID := range revision.TriggerObservationUUIDs {
		observation, ok := byUUID[triggerUUID]
		if !ok {
			return fmt.Errorf("semantic replay observation is missing")
		}
		triggers = append(triggers, observation)
	}
	parent, err := s.Restore(ctx, env, tenantUUID, runUUID, snapshot.SnapshotUUID, *revision.ParentRevisionUUID)
	if err != nil {
		return fmt.Errorf("restore semantic replay parent: %w", err)
	}
	reason, rebuilt, err := (SemanticReplanner{}).Replan(ctx, PlanRevisionRequest{Snapshot: snapshot, Observations: triggers, CurrentPlan: parent.Plan})
	if err != nil || reason != revision.ReasonCode {
		return fmt.Errorf("semantic replay cannot rebuild revision")
	}
	expected, err := json.Marshal(revision.Plan)
	if err != nil {
		return fmt.Errorf("encode persisted semantic plan: %w", err)
	}
	actual, err := json.Marshal(rebuilt)
	if err != nil || string(expected) != string(actual) {
		return fmt.Errorf("semantic replay plan mismatch")
	}
	return nil
}
