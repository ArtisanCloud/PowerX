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

func TestGenericFactsPreserveNewMetricButRejectMismatchedSourceOrUnit(t *testing.T) {
	payload := map[string]any{"message": "直播间停留时长45秒，达人佣金率12%，订单号20260914"}
	tokens, err := TokenizeGenericSources(payload, []string{"/message"})
	require.NoError(t, err)
	require.Len(t, tokens, 3)
	facts, err := ResolveGenericFacts(GenericFactSelection{Schema: GenericFactsSchema, Facts: []GenericFactChoice{
		{Label: "直播间停留时长", Scope: "直播间", TokenRef: "token_0"},
		{Label: "达人佣金率", Scope: "达人", TokenRef: "token_1"},
	}}, tokens)
	require.NoError(t, err)
	require.Len(t, facts, 2)
	require.Equal(t, "reported", facts[0].Kind)
	require.Equal(t, "45", facts[0].Source.Literal)
	_, err = ResolveGenericFacts(GenericFactSelection{Schema: GenericFactsSchema, Facts: []GenericFactChoice{{Label: "不存在的指标", Scope: "直播间", TokenRef: "token_0"}}}, tokens)
	require.ErrorContains(t, err, "evidence.generic_fact_label_not_in_source")
	_, err = ResolveGenericFacts(GenericFactSelection{Schema: GenericFactsSchema, Facts: []GenericFactChoice{{Label: "订单号", Scope: "直播间", TokenRef: "token_2"}}}, tokens)
	require.ErrorContains(t, err, "evidence.generic_fact_identifier_or_date_forbidden")
	unknown, err := TokenizeGenericSources(map[string]any{"message": "新增收藏100UV"}, []string{"/message"})
	require.NoError(t, err)
	require.Equal(t, "UV", unknown[0].Unit)
	_, err = ResolveGenericFacts(GenericFactSelection{Schema: GenericFactsSchema, Facts: []GenericFactChoice{{Label: "新增收藏", Scope: "本次活动", TokenRef: "token_0"}}}, unknown)
	require.NoError(t, err)
}

func TestGenericFactsDeduplicatesRepeatedSourceSelection(t *testing.T) {
	tokens, err := TokenizeGenericSources(map[string]any{"message": "活动投入34.2万元"}, []string{"/message"})
	require.NoError(t, err)
	facts, err := ResolveGenericFacts(GenericFactSelection{Schema: GenericFactsSchema, Facts: []GenericFactChoice{
		{Label: "活动投入", Scope: "全渠道", TokenRef: "token_0"},
		{Label: "活动投入", Scope: "重复的范围描述", TokenRef: "token_0"},
	}}, tokens)
	require.NoError(t, err)
	require.Len(t, facts, 1)
	require.Equal(t, "全渠道", facts[0].Scope)
}

func TestGenericFactsSchemaBindsEveryLabelToItsTokenSource(t *testing.T) {
	tokens, err := TokenizeGenericSources(map[string]any{"message": "活动投入34.2万元，活动标记GMV 46.2万元"}, []string{"/message"})
	require.NoError(t, err)
	schema := GenericFactsJSONSchema(tokens)
	items := schema["properties"].(map[string]any)["facts"].(map[string]any)["items"].(map[string]any)
	choices := items["oneOf"].([]any)
	require.Len(t, choices, 2)
	first := choices[0].(map[string]any)["properties"].(map[string]any)
	require.Equal(t, "token_0", first["token_ref"].(map[string]any)["const"])
	require.Equal(t, []string{"活动投入"}, first["label"].(map[string]any)["enum"])
	second := choices[1].(map[string]any)["properties"].(map[string]any)
	require.Equal(t, []string{"活动标记GMV"}, second["label"].(map[string]any)["enum"])

	_, err = ResolveGenericFacts(GenericFactSelection{Schema: GenericFactsSchema, Facts: []GenericFactChoice{{Label: "GMV", Scope: "活动", TokenRef: "token_1"}}}, tokens)
	require.ErrorContains(t, err, "evidence.generic_fact_label_not_in_source")
}
