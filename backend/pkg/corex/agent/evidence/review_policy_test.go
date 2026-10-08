package evidence

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestReviewRulesUseOriginalSourceAndRealComparisonRatherThanUpstreamClaims(t *testing.T) {
	p := policyFixture()
	rule := ReviewRule{Key: "original_goal", Target: "hypotheses", TextI18n: map[string]string{"en-US": "Source-reported existing customer goal"}, EvidenceAllI18n: map[string][]string{"en-US": {"existing customers"}}, WhenAnyFields: []string{}, WhenAnyConflicts: []string{}, WhenAnyMissing: []string{}}
	policy, err := ReadReviewPolicy(ReviewPolicy{Schema: ReviewPolicySchema, Rules: []ReviewRule{rule}}, p)
	require.NoError(t, err)
	prepared, err := compileTest(WithLedger(context.Background()), testDraft())
	require.NoError(t, err)
	choices, err := policy.Choices(prepared, []MissingOperands{}, map[string]any{"message": "different goal", "upstream_analysis": "existing customers"}, []string{"/message"}, "en-US")
	require.NoError(t, err)
	require.Empty(t, choices.Hypotheses)
	choices, err = policy.Choices(prepared, []MissingOperands{}, map[string]any{"message": "existing customers"}, []string{"/message"}, "en-US")
	require.NoError(t, err)
	require.Equal(t, []string{rule.TextI18n["en-US"]}, choices.Hypotheses)
	require.NoError(t, choices.Validate(Notes{Hypotheses: choices.Hypotheses, Actions: []string{}}))
	require.ErrorContains(t, choices.Validate(Notes{Hypotheses: []string{"Click rate is low without a baseline"}, Actions: []string{}}), "review_notes_unsupported")
	require.ErrorContains(t, choices.Validate(Notes{Hypotheses: []string{}, Actions: []string{}}), "review_notes_incomplete")
	schema := NotesJSONSchema()
	choices.ConstrainSchema(schema)
	field := schema["properties"].(map[string]any)["hypotheses"].(map[string]any)
	require.Equal(t, 1, field["minItems"])
	require.Equal(t, choices.Hypotheses, field["items"].(map[string]any)["enum"])
}

func TestReviewPolicyRejectsInventedReferencesAndNumericNarratives(t *testing.T) {
	base := ReviewRule{Key: "rule", Target: "actions", TextI18n: map[string]string{"en-US": "Check source definition"}, EvidenceAllI18n: map[string][]string{"en-US": {}}, WhenAnyFields: []string{"used"}, WhenAnyConflicts: []string{}, WhenAnyMissing: []string{}}
	for _, change := range []func(*ReviewRule){
		func(r *ReviewRule) { r.WhenAnyFields = []string{"invented"} },
		func(r *ReviewRule) { r.WhenAnyConflicts = []string{"invented"} },
		func(r *ReviewRule) { r.WhenAnyComputed = []string{"invented"} },
		func(r *ReviewRule) { r.TextI18n = map[string]string{"en-US": "Invented target 80%"} },
		func(r *ReviewRule) { r.Target = "computed" },
		func(r *ReviewRule) { r.WhenAnyFields = []string{} },
	} {
		rule := base
		change(&rule)
		_, err := ReadReviewPolicy(ReviewPolicy{Schema: ReviewPolicySchema, Rules: []ReviewRule{rule}}, policyFixture())
		require.Error(t, err)
	}
	policy, err := ReadReviewPolicy(nil, policyFixture())
	require.NoError(t, err)
	require.Nil(t, policy)
}

func TestReviewComputedAndConflictConditionsUseActualReceipts(t *testing.T) {
	calculation := policyFixture()
	calculation.Formulas[0].Key = "ratio"
	rule := ReviewRule{Key: "mismatch", Target: "actions", TextI18n: map[string]string{"en-US": "Verify the source denominator definition"}, EvidenceAllI18n: map[string][]string{"en-US": {}}, WhenAnyFields: []string{}, WhenAnyConflicts: []string{"ratio"}, WhenAnyMissing: []string{}, WhenAnyComputed: []string{"ratio"}}
	policy, err := ReadReviewPolicy(ReviewPolicy{Schema: ReviewPolicySchema, Rules: []ReviewRule{rule}}, calculation)
	require.NoError(t, err)
	prepared, err := compileTest(WithLedger(context.Background()), testDraft())
	require.NoError(t, err)
	choices, err := policy.Choices(prepared, []MissingOperands{}, map[string]any{"message": "source"}, []string{"/message"}, "en-US")
	require.NoError(t, err)
	require.Equal(t, []string{rule.TextI18n["en-US"]}, choices.Actions)
	// 没有实际计算时，上游声称发生冲突也不能触发这项核对行动。
	empty, err := ExecutePlan(WithLedger(context.Background()), Plan{Schema: PlanSchema, Kind: "analysis", Data: []DraftDatum{}, Calculations: []Calculation{}}, map[string]any{"message": "source"}, []string{"/message"}, uuid.NewString(), uuid.NewString(), "trace")
	require.NoError(t, err)
	choices, err = policy.Choices(empty, []MissingOperands{}, map[string]any{"message": "source", "upstream_analysis": "ratio conflict"}, []string{"/message"}, "en-US")
	require.NoError(t, err)
	require.Empty(t, choices.Actions)
}
