package runtime

import (
	"context"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/server/agent"
	"github.com/stretchr/testify/require"
)

func TestDeterministicMarketingReviewRouteUsesBoundSynthesisSkill(t *testing.T) {
	mgr := agent.NewAgentManager()
	mgr.UpsertUnifiedCandidate(agent.ToolCallCandidate{
		Name:          marketingReviewSynthesisSkillID,
		NodeKind:      "skill",
		NodeRef:       marketingReviewSynthesisSkillID,
		FlowID:        marketingReviewSynthesisSkillID,
		AgentID:       "114",
		SourceScope:   "system",
		Visibility:    "public",
		BindingStatus: "active",
	})
	engine := &Engine{mgr: mgr}
	ctx := context.WithValue(context.Background(), "agent_bound_skill_ids", []string{marketingReviewSynthesisSkillID})
	message := "老客唤醒活动投入34.2万元，GMV 46.2万元，短信渠道 ROI 0.65、信息流 ROI 4.8，请做活动复盘。"

	tasks, ok := engine.deterministicMarketingReviewTasks(ctx, message)
	require.True(t, ok)
	require.Len(t, tasks, 1)
	require.Equal(t, marketingReviewSynthesisSkillID, tasks[0].FlowID)
	require.Equal(t, message, tasks[0].Params["payload"].(map[string]any)["message"])
}

func TestDeterministicMarketingReviewRouteDoesNotBypassSkillBinding(t *testing.T) {
	mgr := agent.NewAgentManager()
	mgr.UpsertUnifiedCandidate(agent.ToolCallCandidate{
		Name: marketingReviewSynthesisSkillID, NodeKind: "skill", NodeRef: marketingReviewSynthesisSkillID,
		FlowID: marketingReviewSynthesisSkillID, SourceScope: "system", Visibility: "public", BindingStatus: "active",
	})
	engine := &Engine{mgr: mgr}
	ctx := context.WithValue(context.Background(), "agent_bound_skill_ids", []string{"other.skill"})

	tasks, ok := engine.deterministicMarketingReviewTasks(ctx, "本次活动 ROI 只有 0.65，请做复盘并分析渠道归因。")
	require.False(t, ok)
	require.Empty(t, tasks)
}

func TestMarketingReviewMaterialRequiresActivityAndReviewOrMetrics(t *testing.T) {
	require.True(t, isMarketingReviewMaterial("活动投入 10 万元，ROI 为 1.2，请分析渠道归因。"))
	require.True(t, isMarketingReviewMaterial("请对这次营销活动做复盘，梳理问题和下一步。"))
	require.False(t, isMarketingReviewMaterial("请解释 ROI 是什么。"))
}
