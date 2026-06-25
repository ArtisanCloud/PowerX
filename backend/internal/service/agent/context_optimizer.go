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
	UsedContextLayers  []string            `json:"used_context_layers"`
	PromptTokens       int                 `json:"prompt_tokens"`
	CompletionReserve  int                 `json:"reserved_completion_tokens"`
	TrimActions        []ContextTrimAction `json:"trim_actions"`
	UsedStructuredMemo bool                `json:"used_structured_summary"`
}

type ResponseContextOptions struct {
	ResponseMode        string
	TargetCapabilityIDs []string
	IncludeExamples     bool
	IncludeSchema       bool
	RepeatFullIntro     bool
}

type SessionStructuredSummary struct {
	Schema                string   `json:"schema"`
	Facts                 []string `json:"facts,omitempty"`
	Decisions             []string `json:"decisions,omitempty"`
	OpenIssues            []string `json:"open_issues,omitempty"`
	Constraints           []string `json:"constraints,omitempty"`
	SourceSummaryIDs      []string `json:"source_summary_ids,omitempty"`
	FromMessageID         uint64   `json:"from_message_id,omitempty"`
	ToMessageID           uint64   `json:"to_message_id,omitempty"`
	CompressedMessages    int      `json:"compressed_messages,omitempty"`
	RecentMessagesKept    int      `json:"recent_messages_kept,omitempty"`
	CompressionPolicy     string   `json:"compression_policy,omitempty"`
	PreviousSummaryAt     string   `json:"previous_summary_at,omitempty"`
	LastCompressedTraceID string   `json:"last_compressed_trace_id,omitempty"`
	UpdatedAt             string   `json:"updated_at,omitempty"`
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
	return s.BuildContextForLLMWithResponsePlan(ctx, env, tenantUUID, session, userInput, baseSystemPrompt, candidateSummary, retrievalSnippets, optimizerCfg, nil)
}

func (s *ChatHistoryService) BuildContextForLLMWithResponsePlan(
	ctx context.Context,
	env string,
	tenantUUID *string,
	session *dbmodel.AgentChatSession,
	userInput string,
	baseSystemPrompt string,
	candidateSummary string,
	retrievalSnippets []string,
	optimizerCfg agentcfg.ContextOptimizerConfig,
	responseOpt *ResponseContextOptions,
) (*ContextBuildResult, error) {
	res := &ContextBuildResult{
		Enabled:           optimizerCfg.Enabled,
		SystemPrompt:      strings.TrimSpace(baseSystemPrompt),
		LayerTokenSize:    map[string]int{},
		UsedContextLayers: make([]string, 0, 6),
		CompletionReserve: defaultPositive(optimizerCfg.ReservedCompletionTokens, 1200),
		TrimActions:       make([]ContextTrimAction, 0, 4),
	}
	if (!optimizerCfg.Enabled && responseOpt == nil) || session == nil {
		return res, nil
	}

	recentLimit := defaultPositive(optimizerCfg.RecentMessages, 8)
	retrievalLimit := defaultPositive(optimizerCfg.RetrievalTopK, 6)
	budget := defaultPositive(optimizerCfg.MaxPromptTokens, 12000)

	recentMsgs, _ := s.msg.ListLatestN(ctx, env, tenantUUID, session.ID, recentLimit)
	l0 := strings.TrimSpace(baseSystemPrompt)
	l1 := strings.TrimSpace(renderResponseCapabilityContext(candidateSummary, responseOpt))
	summaryText, structured := renderSessionSummary(session.Summary)
	l2 := strings.TrimSpace(summaryText)
	l3 := strings.TrimSpace(renderRecentMessagesWithResponsePlan(recentMsgs, recentLimit, responseOpt))
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
		res.UsedContextLayers = append(res.UsedContextLayers, "agent_profile")
	}
	if l1 != "" {
		segments = append(segments, "[CONTEXT-L1 CAPABILITIES]\n"+l1)
		res.UsedContextLayers = append(res.UsedContextLayers, "capabilities")
	}
	if l2 != "" {
		segments = append(segments, "[CONTEXT-L2 MEMORY]\n"+l2)
		res.UsedContextLayers = append(res.UsedContextLayers, "session_summary")
	}
	if l3 != "" {
		segments = append(segments, "[CONTEXT-L3 RECENT]\n"+l3)
		res.UsedContextLayers = append(res.UsedContextLayers, "recent_messages")
	}
	if l4 != "" {
		segments = append(segments, "[CONTEXT-L4 RETRIEVAL]\n"+l4)
		res.UsedContextLayers = append(res.UsedContextLayers, "retrieval")
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

func renderResponseCapabilityContext(candidateSummary string, opt *ResponseContextOptions) string {
	candidateSummary = strings.TrimSpace(candidateSummary)
	if opt == nil {
		return candidateSummary
	}
	mode := strings.ToLower(strings.TrimSpace(opt.ResponseMode))
	switch mode {
	case "capability_intro":
		if candidateSummary == "" {
			return ""
		}
		if !opt.RepeatFullIntro {
			return strings.Join([]string{
				"当前用户再次询问 Agent 能力，但本会话最近已经完整介绍过。",
				"本轮仍然要回答用户当前问题，并精简列出当前已绑定能力。",
				"只基于下方 BOUND_SKILLS 事实回答；不得新增、推断或泛化未列出的能力。",
				"不得因为去重而丢失用户本轮明确要求的事实；具体业务重点和表达方式由当前 Agent persona、prompt seed 与 Skill metadata 决定。",
				"BOUND_SKILLS:",
				renderBriefCapabilityContext(candidateSummary),
			}, "\n")
		}
		prefix := []string{
			"当前上下文只包含当前 Agent 已绑定、已发布、租户可见且权限通过的能力。",
			"最终回复只能介绍这些能力，不得补充全局候选池或平台内部工具。",
		}
		return strings.Join(append(prefix, candidateSummary), "\n")
	case "capability_howto":
		if candidateSummary == "" {
			return ""
		}
		return strings.Join([]string{
			"下面是用户追问的目标能力详情。请说明怎么使用、需要哪些信息，并给自然语言示例。",
			candidateSummary,
		}, "\n")
	case "clarify_params":
		if candidateSummary == "" {
			return ""
		}
		return strings.Join([]string{
			"下面是目标能力和缺参澄清所需上下文。请询问用户补充必要参数，不要直接执行失败。",
			"回复必须是自然语言追问，不要说“执行失败”，不要要求用户输入 JSON/schema/字段路径。",
			"优先采用能力的回复规范、示例问法和业务描述来命名缺失信息。",
			candidateSummary,
		}, "\n")
	case "skill_execution", "error_explain":
		return ""
	case "normal_chat":
		return ""
	default:
		return candidateSummary
	}
}

func renderBriefCapabilityContext(candidateSummary string) string {
	lines := strings.Split(candidateSummary, "\n")
	out := make([]string, 0, 6)
	inCapability := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "- ") {
			out = append(out, trimmed)
			inCapability = true
			continue
		}
		if !inCapability {
			continue
		}
		switch {
		case strings.HasPrefix(trimmed, "说明:"):
			out = append(out, "  "+trimCandidateContextLine(trimmed, 180))
		case strings.HasPrefix(trimmed, "可用动作:"):
			out = append(out, "  "+trimCandidateContextLine(trimmed, 120))
		case strings.HasPrefix(trimmed, "必要参数:"):
			out = append(out, "  "+trimCandidateContextLine(trimmed, 160))
		case strings.HasPrefix(trimmed, "示例问法:"):
			out = append(out, "  "+trimCandidateContextLine(trimmed, 180))
		case strings.HasPrefix(trimmed, "回复规范:"):
			out = append(out, "  "+trimCandidateContextLine(trimmed, 220))
		case strings.HasPrefix(trimmed, "可选参数:"), strings.HasPrefix(trimmed, "ref:"):
			continue
		default:
			inCapability = false
		}
	}
	if len(out) == 0 {
		return trimCandidateContextLine(candidateSummary, 180)
	}
	if len(out) > 12 {
		out = out[:12]
	}
	return strings.Join(out, "\n")
}

func trimCandidateContextLine(s string, maxRunes int) string {
	s = strings.TrimSpace(s)
	if maxRunes <= 0 {
		return ""
	}
	rs := []rune(s)
	if len(rs) <= maxRunes {
		return s
	}
	return string(rs[:maxRunes]) + "..."
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
	return renderRecentMessagesWithResponsePlan(msgs, limit, nil)
}

func renderRecentMessagesWithResponsePlan(msgs []dbmodel.AgentChatMessage, limit int, opt *ResponseContextOptions) string {
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
		if shouldSkipRecentMessageForResponsePlan(m, opt) {
			continue
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

func shouldSkipRecentMessageForResponsePlan(msg dbmodel.AgentChatMessage, opt *ResponseContextOptions) bool {
	if opt == nil {
		return false
	}
	mode := strings.ToLower(strings.TrimSpace(opt.ResponseMode))
	if mode != "capability_intro" || opt.RepeatFullIntro {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(msg.Role), "assistant") {
		return false
	}
	if !strings.EqualFold(readJSONMapString(msg.Meta, "response_mode"), "capability_intro") {
		return false
	}
	if len(opt.TargetCapabilityIDs) == 0 {
		return true
	}
	historyIDs := readJSONMapStringList(msg.Meta, "capability_ids")
	if len(historyIDs) == 0 {
		historyIDs = readJSONMapNestedStringList(msg.Meta, "response_plan", "target_capability_ids")
	}
	if len(historyIDs) == 0 {
		return true
	}
	targets := map[string]struct{}{}
	for _, id := range opt.TargetCapabilityIDs {
		id = strings.ToLower(strings.TrimSpace(id))
		if id != "" {
			targets[id] = struct{}{}
		}
	}
	for _, id := range historyIDs {
		if _, ok := targets[strings.ToLower(strings.TrimSpace(id))]; ok {
			return true
		}
	}
	return false
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
