package workflow

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	modelworkflow "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/workflow"
	workflowrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/workflow"
)

type revisionDefinitionStore struct {
	source  *modelworkflow.WorkflowDefinition
	created *modelworkflow.WorkflowDefinition
}

func (s *revisionDefinitionStore) CreateDefinition(_ context.Context, def *modelworkflow.WorkflowDefinition) (*modelworkflow.WorkflowDefinition, error) {
	def.PowerUUIDModel.UUID = uuid.New()
	s.created = def
	return def, nil
}

func (s *revisionDefinitionStore) NextVersion(context.Context, string, string) (int32, error) {
	return 4, nil
}

func (s *revisionDefinitionStore) GetByUUID(context.Context, string, uuid.UUID, *int32) (*modelworkflow.WorkflowDefinition, error) {
	if s.source == nil {
		return nil, errors.New("not found")
	}
	return s.source, nil
}

func (s *revisionDefinitionStore) GetLatestPublished(context.Context, string, uuid.UUID) (*modelworkflow.WorkflowDefinition, error) {
	return s.source, nil
}

func (s *revisionDefinitionStore) ListVersionsByWorkflow(context.Context, string, uuid.UUID) ([]modelworkflow.WorkflowDefinition, error) {
	if s.source == nil {
		return nil, nil
	}
	return []modelworkflow.WorkflowDefinition{*s.source}, nil
}

func (s *revisionDefinitionStore) ListByTenant(context.Context, workflowrepo.DefinitionListFilter) ([]modelworkflow.WorkflowDefinition, int64, error) {
	return nil, 0, errors.New("not used")
}

func (s *revisionDefinitionStore) UpdateStatus(context.Context, string, uuid.UUID, int32, string, map[string]interface{}) error {
	return errors.New("not used")
}

func TestCreateDefinitionRevisionCreatesDraftVersion(t *testing.T) {
	sourceUUID := uuid.New()
	actorUUID := uuid.New()
	store := &revisionDefinitionStore{
		source: &modelworkflow.WorkflowDefinition{
			PowerUUIDModel: coremodel.PowerUUIDModel{UUID: sourceUUID},
			TenantUUID:     "tenant-a",
			Name:           "marketing_knowledge_capture",
			Description:    "workflow.pack.marketingKnowledgeCapture.description",
			Version:        3,
			Status:         "published",
			StepGraph: toJSONOrEmpty([]StepDefinition{
				{ID: "input", Type: "system", NodeKind: "input.capture", Config: map[string]any{
					"input_schema_ref":     "workflow.input.manual.v1",
					"source_policy":        map[string]any{"text": true},
					"artifact_output_path": "$.artifacts.source",
				}, NextStepIDs: []string{"end"}},
				{ID: "end", Type: "system", NodeKind: "workflow.end", Config: map[string]any{}},
			}),
			Metadata: toJSONOrEmpty(map[string]any{"category": "knowledge_curation"}),
		},
	}
	registry := NewNodeAdapterRegistry()
	if err := RegisterWorkflowNodeAdapters(registry, WorkflowNodeAdapterDeps{}); err != nil {
		t.Fatalf("register adapters: %v", err)
	}
	svc := &Service{
		definitions: store,
		adapters:    registry,
	}

	created, err := svc.CreateDefinitionRevision(context.Background(), CreateDefinitionRevisionInput{
		TenantUUID: "tenant-a",
		SourceUUID: sourceUUID,
		CreatedBy:  actorUUID,
		Steps: []StepDefinition{
			{ID: "input", Type: "system", NodeKind: "input.capture", Config: map[string]any{
				"input_schema_ref":     "workflow.input.manual.v1",
				"source_policy":        map[string]any{"text": true},
				"artifact_output_path": "$.artifacts.source",
			}, NextStepIDs: []string{"end"}},
			{ID: "end", Type: "system", NodeKind: "workflow.end", Config: map[string]any{}},
		},
	})
	if err != nil {
		t.Fatalf("create revision: %v", err)
	}
	if created.UUID == sourceUUID {
		t.Fatalf("revision reused source uuid")
	}
	if created.Status != "draft" || created.Version != 4 {
		t.Fatalf("unexpected revision status/version: %#v", created)
	}
	if created.WorkflowPackKey != store.source.WorkflowPackKey || created.Checksum != "" {
		t.Fatalf("unexpected revision pack/checksum fields: %#v", created)
	}
}
