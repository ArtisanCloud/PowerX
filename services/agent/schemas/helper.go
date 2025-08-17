package schemas

import "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"

func IntentFromTask(t *schemas.DetectedTask) *schemas.IntentResult {
	if t == nil {
		return &schemas.IntentResult{Matched: false}
	}
	return &schemas.IntentResult{
		Matched:  true,
		FlowID:   t.FlowID,
		AgentID:  t.AgentID,
		Score:    t.Score,
		Strategy: t.Strategy,
		Reason:   t.Reason,
	}
}
