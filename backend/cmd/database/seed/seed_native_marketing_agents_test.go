package seed

import (
	"context"
	"github.com/ArtisanCloud/PowerX/internal/service/skills"
	"github.com/ArtisanCloud/PowerX/pkg/corex/agent/evidence"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
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
