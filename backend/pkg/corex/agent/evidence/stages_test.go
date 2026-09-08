package evidence

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestNotesCannotChangeOrReexecuteCalculation(t *testing.T) {
	ctx := WithLedger(context.Background())
	d := testDraft()
	prepared, err := ExecutePlan(ctx, Plan{Schema: PlanSchema, Kind: d.Kind, Data: d.Data, Calculations: d.Calculations}, map[string]any{"message": "29万元 34.2万元 3.37 27.8%"}, []string{"/message"}, uuid.NewString(), uuid.NewString(), uuid.NewString())
	require.NoError(t, err)
	out, err := AttachNotes(ctx, prepared, Notes{Schema: NotesSchema, Hypotheses: []string{}, Gaps: []string{"counts_required"}, Actions: []string{}})
	require.NoError(t, err)
	require.NoError(t, Verify(ctx, out))
	require.Equal(t, prepared["presentation"].(map[string]any)["computed"], out["presentation"].(map[string]any)["computed"])
	projection, err := NotesContext(prepared, []MissingOperands{})
	require.NoError(t, err)
	require.NotContains(t, projection, "source")
	require.Equal(t, []map[string]string{{"key": "ratio", "label": "ratio"}}, projection["executed_calculations"])
	require.Equal(t, []map[string]string{{"calculation_label": "ratio", "reported_label": "claimed"}}, projection["conflicts"])
	require.Empty(t, prepared["presentation"].(map[string]any)["gaps"])
	_, err = AttachNotes(WithLedger(context.Background()), prepared, Notes{Schema: NotesSchema, Hypotheses: []string{}, Gaps: []string{}, Actions: []string{}})
	require.ErrorContains(t, err, "agent.response_evidence_untrusted")
}

func TestNoRawNumbersProducesExplicitGapNotFabricatedOperands(t *testing.T) {
	ctx := WithLedger(context.Background())
	prepared, err := ExecutePlan(ctx, Plan{Schema: PlanSchema, Kind: "analysis", Data: []DraftDatum{}, Calculations: []Calculation{}}, map[string]any{"message": "missing_inputs"}, []string{"/message"}, uuid.NewString(), uuid.NewString(), uuid.NewString())
	require.NoError(t, err)
	_, err = AttachNotes(ctx, prepared, Notes{Schema: NotesSchema, Hypotheses: []string{}, Gaps: []string{}, Actions: []string{}})
	require.ErrorContains(t, err, "evidence.empty_report")
	out, err := AttachNotes(ctx, prepared, Notes{Schema: NotesSchema, Hypotheses: []string{}, Gaps: []string{"raw_inputs_required"}, Actions: []string{}})
	require.NoError(t, err)
	require.Equal(t, "needs_action", out["outcome"])
}

func TestNotesRejectNumericalRewritesAndSchemaFields(t *testing.T) {
	var notes Notes
	require.Error(t, Decode([]byte(`{"schema":"powerx.agent.evidence-notes/v1","hypotheses":[],"gaps":[],"actions":[],"computed":[]}`), &notes))
	ctx := WithLedger(context.Background())
	prepared, err := compileTest(ctx, testDraft())
	require.NoError(t, err)
	_, err = AttachNotes(ctx, prepared, Notes{Schema: NotesSchema, Hypotheses: []string{"ratio=3.37"}, Gaps: []string{}, Actions: []string{}})
	require.ErrorContains(t, err, "evidence.numeric_narrative_forbidden")
}
