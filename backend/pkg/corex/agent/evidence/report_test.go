package evidence

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func testDraft() Draft {
	return Draft{Schema: DraftSchema, Kind: "analysis", Data: []DraftDatum{
		{Key: "revenue", Label: "revenue", Unit: "万元", Scope: "campaign", Kind: "quantity", Source: Source{"/message", "29万元", "29"}},
		{Key: "cost", Label: "cost", Unit: "万元", Scope: "campaign", Kind: "quantity", Source: Source{"/message", "34.2万元", "34.2"}},
		{Key: "claimed", Label: "claimed", Unit: "", Scope: "campaign", Kind: "reported", Source: Source{"/message", "3.37", "3.37"}},
		{Key: "repeat", Label: "repeat", Unit: "%", Scope: "cohort", Kind: "reported", Source: Source{"/message", "27.8%", "27.8"}},
	}, Calculations: []Calculation{{Key: "ratio", Label: "ratio", Expression: "r/c", Bindings: map[string]string{"r": "revenue", "c": "cost"}, Precision: 3, CompareTo: "claimed"}}, Hypotheses: []string{}, Gaps: []string{}, Actions: []string{}}
}
func compileTest(ctx context.Context, d Draft) (map[string]any, error) {
	return Compile(ctx, d, map[string]any{"message": "29万元 34.2万元 3.37 27.8%"}, []string{"/message"}, uuid.NewString(), uuid.NewString(), uuid.NewString())
}

func TestCompileSeparatesReportedAndComputedAndDetectsConflict(t *testing.T) {
	ctx := WithLedger(context.Background())
	report, err := compileTest(ctx, testDraft())
	require.NoError(t, err)
	require.Equal(t, "needs_action", report["outcome"])
	p := report["presentation"].(map[string]any)
	require.Len(t, p["reported"], 4)
	require.Len(t, p["computed"], 1)
	require.Len(t, p["conflicts"], 1)
	require.Equal(t, "0.848", p["computed"].([]any)[0].(map[string]any)["display_value"])
	require.NoError(t, Verify(ctx, report))
	summary := TraceSummary(report)
	require.Len(t, summary["computed"], 1)
	require.NotContains(t, summary["computed"].([]map[string]any)[0]["operands"].(map[string]any)["r"], "source")
	_, err = ValidateReport(report)
	require.NoError(t, err)
	p["computed"].([]any)[0].(map[string]any)["display_value"] = "3.37"
	require.Error(t, Verify(ctx, report))
	_, err = ValidateReport(report)
	require.Error(t, err)
}

func TestCompileRejectsInventedCountsAndSources(t *testing.T) {
	for _, change := range []func(*Draft){
		func(d *Draft) { d.Calculations[0].Bindings["r"] = "repeat" },
		func(d *Draft) { d.Data[3].Kind = "quantity" },
		func(d *Draft) { d.Calculations[0].Expression = "r/100" },
		func(d *Draft) { d.Data[0].Source.Literal = "100" },
		func(d *Draft) { d.Data[0].Source.Pointer = "/other_tenant" },
		func(d *Draft) { d.Data[0].Scope = "other_campaign" },
		func(d *Draft) { d.Data[0].Unit = "元" },
	} {
		d := testDraft()
		change(&d)
		_, err := compileTest(WithLedger(context.Background()), d)
		require.Error(t, err)
	}
}

func TestEvidenceCannotBeReusedInAnotherRun(t *testing.T) {
	ctx := WithLedger(context.Background())
	report, err := compileTest(ctx, testDraft())
	require.NoError(t, err)
	require.Error(t, Verify(WithLedger(context.Background()), report))
}

func TestReportedOnlyNeedsNoFabricatedCalculation(t *testing.T) {
	d := testDraft()
	d.Data = d.Data[3:]
	d.Calculations = []Calculation{}
	d.Gaps = []string{"counts_required"}
	out, err := compileTest(WithLedger(context.Background()), d)
	require.NoError(t, err)
	require.Empty(t, out["presentation"].(map[string]any)["computed"])
}

func TestReportedRatioCannotBePromotedViaIdentityCalculation(t *testing.T) {
	d := testDraft()
	d.Calculations = []Calculation{{Key: "copy", Label: "copy", Expression: "x", Bindings: map[string]string{"x": "claimed"}, Precision: 2}}
	_, err := compileTest(WithLedger(context.Background()), d)
	require.ErrorContains(t, err, "evidence.quantity_reference_required: claimed")
}

func TestNarrativeCannotReintroduceModelCalculatedNumbers(t *testing.T) {
	d := testDraft()
	d.Actions = []string{"ratio=0.847"}
	_, err := compileTest(WithLedger(context.Background()), d)
	require.ErrorContains(t, err, "evidence.numeric_narrative_forbidden")
}

func TestExplicitNumberAndUnitAllowOriginalWhitespace(t *testing.T) {
	d := testDraft()
	d.Data[0].Source.Quote = "29 万元"
	_, err := Compile(WithLedger(context.Background()), d, map[string]any{"message": "29 万元 34.2万元 3.37 27.8%"}, []string{"/message"}, uuid.NewString(), uuid.NewString(), uuid.NewString())
	require.NoError(t, err)
}
