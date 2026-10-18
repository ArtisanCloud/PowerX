package skillsintegration

import (
	"context"
	"fmt"
	"testing"

	agentpkg "github.com/ArtisanCloud/PowerX/internal/server/agent"
	"github.com/stretchr/testify/require"
)

func TestSkillAgentPlannerLatencyAndTokenRegression(t *testing.T) {
	m := agentpkg.NewAgentManager()
	m.SetPlannerOptimizerConfig(agentpkg.PlannerOptimizerConfig{
		Enabled:       true,
		CandidateTopK: 32,
		PerKindQuota: agentpkg.PlannerKindQuota{
			Workflow: 8,
			Skill:    16,
			Tooling:  16,
			LLM:      8,
		},
		PromptSlimMode:       "compact",
		DecisionCacheEnabled: true,
		DecisionCacheTTLSec:  60,
	})

	// 关键技能候选
	m.UpsertUnifiedCandidate(agentpkg.ToolCallCandidate{
		Name:          "skill.thirdparty.hello-echo",
		NodeKind:      "skill",
		NodeRef:       "skill.thirdparty.hello-echo",
		SourceScope:   "system",
		BindingStatus: "active",
		Description:   "The smallest demo skill package for installation smoke tests.",
		OptionalArgs:  []string{"text"},
	})
	m.UpsertUnifiedCandidate(agentpkg.ToolCallCandidate{
		Name:          "skill.thirdparty.prompt-template",
		NodeKind:      "skill",
		NodeRef:       "skill.thirdparty.prompt-template",
		SourceScope:   "system",
		BindingStatus: "active",
		Description:   "Render text prompt templates with variables for agent pre-processing.",
		RequiredArgs:  []string{"template"},
		OptionalArgs:  []string{"variables"},
	})

	// 噪声候选（模拟线上规模）
	for i := 0; i < 620; i++ {
		m.UpsertUnifiedCandidate(agentpkg.ToolCallCandidate{
			Name:          fmt.Sprintf("com.corex.tooling.noise.%03d", i),
			NodeKind:      "tooling",
			NodeRef:       fmt.Sprintf("com.corex.tooling.noise.%03d", i),
			SourceScope:   "system",
			BindingStatus: "active",
			Description:   "Generated from proto",
		})
	}

	boundTools := make([]string, 620)
	for i := range boundTools {
		boundTools[i] = fmt.Sprintf("com.corex.tooling.noise.%03d", i)
	}
	ctx := context.Background()
	cctx := agentpkg.CandidateBuildContext{
		TenantUUID:    "tenant-planner-regression",
		AgentID:       "agent-planner-regression",
		BoundSkillIDs: []string{"skill.thirdparty.hello-echo", "skill.thirdparty.prompt-template"},
		BoundToolIDs:  boundTools,
	}

	snapEcho := m.EvaluatePlannerOptimization(ctx, `把 INC-1001 原样返回给我。`, cctx)
	snapTemplate := m.EvaluatePlannerOptimization(ctx, `请使用 prompt-template 输出：事故 {{id}} 影响 {{scope}}，修复建议 {{action}}。其中 id=INC-1001，scope=华东支付，action=先回滚 v2.3.7。`, cctx)

	require.Greater(t, snapEcho.CandidatesBefore, snapEcho.CandidatesAfter)
	require.Greater(t, snapTemplate.CandidatesBefore, snapTemplate.CandidatesAfter)
	require.Greater(t, snapEcho.PromptTokensBefore, snapEcho.PromptTokensAfter)
	require.Greater(t, snapTemplate.PromptTokensBefore, snapTemplate.PromptTokensAfter)

	// 调度/编译负载会影响单次毫秒采样；保留容量边界，避免把 2ms 噪声当算法回退。
	require.LessOrEqual(t, snapEcho.BuildLatencyAfterMS, max(snapEcho.BuildLatencyBeforeMS*4, 50))
	require.LessOrEqual(t, snapTemplate.BuildLatencyAfterMS, max(snapTemplate.BuildLatencyBeforeMS*4, 50))
}
