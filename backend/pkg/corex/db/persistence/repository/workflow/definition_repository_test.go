package workflow

import (
	"testing"
	"time"

	modelworkflow "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/workflow"
)

func TestLatestWorkflowDefinitionsKeepsNewestVersionPerWorkflow(t *testing.T) {
	base := time.Date(2026, 8, 24, 10, 0, 0, 0, time.UTC)
	candidates := []modelworkflow.WorkflowDefinition{
		{
			Name:            "marketing_knowledge_capture",
			Version:         3,
			SourceType:      "workflow_pack",
			WorkflowPackKey: "marketing_knowledge_capture",
		},
		{
			Name:            "marketing_knowledge_capture",
			Version:         4,
			SourceType:      "workflow_pack",
			WorkflowPackKey: "marketing_knowledge_capture",
		},
		{
			Name:       "manual_flow",
			Version:    1,
			SourceType: "manual",
		},
	}
	candidates[0].UpdatedAt = base.Add(2 * time.Hour)
	candidates[1].UpdatedAt = base.Add(time.Hour)
	candidates[2].UpdatedAt = base

	defs := latestWorkflowDefinitions(candidates)

	if len(defs) != 2 {
		t.Fatalf("expected 2 workflow list entries, got %d", len(defs))
	}
	if defs[0].Name != "marketing_knowledge_capture" || defs[0].Version != 4 {
		t.Fatalf("expected latest marketing workflow version first, got %s v%d", defs[0].Name, defs[0].Version)
	}
	if defs[1].Name != "manual_flow" || defs[1].Version != 1 {
		t.Fatalf("expected manual workflow second, got %s v%d", defs[1].Name, defs[1].Version)
	}
}
