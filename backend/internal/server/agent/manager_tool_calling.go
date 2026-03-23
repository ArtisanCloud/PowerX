package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/config"
	"github.com/ArtisanCloud/PowerX/internal/server/ai/factory/llm"
	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
)

type ToolCallCandidate struct {
	Name           string
	NodeKind       string
	NodeRef        string
	FlowID         string // 兼容旧字段：workflow 场景与 NodeRef 一致
	AgentID        string
	SourceScope    string   // system|agent
	Source         string   // builtin|plugin|third_party|...
	TenantUUID     string   // tenant scoped candidate
	Visibility     string   // tenant|public|global|system
	BindingStatus  string   // active|disabled|deprecated
	RequiredGrants []string // hard-filter tool grants
	Description    string
	RequiredArgs   []string
	OptionalArgs   []string
	IntentHints    []string
	Tags           []string
	SemanticText   string
}

type CandidateBuildContext struct {
	TenantUUID    string
	AgentID       string
	ToolGrantIDs  []string
	AllowedSource []string
}

type toolCallingDecision struct {
	ToolCalls []toolCallingDecisionItem `json:"tool_calls"`
}

type toolCallingDecisionItem struct {
	Name       string                 `json:"name"`
	Args       map[string]interface{} `json:"args"`
	Confidence float64                `json:"confidence"`
	Reason     string                 `json:"reason"`
}

var toolDecisionJSONRe = regexp.MustCompile(`\{[\s\S]*\}`)

func CandidateBuildContextFromRequest(ctx context.Context) CandidateBuildContext {
	out := CandidateBuildContext{
		TenantUUID: strings.TrimSpace(reqctx.GetTenantUUID(ctx)),
		AgentID:    readContextString(ctx, "agent_id"),
	}
	if out.AgentID == "" {
		out.AgentID = readContextString(ctx, "agentId")
	}
	out.ToolGrantIDs = readContextStringSlice(ctx, "tool_grant_ids")
	if len(out.ToolGrantIDs) == 0 {
		out.ToolGrantIDs = readContextStringSlice(ctx, "toolGrantIDs")
	}
	out.AllowedSource = readContextStringSlice(ctx, "skill_source_allowlist")
	if len(out.AllowedSource) == 0 {
		out.AllowedSource = readContextStringSlice(ctx, "skills_source_allowlist")
	}
	return out
}

func (m *Manager) BuildToolCallCandidates(limit int) []ToolCallCandidate {
	return m.BuildToolCallCandidatesWithContext(CandidateBuildContext{}, limit)
}

func (m *Manager) BuildToolCallCandidatesWithContext(cctx CandidateBuildContext, limit int) []ToolCallCandidate {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]ToolCallCandidate, 0, len(m.routesByFlow)+len(m.unifiedCandidates))
	seen := make(map[string]ToolCallCandidate, len(m.routesByFlow)+len(m.unifiedCandidates))

	push := func(item ToolCallCandidate) {
		item = normalizeCandidate(item)
		if !isCandidateAllowed(item, cctx) {
			return
		}
		key := strings.ToLower(strings.TrimSpace(item.Name))
		if key == "" {
			key = strings.ToLower(strings.TrimSpace(item.NodeRef))
		}
		if key == "" {
			return
		}
		prev, ok := seen[key]
		if !ok || candidatePriority(item, cctx) > candidatePriority(prev, cctx) {
			seen[key] = item
		}
	}

	// 1) workflow 候选（从 routesByFlow 衍生）
	for flowID, rec := range m.routesByFlow {
		item := ToolCallCandidate{
			Name:          strings.TrimSpace(flowID),
			NodeKind:      "workflow",
			NodeRef:       strings.TrimSpace(flowID),
			FlowID:        strings.TrimSpace(flowID),
			AgentID:       strings.TrimSpace(rec.AgentID),
			SourceScope:   "system",
			Source:        "builtin",
			Visibility:    "system",
			BindingStatus: "active",
			Description:   "",
		}
		if rec.Spec != nil {
			item.Description = strings.TrimSpace(rec.Spec.Name)
			if item.Description == "" && rec.Spec.Metadata != nil {
				item.Description = strings.TrimSpace(rec.Spec.Metadata.Description)
			}
			if rec.Spec.Metadata != nil && rec.Spec.Metadata.IO != nil {
				for _, in := range rec.Spec.Metadata.IO.Inputs {
					name := strings.TrimSpace(in.Name)
					if name == "" {
						continue
					}
					if in.Required {
						item.RequiredArgs = append(item.RequiredArgs, name)
					} else {
						item.OptionalArgs = append(item.OptionalArgs, name)
					}
				}
			}
		}
		if item.FlowID == "" {
			continue
		}
		if item.Name == "" {
			item.Name = item.FlowID
		}
		if item.NodeRef == "" {
			item.NodeRef = item.FlowID
		}
		push(item)
	}

	// 2) skill/tooling 候选（统一候选池）
	for _, rec := range m.unifiedCandidates {
		push(rec)
	}
	for _, c := range seen {
		out = append(out, c)
	}
	out = dedupeCandidateAliases(out, cctx)

	sort.Slice(out, func(i, j int) bool {
		if out[i].NodeKind == out[j].NodeKind {
			if out[i].SourceScope == out[j].SourceScope {
				return out[i].Name < out[j].Name
			}
			return out[i].SourceScope > out[j].SourceScope
		}
		if out[i].NodeKind == out[j].NodeKind {
			return out[i].Name < out[j].Name
		}
		return out[i].NodeKind < out[j].NodeKind
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func (m *Manager) UpsertUnifiedCandidate(c ToolCallCandidate) {
	name := strings.ToLower(strings.TrimSpace(c.Name))
	if name == "" {
		return
	}
	c = normalizeCandidate(c)
	if c.NodeKind == "" || c.NodeRef == "" {
		return
	}
	if c.NodeKind == "workflow" && c.FlowID == "" {
		c.FlowID = c.NodeRef
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.unifiedCandidates == nil {
		m.unifiedCandidates = make(map[string]ToolCallCandidate)
	}
	m.unifiedCandidates[name] = c
}

// DetectTasksWithToolCalling 在 planner 决策层优先尝试 LLM tool-calling；若失败则回退到既有 DetectTasks。
func (m *Manager) DetectTasksWithToolCalling(ctx context.Context, text string, reqCfg *dto.ChatConfig) ([]flowschema.DetectedTask, error) {
	dlogRun := m.newToolCallingDebugRun(ctx, text, reqCfg)
	defer func() {
		dlogRun.flush()
		m.cachePlannerUsage(dlogRun)
	}()

	fallback := func(reason string) ([]flowschema.DetectedTask, error) {
		if strings.TrimSpace(reason) != "" {
			fmt.Printf("[intent-tool-calling] fallback: %s\n", strings.TrimSpace(reason))
			dlogRun.FallbackReason = strings.TrimSpace(reason)
		}
		// 先走统一候选池召回，避免直接退化为 rule-only。
		if recalled := m.detectTasksFromUnifiedCandidates(ctx, text); len(recalled) > 0 {
			dlogRun.ResultSource = "candidate_recall"
			dlogRun.Tasks = recalled
			return recalled, nil
		}
		out, err := m.DetectTasks(ctx, text)
		dlogRun.ResultSource = "detect_tasks"
		dlogRun.Tasks = out
		if err != nil {
			dlogRun.Error = err.Error()
		}
		return out, err
	}
	if strings.TrimSpace(text) == "" {
		return fallback("empty text")
	}
	if reqCfg == nil {
		return fallback("chat config is nil")
	}
	provider := strings.TrimSpace(reqCfg.Provider)
	model := strings.TrimSpace(reqCfg.ModelName)
	if provider == "" || model == "" {
		return fallback("provider/model missing")
	}

	cands := m.BuildToolCallCandidatesWithContext(CandidateBuildContextFromRequest(ctx), 48)
	dlogRun.Candidates = cands
	if len(cands) == 0 {
		return fallback("no candidates")
	}

	cli, err := llm.NewClient(provider)
	if err != nil {
		return fallback("new llm client failed: " + err.Error())
	}

	prompt := buildToolCallingPrompt(text, cands)
	dlogRun.Prompt = prompt
	content, err := cli.Invoke(ctx, &config.ModelConfig{
		Provider:     provider,
		Endpoint:     strings.TrimSpace(reqCfg.Endpoint),
		APIKey:       strings.TrimSpace(reqCfg.APIKey),
		Model:        model,
		SystemPrompt: strings.TrimSpace(reqCfg.SystemPrompt),
		Temperature:  0,
		MaxTokens:    minInt(maxInt(reqCfg.MaxTokens, 512), 1600),
	}, prompt)
	dlogRun.UsedLLM = true
	dlogRun.PromptTokens = estimatePlannerTokens(prompt)
	dlogRun.CompletionTokens = estimatePlannerTokens(content)
	dlogRun.LatencyMS = int(time.Since(dlogRun.At).Milliseconds())
	if err != nil {
		dlogRun.Error = err.Error()
		return fallback("llm invoke failed: " + err.Error())
	}
	dlogRun.LLMRawOutput = content

	decision, err := parseToolCallingDecision(content)
	if err != nil {
		dlogRun.Error = err.Error()
		return fallback("parse tool decision failed: " + err.Error())
	}
	dlogRun.Decision = decision

	byFlow := make(map[string]flowschema.DetectedTask)
	candByName := make(map[string]ToolCallCandidate, len(cands))
	for _, c := range cands {
		candByName[strings.ToLower(c.Name)] = c
	}

	idSeq := 0
	for _, item := range decision.ToolCalls {
		name := strings.ToLower(strings.TrimSpace(item.Name))
		cand, ok := candByName[name]
		if !ok {
			continue
		}
		args, ok := validateToolCallingArgs(item.Args, cand)
		if !ok {
			continue
		}
		score := item.Confidence
		if score <= 0 {
			score = 0.82
		}
		if score > 1 {
			score = 1
		}
		flowID := strings.TrimSpace(cand.NodeRef)
		if flowID == "" {
			flowID = strings.TrimSpace(cand.FlowID)
		}
		if flowID == "" {
			continue
		}
		if prev, exists := byFlow[flowID]; exists {
			if score > prev.Score {
				prev.Score = score
				prev.Reason = strings.TrimSpace(item.Reason)
				prev.Params = args
				byFlow[flowID] = prev
			}
			continue
		}
		idSeq++
		if args == nil {
			args = map[string]interface{}{}
		}
		args["_node_kind"] = cand.NodeKind
		args["_node_ref"] = cand.NodeRef
		args["_source_scope"] = cand.SourceScope
		args["_candidate_name"] = cand.Name
		args["_candidate_desc"] = strings.TrimSpace(cand.Description)
		byFlow[flowID] = flowschema.DetectedTask{
			TaskID:   fmt.Sprintf("tc%d", idSeq),
			FlowID:   flowID,
			AgentID:  cand.AgentID,
			Score:    score,
			Strategy: "tool_calling:" + strings.TrimSpace(cand.NodeKind),
			Reason:   strings.TrimSpace(item.Reason),
			Params:   args,
		}
	}

	if len(byFlow) == 0 {
		return fallback("tool decision produced zero valid tasks")
	}
	out := make([]flowschema.DetectedTask, 0, len(byFlow))
	for _, t := range byFlow {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	out = dedupeAliasTasks(out)
	dlogRun.ResultSource = "tool_calling"
	dlogRun.Tasks = out
	return out, nil
}

func dedupeAliasTasks(in []flowschema.DetectedTask) []flowschema.DetectedTask {
	if len(in) <= 1 {
		return in
	}
	out := make([]flowschema.DetectedTask, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, t := range in {
		key := canonicalTaskKey(t)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, t)
	}
	return out
}

func canonicalTaskKey(t flowschema.DetectedTask) string {
	ref := strings.ToLower(strings.TrimSpace(t.FlowID))
	if t.Params != nil {
		if raw, ok := t.Params["_node_ref"]; ok {
			if s := strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", raw))); s != "" {
				ref = s
			}
		}
	}
	if ref == "" {
		return ""
	}
	// skill.thirdparty.incident-triage 与 incident-triage 归一到同一 key
	if idx := strings.LastIndex(ref, "."); idx >= 0 && idx+1 < len(ref) {
		return ref[idx+1:]
	}
	if idx := strings.LastIndex(ref, "/"); idx >= 0 && idx+1 < len(ref) {
		return ref[idx+1:]
	}
	return ref
}

type toolCallingDebugRun struct {
	Enabled bool
	Dir     string
	MaxBody int
	Ctx     context.Context

	TraceID   string
	RequestID string
	Tenant    string
	Env       string
	At        time.Time

	InputText string
	Provider  string
	Model     string
	Endpoint  string

	Candidates   []ToolCallCandidate
	Prompt       string
	LLMRawOutput string
	Decision     *toolCallingDecision
	Tasks        []flowschema.DetectedTask

	ResultSource   string
	FallbackReason string
	Error          string

	UsedLLM          bool
	PromptTokens     int
	CompletionTokens int
	LatencyMS        int
}

func (m *Manager) newToolCallingDebugRun(ctx context.Context, text string, reqCfg *dto.ChatConfig) *toolCallingDebugRun {
	cfg := m.debugTraceConfig()
	run := &toolCallingDebugRun{
		Enabled:   cfg.Enabled,
		Dir:       cfg.Dir,
		MaxBody:   cfg.MaxBodyBytes,
		Ctx:       ctx,
		TraceID:   strings.TrimSpace(reqctx.GetTraceID(ctx)),
		RequestID: strings.TrimSpace(readContextString(ctx, "request_id")),
		Tenant:    strings.TrimSpace(reqctx.GetTenantUUID(ctx)),
		Env:       strings.TrimSpace(reqctx.GetEnv(ctx)),
		At:        time.Now(),
		InputText: text,
	}
	if reqCfg != nil {
		run.Provider = strings.TrimSpace(reqCfg.Provider)
		run.Model = strings.TrimSpace(reqCfg.ModelName)
		run.Endpoint = strings.TrimSpace(reqCfg.Endpoint)
	}
	return run
}

func estimatePlannerTokens(s string) int {
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

func (m *Manager) cachePlannerUsage(run *toolCallingDebugRun) {
	if run == nil || !run.UsedLLM {
		return
	}
	traceID := strings.TrimSpace(run.TraceID)
	if traceID == "" {
		return
	}
	prompt := run.PromptTokens
	completion := run.CompletionTokens
	total := prompt + completion
	usage := map[string]any{
		"total_prompt_tokens":     prompt,
		"total_completion_tokens": completion,
		"total_tokens":            total,
		"hops": []map[string]any{
			{
				"phase":             "planner",
				"provider":          run.Provider,
				"model":             run.Model,
				"prompt_tokens":     prompt,
				"completion_tokens": completion,
				"total_tokens":      total,
				"latency_ms":        run.LatencyMS,
				"estimated":         true,
			},
		},
	}
	m.setPlannerUsage(traceID, usage)
}

func (r *toolCallingDebugRun) flush() {
	if r == nil || !r.Enabled {
		return
	}
	if strings.TrimSpace(r.Dir) == "" {
		r.Dir = "logs/agent_debug"
	}
	if r.MaxBody <= 0 {
		r.MaxBody = 512 * 1024
	}
	dayDir := filepath.Join(r.Dir, r.At.Format("20060102"))
	if err := os.MkdirAll(dayDir, 0o755); err != nil {
		logger.WarnF(r.Ctx, "[agent.debug_trace] mkdir failed dir=%s err=%v", dayDir, err)
		return
	}
	base := strings.TrimSpace(r.TraceID)
	if base == "" {
		base = strings.TrimSpace(r.RequestID)
	}
	if base == "" {
		base = fmt.Sprintf("agent_debug_%d", r.At.UnixNano())
	}
	filePath := filepath.Join(dayDir, fmt.Sprintf("trace-%s_planner_%s.json", shortDebugBase(base), r.At.Format("15-04-05.000")))
	payload := map[string]any{
		"meta": map[string]any{
			"trace_id":    r.TraceID,
			"request_id":  r.RequestID,
			"tenant_uuid": r.Tenant,
			"env":         r.Env,
			"created_at":  r.At.Format(time.RFC3339Nano),
		},
		"input": map[string]any{
			"text":     r.InputText,
			"provider": r.Provider,
			"model":    r.Model,
			"endpoint": r.Endpoint,
		},
		"candidates_count": len(r.Candidates),
		"candidates":       r.Candidates,
		"prompt":           truncateDebugText(r.Prompt, r.MaxBody),
		"llm_raw_output":   truncateDebugText(r.LLMRawOutput, r.MaxBody),
		"decision":         r.Decision,
		"result_source":    r.ResultSource,
		"fallback_reason":  r.FallbackReason,
		"error":            r.Error,
		"tasks":            r.Tasks,
	}
	bs, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		logger.WarnF(r.Ctx, "[agent.debug_trace] marshal failed trace=%s err=%v", r.TraceID, err)
		return
	}
	if len(bs) > r.MaxBody*3 {
		bs = []byte(truncateDebugText(string(bs), r.MaxBody*3))
	}
	if err := os.WriteFile(filePath, bs, 0o644); err != nil {
		logger.WarnF(r.Ctx, "[agent.debug_trace] write failed path=%s err=%v", filePath, err)
		return
	}
	logger.InfoF(r.Ctx, "[agent.debug_trace] saved path=%s trace_id=%s", filePath, r.TraceID)
}

func truncateDebugText(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max] + "\n...[truncated]..."
}

func sanitizeDebugFilename(s string) string {
	if s == "" {
		return "unknown"
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
		case r == '-' || r == '_' || r == '.':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "unknown"
	}
	return out
}

func shortDebugBase(s string) string {
	s = sanitizeDebugFilename(s)
	if len(s) > 12 {
		return s[:12]
	}
	return s
}

func dedupeCandidateAliases(in []ToolCallCandidate, cctx CandidateBuildContext) []ToolCallCandidate {
	if len(in) <= 1 {
		return in
	}
	best := make(map[string]ToolCallCandidate, len(in))
	for _, c := range in {
		key := candidateAliasKey(c)
		if key == "" {
			key = strings.ToLower(strings.TrimSpace(c.Name))
		}
		prev, ok := best[key]
		if !ok || candidatePriority(c, cctx) > candidatePriority(prev, cctx) || betterCanonicalName(c, prev) {
			best[key] = c
		}
	}
	out := make([]ToolCallCandidate, 0, len(best))
	for _, c := range best {
		out = append(out, c)
	}
	return out
}

func candidateAliasKey(c ToolCallCandidate) string {
	ref := strings.ToLower(strings.TrimSpace(c.NodeRef))
	if ref == "" {
		ref = strings.ToLower(strings.TrimSpace(c.Name))
	}
	if ref == "" {
		return ""
	}
	if idx := strings.LastIndex(ref, "."); idx >= 0 && idx+1 < len(ref) {
		return ref[idx+1:]
	}
	if idx := strings.LastIndex(ref, "/"); idx >= 0 && idx+1 < len(ref) {
		return ref[idx+1:]
	}
	return ref
}

func betterCanonicalName(cur, prev ToolCallCandidate) bool {
	cn := strings.ToLower(strings.TrimSpace(cur.Name))
	pn := strings.ToLower(strings.TrimSpace(prev.Name))
	// 优先短名（incident-triage）而不是命名空间别名（skill.thirdparty.incident-triage）
	return cn != "" && pn != "" && len(cn) < len(pn)
}

func parseToolCallingDecision(raw string) (*toolCallingDecision, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("empty llm output")
	}

	for _, candidate := range toolDecisionJSONCandidates(raw) {
		var out toolCallingDecision
		if err := json.Unmarshal([]byte(candidate), &out); err == nil {
			return &out, nil
		}
	}

	// 最后尝试对截断 JSON 做最小修复（补齐缺失的右花括号）。
	if repaired, ok := repairPossiblyTruncatedJSON(raw); ok {
		var out toolCallingDecision
		if err := json.Unmarshal([]byte(repaired), &out); err == nil {
			return &out, nil
		}
	}
	return nil, fmt.Errorf("invalid tool decision json")
}

func toolDecisionJSONCandidates(raw string) []string {
	out := make([]string, 0, 8)
	seen := map[string]struct{}{}
	push := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		if _, ok := seen[s]; ok {
			return
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}

	clean := strings.TrimSpace(raw)
	push(clean)

	// 去掉 markdown code fence 包裹。
	if strings.HasPrefix(clean, "```") {
		lines := strings.Split(clean, "\n")
		if len(lines) >= 3 {
			core := strings.Join(lines[1:len(lines)-1], "\n")
			push(core)
		}
	}

	// 兼容历史正则提取。
	if candidate := toolDecisionJSONRe.FindString(clean); candidate != "" {
		push(candidate)
	}

	// 提取所有平衡 JSON 对象。
	for _, obj := range extractBalancedJSONObjects(clean) {
		// 优先包含 tool_calls 的对象
		if strings.Contains(strings.ToLower(obj), "tool_calls") {
			push(obj)
		}
	}
	for _, obj := range extractBalancedJSONObjects(clean) {
		push(obj)
	}

	return out
}

func extractBalancedJSONObjects(s string) []string {
	out := make([]string, 0, 4)
	var (
		start      = -1
		depth      = 0
		inString   = false
		escapeNext = false
	)
	for i, r := range s {
		if inString {
			if escapeNext {
				escapeNext = false
				continue
			}
			if r == '\\' {
				escapeNext = true
				continue
			}
			if r == '"' {
				inString = false
			}
			continue
		}
		if r == '"' {
			inString = true
			continue
		}
		if r == '{' {
			if depth == 0 {
				start = i
			}
			depth++
			continue
		}
		if r == '}' {
			if depth > 0 {
				depth--
				if depth == 0 && start >= 0 {
					end := i + utf8.RuneLen(r)
					out = append(out, s[start:end])
					start = -1
				}
			}
		}
	}
	return out
}

func repairPossiblyTruncatedJSON(raw string) (string, bool) {
	first := strings.Index(raw, "{")
	if first < 0 {
		return "", false
	}
	candidate := strings.TrimSpace(raw[first:])
	if candidate == "" {
		return "", false
	}
	// 已经平衡就不修。
	openN := strings.Count(candidate, "{")
	closeN := strings.Count(candidate, "}")
	if openN <= closeN {
		return candidate, true
	}
	missing := openN - closeN
	if missing > 8 {
		return "", false
	}
	return candidate + strings.Repeat("}", missing), true
}

func validateToolCallingArgs(raw map[string]interface{}, cand ToolCallCandidate) (map[string]interface{}, bool) {
	if raw == nil {
		raw = map[string]interface{}{}
	}
	allowed := make(map[string]struct{}, len(cand.RequiredArgs)+len(cand.OptionalArgs))
	for _, k := range cand.RequiredArgs {
		allowed[strings.ToLower(strings.TrimSpace(k))] = struct{}{}
	}
	for _, k := range cand.OptionalArgs {
		allowed[strings.ToLower(strings.TrimSpace(k))] = struct{}{}
	}

	out := make(map[string]interface{}, len(raw))
	// 未声明参数约束时，允许透传 args（用于 skill/tooling 通用节点）
	if len(allowed) == 0 {
		for k, v := range raw {
			key := strings.ToLower(strings.TrimSpace(k))
			if key == "" {
				continue
			}
			out[key] = v
		}
		return out, true
	}
	for k, v := range raw {
		key := strings.ToLower(strings.TrimSpace(k))
		if key == "" {
			continue
		}
		if _, ok := allowed[key]; !ok {
			return nil, false
		}
		out[key] = v
	}

	for _, req := range cand.RequiredArgs {
		k := strings.ToLower(strings.TrimSpace(req))
		v, ok := out[k]
		if !ok || isEmptyToolArg(v) {
			return nil, false
		}
	}
	return out, true
}

func isEmptyToolArg(v interface{}) bool {
	switch x := v.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(x) == ""
	}
	return false
}

func buildToolCallingPrompt(text string, cands []ToolCallCandidate) string {
	var b strings.Builder
	b.WriteString("你是 Agent Planner 的工具调度器。请根据用户输入，从提供的工具清单中选择 0~4 个要调用的工具。\\n")
	b.WriteString("必须只输出 JSON，不要输出解释文本。\\n")
	b.WriteString(`输出格式: {"tool_calls":[{"name":"<candidate_name>","args":{...},"confidence":0.0-1.0,"reason":"..."}]}` + "\n")
	b.WriteString("约束: 只能使用下方清单中的 name；args 只允许写该工具声明的参数。\\n\\n")
	b.WriteString("能力清单（按类型分区 + source_scope）:\\n")
	appendCandidateSection(&b, "workflow_catalog", "workflow", cands)
	appendCandidateSection(&b, "skill_catalog", "skill", cands)
	appendCandidateSection(&b, "tooling_catalog", "tooling", cands)
	appendCandidateSection(&b, "llm_catalog", "llm", cands)
	b.WriteString("\\n字段说明: source_scope=system|agent，ref 为节点引用标识。\\n")
	b.WriteString("若无合适候选，输出 tool_calls=[]。\\n")
	b.WriteString("只在必要时选择多个调用，避免冗余。\\n")
	b.WriteString("\\n")
	for i, c := range cands {
		if strings.TrimSpace(c.Name) == "" {
			continue
		}
		b.WriteString(fmt.Sprintf("schema.%d %s => required=%v optional=%v\\n", i+1, c.Name, c.RequiredArgs, c.OptionalArgs))
	}
	b.WriteString("\\n用户输入:\\n")
	b.WriteString(text)
	return b.String()
}

func appendCandidateSection(b *strings.Builder, title, kind string, cands []ToolCallCandidate) {
	items := make([]ToolCallCandidate, 0, len(cands))
	for _, c := range cands {
		if strings.EqualFold(strings.TrimSpace(c.NodeKind), kind) {
			items = append(items, c)
		}
	}
	if len(items) == 0 {
		return
	}
	b.WriteString(title + ":[\n")
	for _, c := range items {
		b.WriteString(fmt.Sprintf("  {name:%q, source_scope:%q, ref:%q, desc:%q}\n",
			c.Name, c.SourceScope, c.NodeRef, strings.TrimSpace(c.Description)))
	}
	b.WriteString("]\n")
}

func normalizeCandidate(c ToolCallCandidate) ToolCallCandidate {
	c.Name = strings.TrimSpace(c.Name)
	c.NodeKind = strings.ToLower(strings.TrimSpace(c.NodeKind))
	c.NodeRef = strings.TrimSpace(c.NodeRef)
	c.FlowID = strings.TrimSpace(c.FlowID)
	c.AgentID = strings.TrimSpace(c.AgentID)
	c.SourceScope = strings.ToLower(strings.TrimSpace(c.SourceScope))
	c.Source = strings.ToLower(strings.TrimSpace(c.Source))
	c.TenantUUID = strings.TrimSpace(c.TenantUUID)
	c.Visibility = strings.ToLower(strings.TrimSpace(c.Visibility))
	c.BindingStatus = strings.ToLower(strings.TrimSpace(c.BindingStatus))
	if c.SourceScope == "" {
		c.SourceScope = "system"
	}
	if c.Visibility == "" {
		c.Visibility = "public"
	}
	if c.BindingStatus == "" {
		c.BindingStatus = "active"
	}
	if c.NodeKind == "workflow" && c.FlowID == "" {
		c.FlowID = c.NodeRef
	}
	return c
}

func candidatePriority(c ToolCallCandidate, cctx CandidateBuildContext) int {
	p := 0
	switch strings.ToLower(strings.TrimSpace(c.SourceScope)) {
	case "agent":
		p += 100
	case "system":
		p += 10
	}
	if cctx.AgentID != "" && strings.EqualFold(strings.TrimSpace(c.AgentID), strings.TrimSpace(cctx.AgentID)) {
		p += 20
	}
	switch strings.ToLower(strings.TrimSpace(c.BindingStatus)) {
	case "active", "published":
		p += 5
	}
	return p
}

func isCandidateAllowed(c ToolCallCandidate, cctx CandidateBuildContext) bool {
	status := strings.ToLower(strings.TrimSpace(c.BindingStatus))
	if status == "disabled" || status == "deprecated" {
		return false
	}
	if c.SourceScope == "agent" && cctx.AgentID != "" && c.AgentID != "" && !strings.EqualFold(c.AgentID, cctx.AgentID) {
		return false
	}
	tenantUUID := strings.TrimSpace(cctx.TenantUUID)
	if tenantUUID != "" && c.TenantUUID != "" && !strings.EqualFold(strings.TrimSpace(c.TenantUUID), tenantUUID) {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(c.Visibility), "tenant") && tenantUUID == "" {
		return false
	}
	if len(cctx.AllowedSource) > 0 && c.Source != "" {
		found := false
		for _, s := range cctx.AllowedSource {
			if strings.EqualFold(strings.TrimSpace(s), c.Source) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if len(c.RequiredGrants) > 0 {
		if len(cctx.ToolGrantIDs) == 0 {
			return false
		}
		match := false
		for _, req := range c.RequiredGrants {
			for _, got := range cctx.ToolGrantIDs {
				if strings.EqualFold(strings.TrimSpace(req), strings.TrimSpace(got)) {
					match = true
					break
				}
			}
			if match {
				break
			}
		}
		if !match {
			return false
		}
	}
	return true
}

func readContextString(ctx context.Context, key string) string {
	if ctx == nil {
		return ""
	}
	v := ctx.Value(key)
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	case fmt.Stringer:
		return strings.TrimSpace(x.String())
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", x))
	}
}

func readContextStringSlice(ctx context.Context, key string) []string {
	if ctx == nil {
		return nil
	}
	v := ctx.Value(key)
	switch x := v.(type) {
	case []string:
		return x
	case []any:
		out := make([]string, 0, len(x))
		for _, item := range x {
			s := strings.TrimSpace(fmt.Sprintf("%v", item))
			if s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
