package intent

import (
	"context"
	"github.com/ArtisanCloud/PowerX/internal/server/agent"
	"github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
)

// services/agent/intent/rule_strategy.go

type RuleStrategy struct {
	M *agent.Manager
}

func (s *RuleStrategy) Name() string { return "rule" }

func (s *RuleStrategy) Match(ctx context.Context, text string) (*schemas.IntentResult, error) {
	return s.M.RuleMatch(ctx, text) // 调 Manager 的封装（见下文暴露）
}
