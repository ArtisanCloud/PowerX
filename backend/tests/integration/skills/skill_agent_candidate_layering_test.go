package skillsintegration

import (
	"testing"

	agentpkg "github.com/ArtisanCloud/PowerX/internal/server/agent"
	agentschema "github.com/ArtisanCloud/PowerX/internal/server/agent/schemas"
	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/stretchr/testify/require"
)

func TestSkillAgentCandidateLayering_SystemAgentDedupeAndAuthzFilter(t *testing.T) {
	m := agentpkg.NewAgentManager()
	stub := &multiPlanStubAgent{}
	require.NoError(t, m.Register("agent.system.layering", stub, &agentschema.AgentMeta{FlowID: "workflow.same"}))
	require.NoError(t, m.RegisterFlowRoute("agent.system.layering", "workflow.same", &flowschema.IntentSpec{
		FlowID: "workflow.same",
		Metadata: &flowschema.FlowMetadata{
			Description: "system workflow candidate",
		},
	}))

	// 同名候选：agent custom 应覆盖 system builtin。
	m.UpsertUnifiedCandidate(agentpkg.ToolCallCandidate{
		Name:         "workflow.same",
		NodeKind:     "workflow",
		NodeRef:      "workflow.same.agent",
		FlowID:       "workflow.same.agent",
		AgentID:      "agent.custom.layering",
		SourceScope:  "agent",
		Source:       "builtin",
		Visibility:   "tenant",
		BindingStatus:"active",
		Description:  "agent custom workflow candidate",
	})

	// 未授权候选：source 与 tool_grants 均不满足，应被过滤掉。
	m.UpsertUnifiedCandidate(agentpkg.ToolCallCandidate{
		Name:           "tooling.denied",
		NodeKind:       "tooling",
		NodeRef:        "cap.tooling.denied",
		SourceScope:    "agent",
		Source:         "third_party",
		Visibility:     "tenant",
		BindingStatus:  "active",
		RequiredGrants: []string{"ops.write"},
	})

	// 授权候选：应保留。
	m.UpsertUnifiedCandidate(agentpkg.ToolCallCandidate{
		Name:          "tooling.allowed",
		NodeKind:      "tooling",
		NodeRef:       "cap.tooling.allowed",
		SourceScope:   "system",
		Source:        "builtin",
		Visibility:    "public",
		BindingStatus: "active",
	})

	cands := m.BuildToolCallCandidatesWithContext(agentpkg.CandidateBuildContext{
		TenantUUID:    "tenant-layering",
		AgentID:       "agent.custom.layering",
		ToolGrantIDs:  []string{"ops.read"},
		AllowedSource: []string{"builtin"},
	}, 0)

	byName := map[string]agentpkg.ToolCallCandidate{}
	for _, c := range cands {
		byName[c.Name] = c
	}

	workflow, ok := byName["workflow.same"]
	require.True(t, ok)
	require.Equal(t, "workflow.same.agent", workflow.NodeRef)
	require.Equal(t, "agent", workflow.SourceScope)

	_, deniedExists := byName["tooling.denied"]
	require.False(t, deniedExists)

	allowed, ok := byName["tooling.allowed"]
	require.True(t, ok)
	require.Equal(t, "tooling", allowed.NodeKind)
	require.Equal(t, "cap.tooling.allowed", allowed.NodeRef)
}

