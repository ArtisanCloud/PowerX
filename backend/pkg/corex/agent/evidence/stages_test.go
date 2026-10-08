package evidence

import (
	"context"
	"encoding/json"
	"strings"
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

func TestNotesSourceContextPreservesBusinessMeaningWithoutNumbersOrUpstreamAuthority(t *testing.T) {
	projection := map[string]any{}
	payload := map[string]any{
		"message":           "目标是激活近6个月未续费的老客。信息流围观流量多，下单转化0.08%。",
		"upstream_campaign": map[string]any{"result": map[string]any{"content": "声称34.2万元成本缺失", "private_metadata": "do-not-copy"}},
		"unrelated_secret":  "do-not-copy",
	}
	require.NoError(t, AddNotesSourceContext(projection, payload, []string{"/message"}))
	items := projection["source_context"].([]map[string]any)
	require.Len(t, items, 2)
	require.Equal(t, "original_source", items[0]["kind"])
	require.Contains(t, items[0]["text"], "老客")
	require.Contains(t, items[0]["text"], "围观流量")
	require.Equal(t, "upstream_opinion", items[1]["kind"])
	require.Equal(t, false, items[1]["independently_verified"])
	raw, err := json.Marshal(projection)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "0.08")
	require.NotContains(t, string(raw), "34.2")
	require.NotContains(t, string(raw), "do-not-copy")
	require.ErrorContains(t, AddNotesSourceContext(map[string]any{}, map[string]any{}, []string{"/message"}), "source_missing")
	require.ErrorContains(t, AddNotesSourceContext(map[string]any{}, map[string]any{"message": strings.Repeat("a", 128*1024+1)}, []string{"/message"}), "notes_context_limit_exceeded")
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
