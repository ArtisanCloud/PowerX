package evidence

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func policyFixture() CalculationPolicy {
	field := func(key, kind string, units []string) InputField {
		return InputField{Key: key, Kind: kind, UnitTokens: units, LabelI18n: map[string]string{"en-US": key}, DescriptionI18n: map[string]string{"en-US": key}, EvidenceTermsI18n: map[string][]string{"en-US": {key}}, AppliesTo: []string{"test_profile"}}
	}
	return CalculationPolicy{Schema: PolicySchema, ActivityProfiles: []ActivityProfile{{Key: "test_profile", LabelI18n: map[string]string{"en-US": "test"}, EvidenceAnyI18n: map[string][]string{"en-US": {"test"}}}}, InputFields: []InputField{
		field("used", "quantity", []string{""}), field("total", "quantity", []string{""}), field("claim", "reported", []string{"%"}),
	}, Formulas: []Formula{{Key: "fraction", LabelI18n: map[string]string{"en-US": "fraction"}, Expression: "n/d", Bindings: map[string]string{"n": "used", "d": "total"}, Precision: 2, Percent: true, CompareTo: "claim", WhenAnyPresent: []string{"used", "claim"}, AppliesTo: []string{"test_profile"}}}}
}

func TestPolicyOwnsFormulaAndScaleAcrossChangedNumbers(t *testing.T) {
	p, err := ReadCalculationPolicy(policyFixture())
	require.NoError(t, err)
	for _, c := range []struct{ n, d, want string }{{"17", "80", "21.25%"}, {"29", "40", "72.5%"}} {
		extracted := PolicyExtraction{Schema: ExtractionSchema, Data: []InputValue{
			{Key: "used", Scope: "same", Source: Source{"/message", c.n, c.n}},
			{Key: "total", Scope: "same", Source: Source{"/message", c.d, c.d}},
		}}
		plan, gaps, err := p.BuildPlan(extracted, []string{"test_profile"}, "en-US")
		require.NoError(t, err)
		require.Empty(t, gaps)
		require.Equal(t, "n/d", plan.Calculations[0].Expression)
		require.True(t, plan.Calculations[0].Percent)
		out, err := ExecutePlan(WithLedger(context.Background()), plan, map[string]any{"message": c.n + " " + c.d}, []string{"/message"}, uuid.NewString(), uuid.NewString(), uuid.NewString())
		require.NoError(t, err)
		require.Equal(t, c.want, out["presentation"].(map[string]any)["computed"].([]any)[0].(map[string]any)["display_value"])
	}
}

func TestPolicyMissingOperandsAreExplicitNotInvented(t *testing.T) {
	p := policyFixture()
	plan, gaps, err := p.BuildPlan(PolicyExtraction{Schema: ExtractionSchema, Data: []InputValue{{Key: "claim", Unit: "%", Scope: "same", Source: Source{"/message", "27.8%", "27.8"}}}}, []string{"test_profile"}, "en-US")
	require.NoError(t, err)
	require.Empty(t, plan.Calculations)
	require.Equal(t, []string{"used", "total"}, gaps[0].InputLabels)
	plan, gaps, err = p.BuildPlan(PolicyExtraction{Schema: ExtractionSchema, Data: []InputValue{}}, []string{"test_profile"}, "en-US")
	require.NoError(t, err)
	require.Empty(t, plan.Calculations)
	require.Empty(t, gaps)
}

func TestInvalidPolicyRejectedBeforeExecution(t *testing.T) {
	for _, change := range []func(*CalculationPolicy){
		func(p *CalculationPolicy) { p.Formulas[0].Bindings["n"] = "claim" },
		func(p *CalculationPolicy) { p.Formulas[0].Expression = "n / 100" },
		func(p *CalculationPolicy) { p.Formulas[0].WhenAnyPresent = []string{"absent"} },
		func(p *CalculationPolicy) { p.Formulas[0].CompareTo = "used" },
		func(p *CalculationPolicy) { p.Formulas[0].Precision = 13 },
	} {
		p := policyFixture()
		change(&p)
		_, err := ReadCalculationPolicy(p)
		require.Error(t, err)
	}
	_, err := ReadCalculationPolicy(nil)
	require.ErrorContains(t, err, "skill.calculation_policy_required")
}

func TestSourceSchemaDoesNotAllowModelToChangeKindOrLabels(t *testing.T) {
	s := SelectionJSONSchema(policyFixture(), []NumericToken{{Key: "token_0", Unit: "", Source: Source{Quote: "used 1", Literal: "1"}}}, []string{"test_profile"}, "en-US")
	data := s["properties"].(map[string]any)["data"].(map[string]any)
	require.False(t, data["additionalProperties"].(bool))
	p := data["properties"].(map[string]any)["used"].(map[string]any)["properties"].(map[string]any)
	require.NotContains(t, p, "kind")
	require.NotContains(t, p, "label")
	require.NotContains(t, p, "unit")
	require.NotContains(t, p, "source")
	require.Equal(t, []string{"token_0"}, p["token_ref"].(map[string]any)["enum"])
}
