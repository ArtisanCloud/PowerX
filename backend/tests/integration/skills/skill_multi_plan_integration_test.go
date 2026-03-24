package skillsintegration

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	agentpkg "github.com/ArtisanCloud/PowerX/internal/server/agent"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/contract"
	agentschema "github.com/ArtisanCloud/PowerX/internal/server/agent/schemas"
	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"
)

type multiPlanStubAgent struct {
	mu    sync.Mutex
	calls []string
}

func (s *multiPlanStubAgent) GetInfo() *agentschema.AgentInfo {
	return &agentschema.AgentInfo{Name: "stub"}
}
func (s *multiPlanStubAgent) ListFlows(context.Context, agentschema.ExecutionMeta) ([]agentschema.FlowRuntimeInfo, error) {
	return nil, nil
}
func (s *multiPlanStubAgent) GetFlowInfo(context.Context, string, agentschema.ExecutionMeta) (*agentschema.FlowRuntimeInfo, error) {
	return nil, nil
}
func (s *multiPlanStubAgent) ValidateParams(context.Context, string, flowschema.Context, agentschema.ExecutionMeta) error {
	return nil
}
func (s *multiPlanStubAgent) Invoke(ctx context.Context, flowID string, params flowschema.Context, meta agentschema.ExecutionMeta) (*agentschema.ExecutionResult, error) {
	s.mu.Lock()
	s.calls = append(s.calls, flowID)
	s.mu.Unlock()

	if flowID == "flow.alpha" || flowID == "flow.beta" {
		time.Sleep(15 * time.Millisecond)
	}
	depsCount := 0
	if deps, ok := params["_deps"].(map[string]*agentschema.ExecutionResult); ok {
		depsCount = len(deps)
	}
	return &agentschema.ExecutionResult{
		Success: true,
		Data: flowschema.Result{
			"id":         fmt.Sprintf("id-%s", flowID),
			"flow_id":    flowID,
			"deps_count": depsCount,
		},
		Metadata: flowschema.Result{"flow_id": flowID, "is_final": true},
	}, nil
}
func (s *multiPlanStubAgent) InvokeAsync(context.Context, string, flowschema.Context, agentschema.ExecutionMeta) (string, error) {
	return "", nil
}
func (s *multiPlanStubAgent) Stream(context.Context, string, flowschema.Context, agentschema.ExecutionMeta) (*schema.StreamReader[*agentschema.ExecutionResult], error) {
	return nil, nil
}
func (s *multiPlanStubAgent) GetExecutionStatus(context.Context, string, agentschema.ExecutionMeta) (*agentschema.ExecutionStatus, error) {
	return nil, nil
}
func (s *multiPlanStubAgent) CancelExecution(context.Context, string, agentschema.ExecutionMeta) error {
	return nil
}
func (s *multiPlanStubAgent) GetExecutionResult(context.Context, string, agentschema.ExecutionMeta) (*agentschema.ExecutionResult, error) {
	return nil, nil
}
func (s *multiPlanStubAgent) GetMetrics(context.Context, agentschema.ExecutionMeta) (flowschema.Result, error) {
	return flowschema.Result{}, nil
}
func (s *multiPlanStubAgent) Health(context.Context) error   { return nil }
func (s *multiPlanStubAgent) Shutdown(context.Context) error { return nil }

var _ contract.AgentClient = (*multiPlanStubAgent)(nil)

func TestSkillMultiPlanParallelExecutionConsistency(t *testing.T) {
	m := agentpkg.NewAgentManager()
	stub := &multiPlanStubAgent{}
	require.NoError(t, m.Register("agent.stub", stub, &agentschema.AgentMeta{FlowID: "flow.alpha"}))
	require.NoError(t, m.SetDefaultAgent("agent.stub", "flow.alpha"))

	plan := flowschema.ExecutionPlan{
		PlanID: "plan-multi-consistency",
		Tasks: []flowschema.PlanTask{
			{TaskID: "t-alpha", FlowID: "flow.alpha", AgentID: "agent.stub", Stage: 1},
			{TaskID: "t-beta", FlowID: "flow.beta", AgentID: "agent.stub", Stage: 1},
			{TaskID: "t-final", FlowID: "flow.final", AgentID: "agent.stub", Stage: 2, DependsOn: []string{"t-alpha", "t-beta"}},
		},
	}

	out, err := m.ExecutePlan(context.Background(), plan, agentschema.ExecutionMeta{RequestID: "req-multi"})
	require.NoError(t, err)
	require.NotNil(t, out)
	require.True(t, out.Success)
	require.Equal(t, "flow.final", out.Data["flow_id"])
	require.Equal(t, 2, out.Data["deps_count"])

	stub.mu.Lock()
	defer stub.mu.Unlock()
	require.Len(t, stub.calls, 3)
	require.Contains(t, stub.calls, "flow.alpha")
	require.Contains(t, stub.calls, "flow.beta")
	require.Equal(t, "flow.final", stub.calls[2])
}
