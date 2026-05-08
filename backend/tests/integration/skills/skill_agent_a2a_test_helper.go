package skillsintegration

import (
	"context"
	"errors"

	agentschema "github.com/ArtisanCloud/PowerX/internal/server/agent/schemas"
	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
)

type a2aStubAgent struct {
	multiPlanStubAgent
	failFlows map[string]bool
}

func (s *a2aStubAgent) Invoke(ctx context.Context, flowID string, params flowschema.Context, meta agentschema.ExecutionMeta) (*agentschema.ExecutionResult, error) {
	if s.failFlows != nil && s.failFlows[flowID] {
		return nil, errors.New("mock flow failed")
	}
	return s.multiPlanStubAgent.Invoke(ctx, flowID, params, meta)
}
