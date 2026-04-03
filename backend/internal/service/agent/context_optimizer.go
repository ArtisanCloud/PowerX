package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	agentcfg "github.com/ArtisanCloud/PowerX/internal/server/agent/config"
	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
)

type ContextTrimAction struct {
	Layer        string `json:"layer"`
	Action       string `json:"action"`
	Reason       string `json:"reason"`
	TokensBefore int    `json:"tokens_before"`
	TokensAfter  int    `json:"tokens_after"`
}

type ContextBuildResult struct {
	Enabled            bool                `json:"enabled"`
	SystemPrompt       string              `json:"system_prompt"`
	LayerTokenSize     map[string]int      `json:"context_layers_size"`
	PromptTokens       int                 `json:"prompt_tokens"`
	CompletionReserve  int                 `json:"reserved_completion_tokens"`
	TrimActions        []ContextTrimAction `json:"trim_actions"`
	UsedStructuredMemo bool                `json:"used_structured_summary"`
}

type SessionStructuredSummary struct {
	Schema      string   `json:"schema"`
	Facts       []string `json:"facts,omitempty"`
	Decisions   []string `json:"decisions,omitempty"`
	OpenIssues  []string `json:"open_issues,omitempty"`
	Constraints []string `json:"constraints,omitempty"`
	UpdatedAt   string   `json:"updated_at,omitempty"`
}

const structuredSummarySchemaV1 = "powerx.agent.summary.v1"

func (s *ChatHistoryService) BuildContextForLLM(
	ctx context.Context,
	env string,
	tenantUUID *string,
	session *dbmodel.AgentChatSession,
	userInput string,
	baseSystemPrompt string,
	candidateSummary string,
	retrievalSnippets []string,
	optimizerCfg agentcfg.ContextOptimizerConfig,
) (*ContextBuildResult, error) {
	res := &ContextBuildResult{
		Enabled:           optimizerCfg.Enabled,
		SystemPrompt:      strings.TrimSpace(baseSystemPrompt),
		LayerTokenSize:    map[string]int{},
		CompletionReserve: defaultPositive(optimizerCfg.ReservedCompletionTokens, 1200),
		TrimActions:       make([]ContextTrimAction, 0, 4),
	}
	if !optimizerCfg.Enabled || session == nil {
		return res, nil
	}

	recentLimit := defaultPositive(optimizerCfg.RecentMessages, 8)
	retrievalLimit := defaultPositive(optimizerCfg.RetrievalTopK, 6)
	budget := defaultPositive(optimizerCfg.MaxPromptTokens, 12000)

	recentMsgs, _ := s.msg.ListLatestN(ctx, env, tenantUUID, session.ID, recentLimit)
	l0 := strings.TrimSpace(baseSystemPrompt)
	l1 := strings.TrimSpace(candidateSummary)
	summaryText, structured := renderSessionSummary(session.Summary)
	l2 := strings.TrimSpace(summaryText)
	l3 := strings.TrimSpace(renderRecentMessages(recentMsgs, recentLimit))
	l4 := strings.TrimSpace(renderRetrievalSnippets(retrievalSnippets, retrievalLimit))
	l5 := strings.TrimSpace(userInput)

	res.UsedStructuredMemo = structured
	res.LayerTokenSize["L0"] = estimateTokens(l0)
	res.LayerTokenSize["L1"] = estimateTokens(l1)
	res.LayerTokenSize["L2"] = estimateTokens(l2)
	res.LayerTokenSize["L3"] = estimateTokens(l3)
	res.LayerTokenSize["L4"] = estimateTokens(l4)
	res.LayerTokenSize["L5"] = estimateTokens(l5)

	target := budget - res.CompletionReserve
	if target < 512 {
		target = 512
	}

	sumPrompt := func() int {
		return estimateTokens(l0) + estimateTokens(l1) + estimateTokens(l2) + estimateTokens(l3) + estimateTokens(l4) + estimateTokens(l5)
	}

	total := sumPrompt()
	if total > target && l4 != "" {
		before := estimateTokens(l4)
		l4 = trimLinesToTokens(l4, maxInt(target-total+before, before/2))
		after := estimateTokens(l4)
		if after < before {
			res.TrimActions = append(res.TrimActions, ContextTrimAction{
				Layer:        "L4",
				Action:       "trim",
				Reason:       "over_budget",
				TokensBefore: before,
				TokensAfter:  after,
			})
		}
	}
	total = sumPrompt()
	if total > target && l3 != "" {
		before := estimateTokens(l3)
		l3 = trimLinesToTokensFromTail(l3, maxInt(target-total+before, before/2))
		after := estimateTokens(l3)
		if after < before {
			res.TrimActions = append(res.TrimActions, ContextTrimAction{
				Layer:        "L3",
				Action:       "trim",
				Reason:       "over_budget",
				TokensBefore: before,
				TokensAfter:  after,
			})
		}
	}
	total = sumPrompt()
	if total > target && l2 != "" {
		before := estimateTokens(l2)
		l2 = trimRunes(l2, maxInt(len([]rune(l2))/2, 120))
		after := estimateTokens(l2)
		if after < before {
			res.TrimActions = append(res.TrimActions, ContextTrimAction{
				Layer:        "L2",
				Action:       "trim",
				Reason:       "over_budget",
				TokensBefore: before,
				TokensAfter:  after,
			})
		}
	}

	segments := make([]string, 0, 5)
	if l0 != "" {
		segments = append(segments, l0)
	}
	if l1 != "" {
		segments = append(segments, "[CONTEXT-L1 CAPABILITIES]\n"+l1)
	}
	if l2 != "" {
		segments = append(segments, "[CONTEXT-L2 MEMORY]\n"+l2)
	}
	if l3 != "" {
		segments = append(segments, "[CONTEXT-L3 RECENT]\n"+l3)
	}
	if l4 != "" {
		segments = append(segments, "[CONTEXT-L4 RETRIEVAL]\n"+l4)
	}
	res.SystemPrompt = strings.TrimSpace(strings.Join(segments, "\n\n"))
	res.PromptTokens = sumPrompt()
	res.LayerTokenSize["L0"] = estimateTokens(l0)
	res.LayerTokenSize["L1"] = estimateTokens(l1)
	res.LayerTokenSize["L2"] = estimateTokens(l2)
	res.LayerTokenSize["L3"] = estimateTokens(l3)
	res.LayerTokenSize["L4"] = estimateTokens(l4)
	res.LayerTokenSize["L5"] = estimateTokens(l5)
	return res, nil
}

func renderSessionSummary(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	var st SessionStructuredSummary
	if err := json.Unmarshal([]byte(raw), &st); err == nil && strings.TrimSpace(st.Schema) == structuredSummarySchemaV1 {
		parts := make([]string, 0, 4)
		if len(st.Facts) > 0 {
			parts = append(parts, "facts: "+strings.Join(st.Facts, " | "))
		}
		if len(st.Decisions) > 0 {
			parts = append(parts, "decisions: "+strings.Join(st.Decisions, " | "))
		}
		if len(st.OpenIssues) > 0 {
			parts = append(parts, "open_issues: "+strings.Join(st.OpenIssues, " | "))
		}
		if len(st.Constraints) > 0 {
			parts = append(parts, "constraints: "+strings.Join(st.Constraints, " | "))
		}
		return strings.Join(parts, "\n"), true
	}
	return raw, false
}

func renderRecentMessages(msgs []dbmodel.AgentChatMessage, limit int) string {
	if len(msgs) == 0 {
		return ""
	}
	if limit > 0 && len(msgs) > limit {
		msgs = msgs[len(msgs)-limit:]
	}
	lines := make([]string, 0, len(msgs))
	for _, m := range msgs {
		role := strings.TrimSpace(m.Role)
		if role == "" {
			role = "msg"
		}
		content := strings.TrimSpace(m.Content)
		if content == "" {
			continue
		}
		content = trimRunes(content, 240)
		lines = append(lines, fmt.Sprintf("- %s: %s", role, content))
	}
	return strings.Join(lines, "\n")
}

func renderRetrievalSnippets(items []string, topK int) string {
	if len(items) == 0 {
		return ""
	}
	if topK > 0 && len(items) > topK {
		items = items[:topK]
	}
	lines := make([]string, 0, len(items))
	for i, it := range items {
		t := strings.TrimSpace(it)
		if t == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("- R%d: %s", i+1, trimRunes(t, 280)))
	}
	return strings.Join(lines, "\n")
}

func estimateTokens(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	n := utf8.RuneCountInString(s)
	if n <= 0 {
		return 0
	}
	return n/4 + 1
}

func trimLinesToTokens(s string, maxTokens int) string {
	if maxTokens <= 0 {
		return ""
	}
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	cur := 0
	for _, ln := range lines {
		t := estimateTokens(ln)
		if t == 0 {
			continue
		}
		if cur+t > maxTokens {
			break
		}
		out = append(out, ln)
		cur += t
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func trimLinesToTokensFromTail(s string, maxTokens int) string {
	if maxTokens <= 0 {
		return ""
	}
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	cur := 0
	for i := len(lines) - 1; i >= 0; i-- {
		ln := lines[i]
		t := estimateTokens(ln)
		if t == 0 {
			continue
		}
		if cur+t > maxTokens {
			break
		}
		out = append([]string{ln}, out...)
		cur += t
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func trimRunes(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	rs := []rune(strings.TrimSpace(s))
	if len(rs) <= maxRunes {
		return string(rs)
	}
	return strings.TrimSpace(string(rs[:maxRunes])) + "..."
}

func defaultPositive(v, d int) int {
	if v > 0 {
		return v
	}
	return d
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
