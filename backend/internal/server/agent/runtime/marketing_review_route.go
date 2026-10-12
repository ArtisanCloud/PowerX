package runtime

import (
	"context"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/server/agent"
	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
)

const marketingReviewSynthesisSkillID = "marketing.review_summarize"

// deterministicMarketingReviewTasks owns only the narrow hand-off from a
// campaign-review material to its declared synthesis Skill. The LLM planner is
// intentionally not consulted here: selecting zero tasks for the same evidence
// input makes the review contract non-repeatable. The Skill must still be in
// the current Agent's binding-derived candidate set; this rule never bypasses
// binding, tenant, or source-scope authorization.
func (e *Engine) deterministicMarketingReviewTasks(ctx context.Context, message string) ([]flowschema.DetectedTask, bool) {
	if e == nil || e.mgr == nil || !isMarketingReviewMaterial(message) {
		return nil, false
	}
	cctx := agent.CandidateBuildContextFromRequest(ctx)
	if !containsBoundMarketingReviewSkill(cctx.BoundSkillIDs) {
		return nil, false
	}
	candidates := e.mgr.BuildToolCallCandidatesWithContext(cctx, 0)
	for _, candidate := range candidates {
		if !strings.EqualFold(strings.TrimSpace(candidate.NodeKind), "skill") ||
			!strings.EqualFold(strings.TrimSpace(candidate.NodeRef), marketingReviewSynthesisSkillID) {
			continue
		}
		return []flowschema.DetectedTask{{
			TaskID:   "marketing_review_synthesis",
			FlowID:   candidate.NodeRef,
			AgentID:  candidate.AgentID,
			Score:    1,
			Strategy: "deterministic:marketing_campaign_review",
			Reason:   "marketing_campaign_review_material",
			Params: map[string]any{
				"_candidate_name": candidate.Name,
				"_candidate_desc": candidate.Description,
				"_node_kind":      candidate.NodeKind,
				"_node_ref":       candidate.NodeRef,
				"_source_scope":   candidate.SourceScope,
				"payload":         map[string]any{"message": strings.TrimSpace(message)},
				"user_message":    strings.TrimSpace(message),
			},
		}}, true
	}
	return nil, false
}

func isMarketingReviewMaterial(message string) bool {
	text := strings.ToLower(strings.TrimSpace(message))
	activity := containsMarketingReviewMarker(text, []string{
		"营销", "市场活动", "活动", "投放", "渠道", "campaign", "marketing", "promotion",
	})
	review := containsMarketingReviewMarker(text, []string{
		"复盘", "回顾", "分析", "review", "retrospective",
	})
	metric := containsMarketingReviewMarker(text, []string{
		"roi", "gmv", "ctr", "cvr", "转化", "投入", "预算", "成本", "营收", "复购", "留存", "归因", "曝光", "点击", "%", "万元",
	})
	return (activity && review) || (activity && metric)
}

func containsMarketingReviewMarker(text string, markers []string) bool {
	for _, marker := range markers {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func containsBoundMarketingReviewSkill(skillIDs []string) bool {
	for _, skillID := range skillIDs {
		if strings.EqualFold(strings.TrimSpace(skillID), marketingReviewSynthesisSkillID) {
			return true
		}
	}
	return false
}
