package skillsintegration

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	agentpkg "github.com/ArtisanCloud/PowerX/internal/server/agent"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/contract"
	agentschema "github.com/ArtisanCloud/PowerX/internal/server/agent/schemas"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/runtime"
	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/stretchr/testify/require"
)

type compositeIntentStrategy struct {
	agentID string
}

func (s *compositeIntentStrategy) Name() string { return "composite-intent-test" }

func (s *compositeIntentStrategy) Match(_ context.Context, text string) (*flowschema.IntentResult, error) {
	lower := strings.ToLower(strings.TrimSpace(text))
	switch {
	case strings.Contains(lower, "skill"):
		return &flowschema.IntentResult{
			Matched:  true,
			FlowID:   "skill.thirdparty.compose",
			AgentID:  s.agentID,
			Score:    0.92,
			Strategy: "candidate_recall:skill",
			Reason:   "skill clause matched",
		}, nil
	case strings.Contains(lower, "tooling"):
		return &flowschema.IntentResult{
			Matched:  true,
			FlowID:   "capability.compose",
			AgentID:  s.agentID,
			Score:    0.91,
			Strategy: "candidate_recall:tooling",
			Reason:   "tooling clause matched",
		}, nil
	case strings.Contains(lower, "workflow"):
		return &flowschema.IntentResult{
			Matched:  true,
			FlowID:   "workflow.compose",
			AgentID:  s.agentID,
			Score:    0.95,
			Strategy: "rule",
			Reason:   "workflow clause matched",
		}, nil
	default:
		return &flowschema.IntentResult{
			Matched:  false,
			Strategy: "composite-intent-test",
			Reason:   "no match",
		}, nil
	}
}

var _ contract.IntentStrategy = (*compositeIntentStrategy)(nil)

type collectSink struct {
	events []string
	data   []map[string]any
}

func (s *collectSink) Emit(event string, payload any) error {
	s.events = append(s.events, event)
	if m, ok := payload.(map[string]any); ok {
		s.data = append(s.data, m)
		return nil
	}
	s.data = append(s.data, map[string]any{})
	return nil
}

func TestSkillAgentCompositePlanExecuteWithEventSourceScope(t *testing.T) {
	m := agentpkg.GetAgentManager()
	agentID := fmt.Sprintf("agent.composite.%d", time.Now().UnixNano())
	stub := &multiPlanStubAgent{}
	require.NoError(t, m.Register(agentID, stub, &agentschema.AgentMeta{FlowID: "workflow.compose"}))
	require.NoError(t, m.SetDefaultAgent(agentID, "workflow.compose"))
	require.NoError(t, m.RegisterFlowRoute(agentID, "workflow.compose", &flowschema.IntentSpec{
		FlowID: "workflow.compose",
		Metadata: &flowschema.FlowMetadata{
			Requires: []string{"skill.thirdparty.compose", "capability.compose"},
		},
	}))

	m.SetIntentStrategies([]contract.IntentStrategy{
		&compositeIntentStrategy{agentID: agentID},
	}, 0.6, 0.95)
	t.Cleanup(func() {
		m.SetIntentStrategies(nil, 0.6, 0.95)
	})

	m.SetSkillInvoker(func(ctx context.Context, in agentpkg.SkillInvokeInput) (*agentpkg.SkillInvokeOutput, error) {
		return &agentpkg.SkillInvokeOutput{
			TraceID:      "trace-skill-composite",
			Status:       "completed",
			ProtocolUsed: "skill",
			FallbackUsed: false,
			SkillID:      in.SkillID,
			Version:      "1.0.0",
			Result: map[string]any{
				"content": "skill done",
			},
		}, nil
	})
	m.SetToolingInvoker(func(ctx context.Context, in agentpkg.ToolingInvokeInput) (*agentpkg.ToolingInvokeOutput, error) {
		return &agentpkg.ToolingInvokeOutput{
			TraceID:      "trace-tooling-composite",
			Status:       "completed",
			ProtocolUsed: "http",
			FallbackUsed: false,
			Result: map[string]any{
				"content": "tooling done",
			},
		}, nil
	})

	sink := &collectSink{}
	out, plan, err := runtime.NewEngine().RunPlanInvoke(
		context.Background(),
		"先执行 skill 步骤，然后执行 tooling 步骤，最后 workflow 汇总",
		&dto.ChatConfig{},
		"",
		sink,
	)
	require.NoError(t, err)
	require.NotNil(t, out)
	require.NotNil(t, plan)
	require.NotEmpty(t, plan.Tasks)

	byFlow := map[string]flowschema.PlanTask{}
	for _, task := range plan.Tasks {
		byFlow[task.FlowID] = task
	}
	require.Equal(t, "skill", byFlow["skill.thirdparty.compose"].NodeKind)
	require.Equal(t, "tooling", byFlow["capability.compose"].NodeKind)
	require.Equal(t, "workflow", byFlow["workflow.compose"].NodeKind)
	require.Equal(t, "system", byFlow["workflow.compose"].SourceScope)
	require.Len(t, byFlow["workflow.compose"].DependsOn, 2)

	var hasSkillNodeStart, hasToolingNodeEnd bool
	for i, evt := range sink.events {
		if evt != dto.EventNodeStart && evt != dto.EventNodeEnd {
			continue
		}
		payload := sink.data[i]
		kind := strings.TrimSpace(fmt.Sprintf("%v", payload["node_kind"]))
		ref := strings.TrimSpace(fmt.Sprintf("%v", payload["node_ref"]))
		scope := strings.TrimSpace(fmt.Sprintf("%v", payload["source_scope"]))
		require.NotEmpty(t, kind)
		require.NotEmpty(t, ref)
		require.NotEmpty(t, scope)
		if evt == dto.EventNodeStart && kind == "skill" {
			hasSkillNodeStart = true
		}
		if evt == dto.EventNodeEnd && kind == "tooling" {
			hasToolingNodeEnd = true
		}
	}
	require.True(t, hasSkillNodeStart)
	require.True(t, hasToolingNodeEnd)
}

