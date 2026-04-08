package intent

import (
	"context"
	"github.com/ArtisanCloud/PowerX/internal/server/agent"
	"github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"strings"
)

// services/agent/intent/rule_strategy.go

type RuleStrategy struct {
	M *agent.Manager
}

func (s *RuleStrategy) Name() string { return "rule" }

func (s *RuleStrategy) Match(ctx context.Context, text string) (*schemas.IntentResult, error) {
	trimmed := strings.TrimSpace(text)
	// 规则策略仅用于快捷命令（/command...），普通自然语言交给 LLM/embedding 识别。
	if !strings.HasPrefix(trimmed, "/") {
		return &schemas.IntentResult{
			Matched:  false,
			Strategy: s.Name(),
			Reason:   "rule only for slash command",
		}, nil
	}
	return s.M.RuleMatch(ctx, text) // 调 Manager 的封装（见下文暴露）
}
