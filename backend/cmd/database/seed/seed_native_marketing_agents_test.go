package seed

import (
	"context"
	"github.com/ArtisanCloud/PowerX/internal/service/skills"
	"github.com/ArtisanCloud/PowerX/pkg/corex/agent/evidence"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
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
