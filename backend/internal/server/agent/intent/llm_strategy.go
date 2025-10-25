package intent

import (
	"context"
	"github.com/ArtisanCloud/PowerX/internal/server/agent"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/intent/llm"
	"github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
)

type LLMStrategy struct {
	M         *agent.Manager
	AgentID   string
	LLM       llm.Classifier
	Threshold float64 // 命中阈值
}

func (s *LLMStrategy) Name() string { return "llm" }

func (s *LLMStrategy) Match(ctx context.Context, text string) (*schemas.IntentResult, error) {
	specs := s.M.ListFlowRoutesByAgent(s.AgentID)
	if len(specs) == 0 {
		return &schemas.IntentResult{Matched: false, Strategy: s.Name(), Reason: "no candidates"}, nil
	}

	// 组装候选
	cands := make([]llm.Candidate, 0, len(specs))
	for _, sp := range specs {
		hints := sp.Examples.Positive
		if len(hints) > 5 {
			hints = hints[:5]
		} // 限长，降低 token
		cands = append(cands, llm.Candidate{
			FlowID: sp.FlowID,
			Name:   sp.Name,
			Hints:  hints,
		})
	}

	res, err := s.LLM.Classify(ctx, text, cands)
	if err != nil {
		return &schemas.IntentResult{Matched: false, Strategy: s.Name(), Reason: "llm error: " + err.Error()}, nil
	}
	_, agentID := s.M.GetIntentSpecByFlow(res.FlowID)
	if res.Confidence >= s.Threshold && res.FlowID != "" {
		return &schemas.IntentResult{
			Matched:  true,
			FlowID:   res.FlowID,
			AgentID:  agentID,
			Score:    res.Confidence,
			Strategy: s.Name(),
			Reason:   res.Reason,
		}, nil
	}
	return &schemas.IntentResult{
		Matched:  false,
		FlowID:   res.FlowID,
		AgentID:  agentID,
		Score:    res.Confidence,
		Strategy: s.Name(),
		Reason:   "below threshold",
	}, nil
}
