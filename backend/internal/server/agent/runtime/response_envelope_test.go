package runtime

import (
	"context"
	"github.com/ArtisanCloud/PowerX/pkg/corex/agent/evidence"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"testing"

	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
)

func TestRuntimeAcceptsPlatformEvidenceButRejectsRetiredContract(t *testing.T) {
	ctx := evidence.WithLedger(context.Background())
	d := evidence.Draft{Schema: evidence.DraftSchema, Kind: "analysis", Data: []evidence.DraftDatum{}, Calculations: []evidence.Calculation{}, Hypotheses: []string{}, Gaps: []string{"input_required"}, Actions: []string{}}
	r, err := evidence.Compile(ctx, d, map[string]any{}, nil, uuid.NewString(), uuid.NewString(), uuid.NewString())
	require.NoError(t, err)
	got, err := responseEnvelopeFromExecutionResult(map[string]any{"response_envelope": r})
	require.NoError(t, err)
	require.Equal(t, "needs_action", got["outcome"])
	require.NoError(t, evidence.Verify(ctx, got))
	r["schema"] = "powerx.agent.response/v3"
	_, err = ValidateAgentResponseEnvelope(r)
	require.Error(t, err)
}

func TestResponseEnvelopeFromExecutionResultRequiresExplicitEnvelope(t *testing.T) {
	got, err := responseEnvelopeFromExecutionResult(map[string]any{"result": map[string]any{"content": "raw markdown"}})
	if err != nil {
		t.Fatalf("absence is distinguished from invalid envelope: %v", err)
	}
	if got != nil {
		t.Fatalf("raw markdown must not be auto-wrapped: %#v", got)
	}
}

func TestFinalResponseUpstreamTaskRefsUsesOnlyTerminalDependencies(t *testing.T) {
	refs := finalResponseUpstreamTaskRefs(&flowschema.ExecutionPlan{Tasks: []flowschema.PlanTask{
		{TaskID: "source_analysis", Stage: 1},
		{TaskID: "campaign_analysis", Stage: 1},
		{TaskID: "knowledge_curation", Stage: 2, DependsOn: []string{"source_analysis", "campaign_analysis"}},
		{TaskID: "campaign_review_synthesis", Stage: 3, DependsOn: []string{"knowledge_curation"}},
	}})
	for _, expected := range []string{"source_analysis", "campaign_analysis", "knowledge_curation"} {
		if _, ok := refs[expected]; !ok {
			t.Fatalf("missing upstream task ref %q: %#v", expected, refs)
		}
	}
	if _, ok := refs["campaign_review_synthesis"]; ok {
		t.Fatalf("terminal task must not cite itself: %#v", refs)
	}
}
