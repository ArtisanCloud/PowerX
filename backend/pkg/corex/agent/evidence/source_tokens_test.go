package evidence

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestSourceTokenPreservesLiteralAndLongestDeclaredUnit(t *testing.T) {
	p := policyFixture()
	p.InputFields[0].UnitTokens = []string{"元", "万元"}
	p.InputFields[1].UnitTokens = []string{"元", "万元"}
	p.InputFields[0].EvidenceTermsI18n = map[string][]string{"en-US": {"投入"}}
	p.InputFields[1].EvidenceTermsI18n = map[string][]string{"en-US": {"产出"}}
	input := "30天，投入20万元，产出40万元，原文15%"
	tokens, err := TokenizeSources(map[string]any{"message": input}, []string{"/message"}, p)
	require.NoError(t, err)
	require.Len(t, tokens, 3)
	require.Equal(t, "20", tokens[0].Source.Literal)
	require.Equal(t, "万元", tokens[0].Unit)
	require.Equal(t, "40", tokens[1].Source.Literal)
	require.Equal(t, "%", tokens[2].Unit)
	for _, token := range tokens {
		require.Contains(t, input, token.Source.Quote)
	}
	out, err := ResolveSelection(SourceSelection{Schema: ExtractionSchema, Data: map[string]SelectedValue{"used": {Scope: "s", TokenRef: "token_0"}}}, tokens, p, []string{"test_profile"}, "en-US")
	require.NoError(t, err)
	require.Equal(t, "20", out.Data[0].Source.Literal)
	_, err = ResolveSelection(SourceSelection{Schema: ExtractionSchema, Data: map[string]SelectedValue{"used": {Scope: "s", TokenRef: "invented"}}}, tokens, p, []string{"test_profile"}, "en-US")
	require.ErrorContains(t, err, "evidence.source_token_missing")
	_, err = ResolveSelection(SourceSelection{Schema: ExtractionSchema, Data: map[string]SelectedValue{"invented": {Scope: "s", TokenRef: "token_0"}}}, tokens, p, []string{"test_profile"}, "en-US")
	require.ErrorContains(t, err, "evidence.source_input_key_unknown: invented")
}

func TestSourceTokenPreservesCommasAndWhitespace(t *testing.T) {
	p := policyFixture()
	p.InputFields[0].UnitTokens = []string{"元"}
	tokens, err := TokenizeSources(map[string]any{"message": "cost 80,000 元; count 860"}, []string{"/message"}, p)
	require.NoError(t, err)
	require.Len(t, tokens, 2)
	require.Equal(t, "80,000", tokens[0].Source.Literal)
	require.Equal(t, "元", tokens[0].Unit)
}

func TestSkillProfileAndEvidenceTermsRejectTemporalNumberAsLeadMetric(t *testing.T) {
	p := CalculationPolicy{Schema: PolicySchema,
		ActivityProfiles: []ActivityProfile{
			{Key: "transaction", LabelI18n: map[string]string{"zh-CN": "交易"}, EvidenceAnyI18n: map[string][]string{"zh-CN": {"GMV", "ROI"}}},
			{Key: "lead", LabelI18n: map[string]string{"zh-CN": "线索"}, EvidenceAnyI18n: map[string][]string{"zh-CN": {"有效线索", "落地页访问"}}},
		},
		InputFields: []InputField{
			{Key: "gmv", Kind: "quantity", UnitTokens: []string{"万元"}, LabelI18n: map[string]string{"zh-CN": "GMV"}, DescriptionI18n: map[string]string{"zh-CN": "GMV"}, EvidenceTermsI18n: map[string][]string{"zh-CN": {"GMV"}}, AppliesTo: []string{"transaction"}},
			{Key: "visits", Kind: "quantity", UnitTokens: []string{"个"}, LabelI18n: map[string]string{"zh-CN": "访问"}, DescriptionI18n: map[string]string{"zh-CN": "访问"}, EvidenceTermsI18n: map[string][]string{"zh-CN": {"落地页访问"}}, AppliesTo: []string{"lead"}},
		},
		Formulas: []Formula{{Key: "gmv_rate", LabelI18n: map[string]string{"zh-CN": "GMV"}, Expression: "n/d", Bindings: map[string]string{"n": "gmv", "d": "gmv"}, Precision: 2, WhenAnyPresent: []string{"gmv"}, AppliesTo: []string{"transaction"}}},
	}
	_, err := ReadCalculationPolicy(p)
	require.NoError(t, err)
	payload := map[string]any{"message": "激活近6个月未续费客户，活动标记GMV 46.2万元，财务ROI 1.35"}
	profiles, err := p.DetectProfiles(payload, []string{"/message"}, "zh-CN")
	require.NoError(t, err)
	require.Equal(t, []string{"transaction"}, profiles)
	tokens, err := TokenizeSources(payload, []string{"/message"}, p)
	require.NoError(t, err)
	schema := SelectionJSONSchema(p, tokens, profiles, "zh-CN")
	properties := schema["properties"].(map[string]any)["data"].(map[string]any)["properties"].(map[string]any)
	require.Contains(t, properties, "gmv")
	require.NotContains(t, properties, "visits")
}
