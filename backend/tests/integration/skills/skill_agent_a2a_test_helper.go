package skillsintegration

import (
	"context"
	"errors"

	agentpkg "github.com/ArtisanCloud/PowerX/internal/server/agent"
	agentschema "github.com/ArtisanCloud/PowerX/internal/server/agent/schemas"
	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
)

type a2aStubAgent struct {
	multiPlanStubAgent
	failFlows map[string]bool
}

func newA2AHandoffInvoker(failFlows map[string]bool) agentpkg.AgentHandoffInvoker {
	return func(_ context.Context, in agentpkg.AgentHandoffInput) (*agentpkg.AgentHandoffOutput, error) {
		if failFlows != nil && failFlows[in.FlowID] {
			return nil, errors.New("mock handoff failed")
		}
		return &agentpkg.AgentHandoffOutput{
			TaskID:         in.TaskID,
			HandoffTraceID: in.HandoffTraceID,
			Status:         "completed",
			Result:         map[string]any{"flow_id": in.FlowID},
		}, nil
	}
}

func (s *a2aStubAgent) Invoke(ctx context.Context, flowID string, params flowschema.Context, meta agentschema.ExecutionMeta) (*agentschema.ExecutionResult, error) {
	if s.failFlows != nil && s.failFlows[flowID] {
		return nil, errors.New("mock flow failed")
	}
	return s.multiPlanStubAgent.Invoke(ctx, flowID, params, meta)
}
