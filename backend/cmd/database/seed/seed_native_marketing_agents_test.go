package seed

import (
	"context"
	"encoding/json"
	"github.com/ArtisanCloud/PowerX/internal/service/skills"
	"github.com/ArtisanCloud/PowerX/pkg/corex/agent/evidence"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"strings"
	"testing"
)

func TestNativeMarketingSkillsDeclareExecutableEvidenceContract(t *testing.T) {
	for _, item := range nativeMarketingSkillSeeds() {
		require.NotEmpty(t, item.PromptI18n["zh-CN"])
		require.NotEmpty(t, item.PromptI18n["en-US"])
		definition, err := nativeMarketingSkillDefinition(item)
		require.NoError(t, err)
		require.NoError(t, skills.CheckToolDependencies(context.Background(), uuid.NewString(), definition, nil))
		executor := definition["executor"].(map[string]any)
		if item.SkillID == MarketingReviewSummarizeSkillID {
			require.Equal(t, evidence.ReportSchema, executor["response_contract"])
			require.Equal(t, []string{"/message"}, executor["evidence_sources"])
			require.Equal(t, "response_envelope", executor["output_mode"])
		} else {
			require.Equal(t, "markdown", executor["output_mode"])
		}
	}
}

func TestNativeMarketingPublicationComparisonNormalizesTypedDependencies(t *testing.T) {
	for _, item := range nativeMarketingSkillSeeds() {
		definition, err := nativeMarketingSkillDefinition(item)
		require.NoError(t, err)
		body, err := json.Marshal(definition)
		require.NoError(t, err)
		require.True(t, nativeMarketingDefinitionMatches(datatypes.JSON(body), definition), item.SkillID)
		var changed map[string]any
		require.NoError(t, json.Unmarshal(body, &changed))
		changed["executor"].(map[string]any)["output_mode"] = "changed"
		body, err = json.Marshal(changed)
		require.NoError(t, err)
		require.False(t, nativeMarketingDefinitionMatches(datatypes.JSON(body), definition))
	}
}

func TestNativeMarketingPolicyDoesNotOfferTemporalTokenAsLeadMetric(t *testing.T) {
	policy, err := nativeMarketingCalculationPolicy()
	require.NoError(t, err)
	parsed, err := evidence.ReadCalculationPolicy(policy)
	require.NoError(t, err)
	payload := map[string]any{"message": "激活近6个月内未续费客户。活动投入34.2万元，活动标记GMV46.2万元，财务口径ROI为1.35。"}
	profiles, err := parsed.DetectProfiles(payload, []string{"/message"}, "zh-CN")
	require.NoError(t, err)
	require.Equal(t, []string{"transaction_retention"}, profiles)
	tokens, err := evidence.TokenizeSources(payload, []string{"/message"}, parsed)
	require.NoError(t, err)
	schema := evidence.SelectionJSONSchema(parsed, tokens, profiles, "zh-CN")
	fields := schema["properties"].(map[string]any)["data"].(map[string]any)["properties"].(map[string]any)
	require.Contains(t, fields, "spend")
	require.Contains(t, fields, "gmv")
	require.NotContains(t, fields, "visits")
	require.NotContains(t, fields, "target_leads")
	for _, token := range tokens {
		require.False(t, strings.Contains(token.Source.Quote, "6个月") && token.Unit == "个")
	}
}

func TestNativeMarketingRepurchaseCountsRemainComputableWithDeclaredScope(t *testing.T) {
	raw, err := nativeMarketingCalculationPolicy()
	require.NoError(t, err)
	policy, err := evidence.ReadCalculationPolicy(raw)
	require.NoError(t, err)
	payload := map[string]any{"message": "复购率27.8%，复购客户278人，复购客户池1000人。"}
	profiles, err := policy.DetectProfiles(payload, []string{"/message"}, "zh-CN")
	require.NoError(t, err)
	tokens, err := evidence.TokenizeSources(payload, []string{"/message"}, policy)
	require.NoError(t, err)
	schema := evidence.SelectionJSONSchema(policy, tokens, profiles, "zh-CN")
	fields := schema["properties"].(map[string]any)["data"].(map[string]any)["properties"].(map[string]any)
	selection := evidence.SourceSelection{Schema: evidence.ExtractionSchema, Data: map[string]evidence.SelectedValue{}}
	for key, value := range fields {
		props := value.(map[string]any)["properties"].(map[string]any)
		refs := props["token_ref"].(map[string]any)["enum"].([]string)
		require.Len(t, refs, 1)
		selection.Data[key] = evidence.SelectedValue{Scope: props["scope"].(map[string]any)["const"].(string), TokenRef: refs[0]}
	}
	extraction, err := evidence.ResolveSelection(selection, tokens, policy, profiles, "zh-CN")
	require.NoError(t, err)
	plan, gaps, err := policy.BuildPlan(extraction, profiles, "zh-CN")
	require.NoError(t, err)
	require.Empty(t, gaps)
	report, err := evidence.ExecutePlan(evidence.WithLedger(context.Background()), plan, payload, []string{"/message"}, uuid.NewString(), uuid.NewString(), uuid.NewString())
	require.NoError(t, err)
	body, err := json.Marshal(report)
	require.NoError(t, err)
	var decoded evidence.Report
	require.NoError(t, evidence.Decode(body, &decoded))
	require.Equal(t, "27.8%", decoded.Presentation.Computed[0].DisplayValue)
	require.Empty(t, decoded.Presentation.Conflicts)
}
