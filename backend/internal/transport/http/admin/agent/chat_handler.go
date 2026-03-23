// file: internal/app/http/admin/agent/chat_handler.go
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	appcfg "github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/ArtisanCloud/PowerX/internal/server/agent"
	agentcfg "github.com/ArtisanCloud/PowerX/internal/server/agent/config"
	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/runtime"
	agentschema "github.com/ArtisanCloud/PowerX/internal/server/agent/schemas"
	agentSvc "github.com/ArtisanCloud/PowerX/internal/service/agent"
	capservice "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	skillservice "github.com/ArtisanCloud/PowerX/internal/service/skills"
	skillrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/skills"
	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	dto "github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/utils"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type AgentChatHandler struct {
	his         *agentSvc.ChatHistoryService
	cfgResolver *agentSvc.ChatConfigResolver
	ag          *agentSvc.AgentService
	audit       *capservice.AuditService
	settings    *agentSvc.AgentSettingService
	skillAudit  *skillservice.AuditTraceService
	ctxOptSvc   *agentSvc.ContextOptimizerConfigService
}

type agentInvokeRequest struct {
	AgentID   string                 `json:"agent_id"`
	SessionID string                 `json:"session_id,omitempty"`
	Message   string                 `json:"message"`
	Meta      map[string]interface{} `json:"meta,omitempty"`
}

type agentInvokeSink struct {
	buf   strings.Builder
	final string
}

func (s *agentInvokeSink) Emit(event string, payload any) error {
	switch event {
	case dto.EventToken:
		if m, ok := payload.(map[string]any); ok {
			if d, ok := m["delta"].(string); ok && d != "" {
				s.buf.WriteString(d)
			}
		}
	case dto.EventFinal:
		if text := extractInvokeAssistantText(payload); strings.TrimSpace(text) != "" {
			s.final = text
		}
	}
	return nil
}

func (s *agentInvokeSink) Reply() string {
	if strings.TrimSpace(s.final) != "" {
		return s.final
	}
	return runtime.SanitizeAssistantVisibleText(s.buf.String())
}

type plannerTraceSink struct {
	next       runtime.EventSink
	audit      *skillservice.AuditTraceService
	tenantUUID string
	traceID    string
	planID     string
	mu         sync.Mutex
}

type streamDebugTraceSink struct {
	next    runtime.EventSink
	enabled bool
	dir     string
	maxBody int
	ctx     context.Context
	traceID string
	at      time.Time

	meta    map[string]any
	request map[string]any

	mu        sync.Mutex
	events    []map[string]any
	final     map[string]any
	lastError map[string]any
	end       map[string]any
	usage     map[string]any
}

func newStreamDebugTraceSink(next runtime.EventSink, cfg agent.DebugTraceConfig, ctx context.Context, traceID string, req map[string]any, meta map[string]any) *streamDebugTraceSink {
	maxBody := cfg.MaxBodyBytes
	if maxBody <= 0 {
		maxBody = 512 * 1024
	}
	dir := strings.TrimSpace(cfg.Dir)
	if dir == "" {
		dir = "logs/agent_debug"
	}
	return &streamDebugTraceSink{
		next:    next,
		enabled: cfg.Enabled,
		dir:     dir,
		maxBody: maxBody,
		ctx:     ctx,
		traceID: strings.TrimSpace(traceID),
		at:      time.Now(),
		request: req,
		meta:    meta,
		events:  make([]map[string]any, 0, 32),
		usage:   map[string]any{},
	}
}

func (s *streamDebugTraceSink) Emit(event string, payload any) error {
	if s.next != nil {
		if err := s.next.Emit(event, payload); err != nil {
			return err
		}
	}
	if !s.enabled {
		return nil
	}
	s.capture(event, payload)
	return nil
}

func (s *streamDebugTraceSink) capture(event string, payload any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch event {
	case dto.EventIntent, dto.EventPlan, dto.EventNodeStart, dto.EventNodeEnd, dto.EventFinal, dto.EventError, dto.EventEnd:
		s.events = append(s.events, map[string]any{
			"event": event,
			"at":    time.Now().Format(time.RFC3339Nano),
			"data":  truncateDebugAny(payload, s.maxBody/8),
		})
	}
	switch event {
	case dto.EventMeta:
		if m, ok := payload.(map[string]any); ok {
			if v, ok := m["llm_provider"]; ok {
				s.usage["_llm_provider"] = readStringAny(v)
			}
			if v, ok := m["llm_model"]; ok {
				s.usage["_llm_model"] = readStringAny(v)
			}
			if co, ok := m["context_optimizer"].(map[string]any); ok {
				if v, ok := co["prompt_tokens"]; ok {
					s.usage["prompt_tokens"] = toIntAny(v)
				}
				if v, ok := co["cache_mode"]; ok {
					s.usage["cache_mode"] = readStringAny(v)
				}
				if v, ok := co["context_layers_size"]; ok {
					s.usage["context_layers_size"] = v
				}
				if v, ok := co["trim_actions"]; ok {
					s.usage["trim_actions"] = v
				}
			}
		}
	case dto.EventFinal:
		if m, ok := payload.(map[string]any); ok {
			s.final = m
			text := extractInvokeAssistantText(m)
			if strings.TrimSpace(text) != "" {
				s.usage["completion_tokens"] = estimateDebugTokens(text)
			}
			if md, ok := m["metadata"].(map[string]any); ok {
				if v, ok := md["cached_tokens"]; ok {
					s.usage["cached_tokens"] = toIntAny(v)
				}
				if v, ok := md["cache_hit"]; ok {
					if b, ok := v.(bool); ok {
						s.usage["cache_hit"] = b
					}
				}
			}
		}
	case dto.EventError:
		if m, ok := payload.(map[string]any); ok {
			s.lastError = m
		}
	case dto.EventEnd:
		if m, ok := payload.(map[string]any); ok {
			s.end = m
		}
	}
}

func (s *streamDebugTraceSink) Flush() {
	if s == nil || !s.enabled {
		return
	}
	dayDir := filepath.Join(s.dir, s.at.Format("20060102"))
	if err := os.MkdirAll(dayDir, 0o755); err != nil {
		logger.WarnF(s.ctx, "[agent.debug_trace] mkdir failed dir=%s err=%v", dayDir, err)
		return
	}
	base := strings.TrimSpace(s.traceID)
	if base == "" {
		base = fmt.Sprintf("agent_stream_%d", s.at.UnixNano())
	}
	path := filepath.Join(dayDir, fmt.Sprintf("trace-%s_stream_%s.json", shortDebugBase(base), s.at.Format("15-04-05.000")))

	s.mu.Lock()
	usageSnapshot := s.usageSnapshotLocked()
	payload := map[string]any{
		"meta": map[string]any{
			"trace_id":   s.traceID,
			"created_at": s.at.Format(time.RFC3339Nano),
			"type":       "agent_stream",
			"runtime":    s.meta,
		},
		"request": map[string]any{
			"body": s.request,
		},
		"response": map[string]any{
			"final":        s.final,
			"last_error":   s.lastError,
			"end":          s.end,
			"usage":        usageSnapshot,
			"events_count": len(s.events),
			"events":       s.events,
		},
	}
	s.mu.Unlock()

	bs, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		logger.WarnF(s.ctx, "[agent.debug_trace] marshal stream trace failed trace=%s err=%v", s.traceID, err)
		return
	}
	if len(bs) > s.maxBody*3 {
		bs = []byte(truncateDebugText(string(bs), s.maxBody*3))
	}
	if err := os.WriteFile(path, bs, 0o644); err != nil {
		logger.WarnF(s.ctx, "[agent.debug_trace] write stream trace failed path=%s err=%v", path, err)
		return
	}
	logger.InfoF(s.ctx, "[agent.debug_trace] saved path=%s trace_id=%s", path, s.traceID)
}

func (s *streamDebugTraceSink) UsageSnapshot() map[string]any {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.usageSnapshotLocked()
}

func (s *streamDebugTraceSink) usageSnapshotLocked() map[string]any {
	if s == nil {
		return nil
	}
	prompt := toIntAny(s.usage["prompt_tokens"])
	completion := toIntAny(s.usage["completion_tokens"])
	provider := readStringAny(s.usage["_llm_provider"])
	model := readStringAny(s.usage["_llm_model"])
	hops := make([]map[string]any, 0, 4)
	if raw, ok := s.usage["hops"].([]map[string]any); ok {
		hops = append(hops, raw...)
	} else if rawAny, ok := s.usage["hops"].([]any); ok {
		for _, item := range rawAny {
			if m, ok := item.(map[string]any); ok {
				hops = append(hops, m)
			}
		}
	}
	if prompt > 0 || completion > 0 {
		hops = append(hops, map[string]any{
			"phase":             "chat",
			"provider":          provider,
			"model":             model,
			"prompt_tokens":     prompt,
			"completion_tokens": completion,
			"total_tokens":      prompt + completion,
			"estimated":         true,
		})
	}
	totalPrompt := 0
	totalCompletion := 0
	for _, hop := range hops {
		totalPrompt += toIntAny(hop["prompt_tokens"])
		totalCompletion += toIntAny(hop["completion_tokens"])
	}
	if totalPrompt == 0 && prompt > 0 {
		totalPrompt = prompt
	}
	if totalCompletion == 0 && completion > 0 {
		totalCompletion = completion
	}
	out := map[string]any{
		"total_prompt_tokens":     totalPrompt,
		"total_completion_tokens": totalCompletion,
		"total_tokens":            totalPrompt + totalCompletion,
		"hops":                    hops,
	}
	if v, ok := s.usage["cache_mode"]; ok {
		out["cache_mode"] = v
	}
	if v, ok := s.usage["cached_tokens"]; ok {
		out["cached_tokens"] = v
	}
	if v, ok := s.usage["context_layers_size"]; ok {
		out["context_layers_size"] = v
	}
	if v, ok := s.usage["trim_actions"]; ok {
		out["trim_actions"] = v
	}
	return out
}

func (s *streamDebugTraceSink) AddUsageHop(hop map[string]any) {
	if s == nil || len(hop) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, ok := s.usage["hops"].([]map[string]any)
	if !ok {
		raw = []map[string]any{}
	}
	raw = append(raw, hop)
	s.usage["hops"] = raw
}

func toIntAny(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	case float32:
		return int(x)
	case json.Number:
		n, _ := x.Int64()
		return int(n)
	default:
		return 0
	}
}

func estimateDebugTokens(s string) int {
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

func truncateDebugAny(v any, max int) any {
	if max <= 0 {
		return v
	}
	b, err := json.Marshal(v)
	if err != nil {
		return v
	}
	if len(b) <= max {
		var out any
		if err := json.Unmarshal(b, &out); err == nil {
			return out
		}
		return string(b)
	}
	return truncateDebugText(string(b), max)
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

func newPlannerTraceSink(next runtime.EventSink, audit *skillservice.AuditTraceService, tenantUUID, traceID string) *plannerTraceSink {
	return &plannerTraceSink{
		next:       next,
		audit:      audit,
		tenantUUID: strings.TrimSpace(tenantUUID),
		traceID:    strings.TrimSpace(traceID),
	}
}

func (s *plannerTraceSink) Emit(event string, payload any) error {
	if s.next != nil {
		if err := s.next.Emit(event, payload); err != nil {
			return err
		}
	}
	if s.audit == nil {
		return nil
	}
	m, ok := payload.(map[string]any)
	if !ok {
		return nil
	}
	switch event {
	case dto.EventPlan:
		s.mu.Lock()
		planID := strings.TrimSpace(readStringAny(m["plan_id"]))
		if planID == "" {
			if p, ok := m["plan"].(map[string]any); ok {
				planID = strings.TrimSpace(readStringAny(p["plan_id"]))
			}
		}
		s.planID = planID
		s.mu.Unlock()
	case dto.EventNodeStart:
		s.recordNodeTrace(m, "running")
	case dto.EventNodeEnd:
		status := strings.TrimSpace(readStringAny(m["status"]))
		if status == "" {
			status = "completed"
		}
		s.recordNodeTrace(m, strings.ToLower(status))
	}
	return nil
}

func (s *plannerTraceSink) recordNodeTrace(m map[string]any, status string) {
	nodeID := strings.TrimSpace(readStringAny(m["node_id"]))
	if nodeID == "" {
		nodeID = strings.TrimSpace(readStringAny(m["task_id"]))
	}
	nodeRef := strings.TrimSpace(readStringAny(m["node_ref"]))
	if nodeRef == "" {
		nodeRef = strings.TrimSpace(readStringAny(m["flow_id"]))
	}
	nodeKind := strings.ToLower(strings.TrimSpace(readStringAny(m["node_kind"])))
	if nodeKind == "" {
		nodeKind = "workflow"
	}
	if nodeRef == "" || nodeID == "" {
		return
	}
	s.mu.Lock()
	planID := strings.TrimSpace(readStringAny(m["plan_id"]))
	if planID == "" {
		planID = s.planID
	}
	if s.planID == "" && planID != "" {
		s.planID = planID
	}
	s.mu.Unlock()

	_ = s.audit.RecordExecutionTrace(context.Background(), skillservice.ExecutionTraceInput{
		TraceID:      s.traceID,
		TenantUUID:   s.tenantUUID,
		SkillID:      nodeRef,
		Version:      "",
		Entrypoint:   nodeID,
		InvokePath:   "agent.invoke.plan",
		ProtocolUsed: "agent." + nodeKind,
		Status:       status,
		PlanID:       planID,
		NodeID:       nodeID,
		NodeStatus:   status,
		RetryTrace:   strings.TrimSpace(readStringAny(m["error"])),
		LatencyMS:    0,
		AuthPass:     true,
	})
}

func readStringAny(v interface{}) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	case fmt.Stringer:
		return x.String()
	default:
		return fmt.Sprintf("%v", x)
	}
}

func buildCandidateSummary(cands []agent.ToolCallCandidate, maxPerKind int) string {
	if len(cands) == 0 {
		return ""
	}
	if maxPerKind <= 0 {
		maxPerKind = 8
	}
	buckets := map[string][]string{
		"workflow": {},
		"skill":    {},
		"tooling":  {},
		"llm":      {},
	}
	for _, c := range cands {
		kind := strings.ToLower(strings.TrimSpace(c.NodeKind))
		if kind == "" {
			kind = "workflow"
		}
		if _, ok := buckets[kind]; !ok {
			buckets[kind] = []string{}
		}
		if len(buckets[kind]) >= maxPerKind {
			continue
		}
		name := strings.TrimSpace(c.Name)
		if name == "" {
			name = strings.TrimSpace(c.NodeRef)
		}
		if name == "" {
			continue
		}
		buckets[kind] = append(buckets[kind], name)
	}
	parts := make([]string, 0, 4)
	for _, kind := range []string{"workflow", "skill", "tooling", "llm"} {
		items := buckets[kind]
		if len(items) == 0 {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s(%d): %s", kind, len(items), strings.Join(items, ", ")))
	}
	return strings.Join(parts, "\n")
}

func supportsProviderPromptCache(provider string) bool {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "openai", "anthropic", "google", "gemini":
		return true
	default:
		return false
	}
}

func extractInvokeAssistantText(payload any) string {
	switch m := payload.(type) {
	case map[string]any:
		if d, ok := m["data"].(map[string]any); ok {
			if s, ok := d["content"].(string); ok {
				return runtime.SanitizeAssistantVisibleText(s)
			}
		}
		if s, ok := m["content"].(string); ok {
			return runtime.SanitizeAssistantVisibleText(s)
		}
	}
	return ""
}

func NewAgentChatHandler(dep *shared.Deps) *AgentChatHandler {
	traceRepo := skillrepo.NewSkillExecutionTraceRepository(dep.DB)
	auditRepo := skillrepo.NewSkillLifecycleAuditRepository(dep.DB)
	return &AgentChatHandler{
		his:         agentSvc.NewChatHistoryService(dep.DB),
		cfgResolver: agentSvc.NewChatConfigResolver(dep.DB),
		ag:          agentSvc.NewAgentService(dep.DB),
		audit:       dep.CapabilityRegistryAudit,
		settings:    agentSvc.NewAgentSettingService(dep.DB),
		skillAudit:  skillservice.NewAuditTraceService(traceRepo, auditRepo),
		ctxOptSvc:   agentSvc.NewContextOptimizerConfigService(dep.DB),
	}
}

func (h *AgentChatHandler) SimulateSSE(c *gin.Context) {
	// 探针
	if strings.EqualFold(c.Query("probe"), "1") || strings.EqualFold(c.Query("probe"), "true") {
		setSSEHeaders(c)
		c.SSEvent(dto.EventStart, gin.H{"message": "probe ok"})
		c.SSEvent(dto.EventEnd, gin.H{"ok": true})
		return
	}

	setSSEHeaders(c)

	text := strings.TrimSpace(utils.FirstNonEmpty(c.Query("text"), c.Query("message")))
	if text == "" {
		text = `<think>这是一个 SSE 模拟流，前端可以用它测试逐字渲染与事件解析。
这是一个 SSE 模拟流，前端可以用它测试逐字渲染与事件解析。
这是一个 SSE 模拟流，前端可以用它测试逐字渲染与事件解析。</think> 
这是think后，完成的结论1,
这是think后，完成的结论2,
这是think后，完成的结论3,
这是think后，完成的结论4,
这是think后，完成的结论5
`
	}
	chunk := utils.ParseIntDefault(c.Query("chunk"), 1)       // 每次输出多少字符
	delayMs := utils.ParseIntDefault(c.Query("delay_ms"), 60) // 每块之间延时（毫秒）
	if chunk <= 0 {
		chunk = 1
	}
	if delayMs < 0 {
		delayMs = 0
	}

	ctx := c.Request.Context()
	flowID := "mock_flow"
	execID := fmt.Sprintf("mock_%d", time.Now().UnixNano())

	// 开始帧
	c.SSEvent(dto.EventStart, gin.H{
		"flow_id":      flowID,
		"execution_id": execID,
		"message":      "开始模拟 SSE 输出",
	})
	c.Writer.Flush()

	// 心跳
	hb := time.NewTicker(25 * time.Second)
	defer hb.Stop()

	// 分块（按 rune，避免多字节拆断）
	rs := []rune(text)
	stepID := "mock"
	nowTs := func() int64 { return time.Now().Unix() }

	// 循环输出 token
LOOP:
	for i := 0; i < len(rs); i += chunk {
		select {
		case <-ctx.Done():
			// 客户端断开或被取消
			c.SSEvent("error", gin.H{"success": false, "error": ctx.Err().Error()})
			c.SSEvent("end", gin.H{"success": false, "message": "连接已中断"})
			return
		case <-hb.C:
			c.SSEvent("heartbeat", gin.H{"ts": nowTs()})
			c.Writer.Flush()
			i -= chunk // 这次只发心跳，不消耗文本
			continue
		default:
		}

		j := i + chunk
		if j > len(rs) {
			j = len(rs)
		}
		delta := string(rs[i:j])

		c.SSEvent(dto.EventToken, gin.H{
			"delta":     delta,
			"step_id":   stepID,
			"timestamp": nowTs(),
		})
		c.Writer.Flush()

		if delayMs > 0 {
			select {
			case <-ctx.Done():
				break LOOP
			case <-time.After(time.Duration(delayMs) * time.Millisecond):
			}
		}
	}

	// 最终帧
	c.SSEvent(dto.EventFinal, gin.H{
		"success":   true,
		"data":      gin.H{"content": text},
		"metadata":  gin.H{"mock": true},
		"timestamp": nowTs(),
	})
	c.SSEvent(dto.EventEnd, gin.H{"success": true, "message": "SSE 模拟完成"})
}

// GET /api/agents/stream/sse?q=...&env=dev&agent_id=...&session_id=...
// internal/app/http/admin/agent/chat_handler.go （节选）
func (h *AgentChatHandler) StreamSSE(c *gin.Context) {
	// 1) 探活
	if strings.EqualFold(c.Query("probe"), "1") || strings.EqualFold(c.Query("probe"), "true") {
		runtime.SetSSEHeaders(c)
		c.SSEvent(dto.EventStart, gin.H{"message": "probe ok"})
		c.SSEvent(dto.EventEnd, gin.H{"ok": true})
		return
	}
	// 2) 解析入参 & 会话（保持你现有逻辑）
	env, err := resolveAgentEnv(c, h.settings)
	if err != nil {
		dto.ResponseError(c, 400, err.Error(), nil)
		return
	}
	getParam := func(key string) string {
		if v, ok := c.Get(key); ok {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
		return strings.TrimSpace(c.Query(key))
	}
	q := strings.TrimSpace(utils.FirstNonEmpty(getParam("q"), getParam("message")))
	regenFromID, _ := utils.ParseUintID(strings.TrimSpace(c.Query("regen_from_message_id")))
	if q == "" && regenFromID == 0 {
		dto.ResponseError(c, 400, "缺少 q（消息内容）", nil)
		return
	}
	tenantCtx, err := requireTenantContext(c)
	if err != nil {
		dto.ResponseError(c, 400, err.Error(), nil)
		return
	}
	tenantRef := tenantCtx.UUIDPtr()
	var agentID uint64
	if agentUUIDStr := getParam("agent_uuid"); agentUUIDStr != "" {
		agentUUID, err := uuid.Parse(agentUUIDStr)
		if err != nil {
			dto.ResponseError(c, 400, "agent_uuid 非法", err)
			return
		}
		exist, err := h.ag.GetByUUID(c.Request.Context(), env, tenantRef, agentUUID)
		if err != nil {
			dto.ResponseError(c, 404, "未找到指定的 Agent", err)
			return
		}
		agentID = exist.ID
	} else {
		id, _ := utils.ParseUintID(getParam("agent_id"))
		agentID = id
	}
	// 会话：要求显式传入 session_id/session_uuid
	var sess *dbmodel.AgentChatSession
	if sidStr := getParam("session_id"); sidStr != "" {
		sess, _ = h.resolveSessionByParam(c, env, tenantRef, sidStr)
	}
	if sess == nil {
		if sidStr := getParam("session_uuid"); sidStr != "" {
			sess, _ = h.resolveSessionByParam(c, env, tenantRef, sidStr)
		}
	}
	if sess == nil {
		dto.ResponseError(c, 400, "session_id 必填，请先创建会话", nil)
		return
	}
	if agentID == 0 {
		agentID = sess.AgentID
	}
	if agentID == 0 {
		dto.ResponseError(c, 400, "agent_uuid/agent_id 必填", nil)
		return
	}
	if _, err := h.ag.Get(c.Request.Context(), env, tenantRef, agentID); err != nil {
		dto.ResponseError(c, 404, "未找到指定的 Agent", err)
		return
	}
	// 若会话标题为空，则用首个问题生成标题（ChatGPT 风格）
	if sess != nil && strings.TrimSpace(sess.Title) == "" && strings.TrimSpace(q) != "" {
		title := runtime.MakeDefaultSessionTitle(q, 24)
		_ = h.his.RenameSession(c, env, tenantRef, sess.ID, title)
	}

	// 3) 适配器 + Engine
	runtime.SetSSEHeaders(c)
	startedAt := time.Now()
	traceID := strings.TrimSpace(reqctx.GetTraceID(c.Request.Context()))
	if traceID == "" {
		traceID = uuid.NewString()
	}
	baseSink := runtime.NewSSESink(c)
	histSink := runtime.NewHistorySink(baseSink, h.his, c, env, tenantRef, sess, agentID, true)
	clientMsgID := strings.TrimSpace(c.Query("client_msg_id"))
	debugReqBody := map[string]any{
		"q":             q,
		"env":           env,
		"agent_id":      agentID,
		"agent_uuid":    strings.TrimSpace(getParam("agent_uuid")),
		"session_id":    sess.ID,
		"session_uuid":  sess.UUID.String(),
		"client_msg_id": clientMsgID,
		"flow_id":       strings.TrimSpace(c.Query("flow_id")),
	}
	debugMeta := map[string]any{
		"tenant_uuid": strings.TrimSpace(tenantCtx.UUID()),
		"request_id":  strings.TrimSpace(c.GetString("request_id")),
	}

	optCfg := agent.GetAgentManager().GetContextOptimizerConfig()
	debugTraceCfg := agent.GetAgentManager().GetDebugTraceConfig()
	if h.ctxOptSvc != nil {
		debugEnabled := false
		if gc := appcfg.GetGlobalConfig(); gc != nil {
			debugEnabled = gc.LogConfig.AgentDebug.Enable
		}
		if activeOpt, err := h.ctxOptSvc.GetActive(c.Request.Context(), env, tenantRef, optCfg, debugEnabled); err == nil && activeOpt != nil {
			optCfg = agent.ContextOptimizerConfig{
				Enabled:                   activeOpt.Config.Enabled,
				MaxPromptTokens:           activeOpt.Config.MaxPromptTokens,
				ReservedCompletionTokens:  activeOpt.Config.ReservedCompletionTokens,
				RecentMessages:            activeOpt.Config.RecentMessages,
				RetrievalTopK:             activeOpt.Config.RetrievalTopK,
				CacheMode:                 activeOpt.Config.CacheMode,
				SummaryRefreshIntervalSec: activeOpt.Config.SummaryRefreshIntervalSec,
			}
			debugTraceCfg.Enabled = activeOpt.Config.DebugTraceEnabled
		}
	}
	debugSink := newStreamDebugTraceSink(
		histSink,
		debugTraceCfg,
		c.Request.Context(),
		traceID,
		debugReqBody,
		debugMeta,
	)
	defer debugSink.Flush()

	// 支持“从某条 user 消息重新生成”：裁剪后续消息并以该消息内容作为 prompt
	if regenFromID > 0 {
		msgRec, err := h.his.FindMessageByID(c, env, tenantRef, regenFromID)
		if err != nil || msgRec == nil || msgRec.SessionID != sess.ID || strings.ToLower(strings.TrimSpace(msgRec.Role)) != "user" {
			_ = debugSink.Emit(dto.EventError, map[string]any{"message": "regen_from_message_id 无效或不属于该会话"})
			_ = debugSink.Emit(dto.EventEnd, map[string]any{"success": false})
			return
		}
		// 允许前端传 q 作为“编辑后的问题”，并更新这条 user 消息内容
		if strings.TrimSpace(q) != "" && strings.TrimSpace(q) != strings.TrimSpace(msgRec.Content) {
			if err := h.his.UpdateMessageContent(c, env, tenantRef, regenFromID, q); err == nil {
				msgRec.Content = strings.TrimSpace(q)
			}
		}
		_, _ = h.his.TruncateMessagesAfter(c, env, tenantRef, sess.ID, regenFromID)
		q = strings.TrimSpace(msgRec.Content)
		_ = debugSink.Emit(dto.EventMeta, map[string]any{
			"session_id":              sess.ID,
			"agent_id":                agentID,
			"regen_from_message_id":   regenFromID,
			"regen_from_message_role": "user",
		})
	} else {
		// 正常发送：先写入 user 消息，并把 DB message_id 回传给前端用于“从此问题重新生成”
		userMsg, _ := h.his.AppendMessage(c, env, tenantRef, sess.ID, agentID, "user", q, "text", 0, 0, false, nil)
		if userMsg != nil {
			_ = debugSink.Emit(dto.EventMeta, map[string]any{
				"session_id":        sess.UUID.String(),
				"session_id_num":    sess.ID,
				"agent_id":          agentID,
				"user_message_id":   userMsg.ID,
				"client_msg_id":     clientMsgID,
				"user_message_role": "user",
			})
		}
	}

	cfg, cfgErr := h.cfgResolver.ResolveForAgentChat(c.Request.Context(), env, tenantRef, agentID, nil)
	if cfgErr != nil {
		_ = debugSink.Emit(dto.EventError, map[string]any{"message": cfgErr.Error()})
		_ = debugSink.Emit(dto.EventEnd, map[string]any{"success": false})
		return
	}
	// 让前端/排障能看到“实际执行用的 provider/model”，避免把模型自报当成事实。
	_ = debugSink.Emit(dto.EventMeta, map[string]any{
		"env":          env,
		"llm_provider": strings.TrimSpace(cfg.Provider),
		"llm_model":    strings.TrimSpace(cfg.ModelName),
	})

	// Context 优化：分层上下文 + 预算裁剪（仅增强 system prompt，不改变用户输入）。
	if optCfg.Enabled {
		cfg.CacheMode = optCfg.CacheMode
		cacheSupported := supportsProviderPromptCache(cfg.Provider)
		cacheEnabled := false
		switch strings.ToLower(strings.TrimSpace(optCfg.CacheMode)) {
		case "force_on":
			cacheEnabled = true
		case "force_off":
			cacheEnabled = false
		default:
			cacheEnabled = cacheSupported
		}
		cctx := agent.CandidateBuildContextFromRequest(c.Request.Context())
		candidates := agent.GetAgentManager().BuildToolCallCandidatesWithContext(cctx, 64)
		candidateSummary := buildCandidateSummary(candidates, 8)
		build, buildErr := h.his.BuildContextForLLM(
			c.Request.Context(),
			env,
			tenantRef,
			sess,
			q,
			cfg.SystemPrompt,
			candidateSummary,
			nil,
			agentcfg.ContextOptimizerConfig{
				Enabled:                   optCfg.Enabled,
				MaxPromptTokens:           optCfg.MaxPromptTokens,
				ReservedCompletionTokens:  optCfg.ReservedCompletionTokens,
				RecentMessages:            optCfg.RecentMessages,
				RetrievalTopK:             optCfg.RetrievalTopK,
				CacheMode:                 optCfg.CacheMode,
				SummaryRefreshIntervalSec: optCfg.SummaryRefreshIntervalSec,
			},
		)
		if buildErr == nil && build != nil {
			cfg.SystemPrompt = strings.TrimSpace(build.SystemPrompt)
			_ = debugSink.Emit(dto.EventMeta, map[string]any{
				"context_optimizer": map[string]any{
					"enabled":                 build.Enabled,
					"prompt_tokens":           build.PromptTokens,
					"reserved_completion":     build.CompletionReserve,
					"context_layers_size":     build.LayerTokenSize,
					"trim_actions":            build.TrimActions,
					"used_structured_summary": build.UsedStructuredMemo,
					"cache_mode":              optCfg.CacheMode,
					"cache_supported":         cacheSupported,
					"cache_enabled":           cacheEnabled,
					"cached_tokens":           0,
				},
			})
		}
	}

	err = runtime.NewEngine().Run(c.Request.Context(), q, cfg, "", debugSink) // explicitFlow 传空，交给意图/plan 选择
	if plannerUsage := agent.GetAgentManager().PopPlannerUsage(traceID); len(plannerUsage) > 0 {
		if hops, ok := plannerUsage["hops"].([]map[string]any); ok {
			for _, hop := range hops {
				debugSink.AddUsageHop(hop)
			}
		} else if hopsAny, ok := plannerUsage["hops"].([]any); ok {
			for _, item := range hopsAny {
				if hop, ok := item.(map[string]any); ok {
					debugSink.AddUsageHop(hop)
				}
			}
		}
	}
	status := "completed"
	if err != nil {
		status = "failed"
	}
	h.recordAgentInvocation(c.Request.Context(), agentStreamCapability, tenantCtx.UUID(), agentID, sess.ID, q, traceID, "rest", status, err, time.Since(startedAt), debugSink.UsageSnapshot())
}

// ---- 核心 ----
func (h *AgentChatHandler) streamCore(c *gin.Context, req dto.StreamChatRequest) {
	msg := strings.TrimSpace(req.Message)
	if msg == "" {
		dto.ResponseError(c, 400, "message 不能为空", nil)
		return
	}
	ctx := c.Request.Context()

	// 解析上下文（容错类型）
	env := reqctx.GetEnv(c.Request.Context())
	tenantCtx, err := requireTenantContext(c)
	if err != nil {
		c.SSEvent(dto.EventError, gin.H{"message": "租户上下文缺失", "detail": err.Error()})
		c.SSEvent(dto.EventEnd, gin.H{"ok": false})
		return
	}
	tenantRef := tenantCtx.UUIDPtr()
	tenantUUID := tenantCtx.UUID()
	userID := reqctx.GetUserID(c.Request.Context())
	agentID, _ := utils.AsUint64(req.Context["agent_id"])
	if agentID == 0 {
		c.SSEvent(dto.EventError, gin.H{"message": "agent_id 缺失"})
		c.SSEvent(dto.EventEnd, gin.H{"ok": false})
		return
	}
	if _, err := h.ag.Get(ctx, env, tenantRef, agentID); err != nil {
		c.SSEvent(dto.EventError, gin.H{"message": "未找到指定的 Agent", "detail": err.Error()})
		c.SSEvent(dto.EventEnd, gin.H{"ok": false})
		return
	}

	// 会话：优先 session_id -> 否则 sticky（env, tenant, agent, user）
	var sess *dbmodel.AgentChatSession
	if sidStr := utils.StrOr(req.Context["session_id"].(string), ""); sidStr != "" {
		if sid, err := utils.ParseUintID(sidStr); err == nil && sid > 0 {
			sess, _ = h.his.FindSessionByID(ctx, env, tenantRef, sid)
		}
	}
	if sess == nil {
		var sessErr error
		sess, sessErr = h.his.GetOrCreateSession(ctx, env, tenantRef, agentID, userID, false, nil)
		if sessErr != nil {
			c.SSEvent(dto.EventError, gin.H{"message": "创建会话失败", "detail": sessErr.Error()})
			c.SSEvent(dto.EventEnd, gin.H{"ok": false})
			return
		}
	}
	if strings.TrimSpace(sess.Title) == "" {
		title := runtime.MakeDefaultSessionTitle(req.Message, 24)
		_ = h.his.RenameSession(c, env, tenantRef, sess.ID, title)
	}

	// 写入 user 消息
	if _, err := h.his.AppendMessage(ctx, env, tenantRef, sess.ID, agentID, "user", msg, "text", 0, 0, false, nil); err != nil {
		c.SSEvent(dto.EventError, gin.H{"message": "写入用户消息失败", "detail": err.Error()})
		c.SSEvent(dto.EventEnd, gin.H{"ok": false})
		return
	}

	// 意图 → 计划（未提供 flow_id 时）
	mgr := agent.GetAgentManager()
	var plan *flowschema.ExecutionPlan
	flowID := strings.TrimSpace(req.FlowID)
	if flowID == "" {
		tasks, err := mgr.DetectTasks(c, msg)
		if err != nil {
			c.SSEvent(dto.EventError, gin.H{"message": "意图识别失败", "detail": err.Error()})
			c.SSEvent(dto.EventEnd, gin.H{"ok": false})
			return
		}
		c.SSEvent(dto.EventIntent, gin.H{"mode": "intent_multi", "tasks": tasks})
		c.Writer.Flush()

		raw := mgr.BuildPlan(tasks)
		switch v := any(raw).(type) {
		case flowschema.ExecutionPlan:
			plan = &v
		case *flowschema.ExecutionPlan:
			plan = v
		default:
			b, _ := json.Marshal(raw)
			var tmp flowschema.ExecutionPlan
			if err := json.Unmarshal(b, &tmp); err == nil && len(tmp.Tasks) > 0 {
				plan = &tmp
			}
		}
		if plan != nil {
			c.SSEvent("plan", plan)
		} else {
			c.SSEvent("plan", raw)
		}
		c.Writer.Flush()

		flowID = runtime.PickFirstFlowID(plan)
	}

	// 路由 & 兜底
	ag, fallbackFlowID, err := mgr.GetDefaultRoute()
	if err != nil {
		c.SSEvent(dto.EventError, gin.H{"message": "获取默认 Agent 失败", "detail": err.Error()})
		c.SSEvent(dto.EventEnd, gin.H{"ok": false})
		return
	}
	if strings.TrimSpace(flowID) == "" {
		flowID = strings.TrimSpace(fallbackFlowID) // e.g. base_flow
		if flowID == "" {
			c.SSEvent(dto.EventError, gin.H{"code": "no_fallback_flow", "message": "未配置默认兜底 flow"})
			c.SSEvent(dto.EventEnd, gin.H{"ok": false})
			return
		}
	}

	// 执行 flow
	params := flowschema.Context{
		"message": msg,
		"context": map[string]any{
			"env":         env,
			"tenant_uuid": tenantUUID,
			"agent_id":    agentID,
			"session_id":  sess.ID,
			"user_id":     userID,
		},
		"plan": plan,
	}
	execID := fmt.Sprintf("sess_%d_%d", sess.ID, time.Now().UnixNano())
	meta := agentschema.ExecutionMeta{
		RequestID:  execID,
		UserID:     userID,
		TenantUUID: tenantUUID,
		TraceID:    fmt.Sprintf("trace_%d", time.Now().UnixNano()),
		Priority:   1,
		Timeout:    60,
	}

	runCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	sr, err := ag.Stream(runCtx, flowID, params, meta)
	if err != nil {
		c.SSEvent(dto.EventError, gin.H{"message": "流式聊天执行失败", "detail": err.Error()})
		c.SSEvent(dto.EventEnd, gin.H{"ok": false})
		return
	}

	// Tap：增量与最终回写
	var buf strings.Builder
	startedAt := time.Now()
	traceID := strings.TrimSpace(reqctx.GetTraceID(ctx))
	if traceID == "" {
		traceID = strings.TrimSpace(meta.TraceID)
	}
	if traceID == "" {
		traceID = uuid.NewString()
	}
	recorded := false
	recordOnce := func(status string, err error) {
		if recorded {
			return
		}
		recorded = true
		h.recordAgentInvocation(ctx, agentStreamCapability, tenantUUID, agentID, sess.ID, msg, traceID, "rest", status, err, time.Since(startedAt), nil)
	}
	hooks := dto.SSEHooks{
		HeartbeatInterval: 25 * time.Second,
		OnStart: func(fid, eid string) {
			c.SSEvent("meta", gin.H{"session_id": sess.ID, "agent_id": agentID})
			c.Writer.Flush()
		},
		OnDelta: func(delta string, _ *agentschema.ExecutionResult) { buf.WriteString(delta) },
		OnFinal: func(final *agentschema.ExecutionResult) {
			text := runtime.ExtractAssistantText(final)
			if strings.TrimSpace(text) == "" {
				text = runtime.SanitizeAssistantVisibleText(buf.String())
			}
			if strings.TrimSpace(text) != "" {
				_, _ = h.his.AppendMessage(c.Request.Context(), env, tenantRef, sess.ID, agentID, "assistant", text, "text", 0, 0, false, nil)
			}
			_, _ = h.his.SummarizeIfNeeded(c.Request.Context(), env, tenantRef, sess)
			recordOnce("completed", nil)
		},
		OnError: func(err error) {
			recordOnce("failed", err)
		},
	}
	_ = dto.WriteToSSEWithTap(c, flowID, execID, sr, hooks)
}

// 非流式（保留）
func (h *AgentChatHandler) Invoke(c *gin.Context) {
	var req agentInvokeRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	h.invokeWithSession(c, req, "")
}

// POST /agents/sessions/:id/invoke
func (h *AgentChatHandler) InvokeSession(c *gin.Context) {
	var req agentInvokeRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	h.invokeWithSession(c, req, strings.TrimSpace(c.Param("id")))
}

// GET /agents/sessions/:id/stream/sse?q=...&env=...
func (h *AgentChatHandler) StreamSessionSSE(c *gin.Context) {
	env, err := resolveAgentEnv(c, h.settings)
	if err != nil {
		dto.ResponseError(c, 400, err.Error(), nil)
		return
	}
	tenantCtx, err := requireTenantContext(c)
	if err != nil {
		dto.ResponseError(c, 400, err.Error(), nil)
		return
	}
	tenantRef := tenantCtx.UUIDPtr()
	sess, err := h.resolveSessionByParam(c, env, tenantRef, strings.TrimSpace(c.Param("id")))
	if err != nil || sess == nil {
		dto.ResponseError(c, 404, "未找到指定的 Session", err)
		return
	}

	c.Set("agent_id", fmt.Sprintf("%d", sess.AgentID))
	c.Set("session_id", fmt.Sprintf("%d", sess.ID))
	c.Set("session_uuid", sess.UUID.String())

	h.StreamSSE(c)
}

func (h *AgentChatHandler) invokeWithSession(c *gin.Context, req agentInvokeRequest, sessionParam string) {
	msg := strings.TrimSpace(req.Message)
	if msg == "" {
		dto.ResponseError(c, 400, "message 不能为空", nil)
		return
	}

	tenantCtx, err := requireTenantContext(c)
	if err != nil {
		dto.ResponseError(c, 400, err.Error(), nil)
		return
	}
	tenantRef := tenantCtx.UUIDPtr()
	tenantUUID := tenantCtx.UUID()
	env, err := resolveAgentEnv(c, h.settings)
	if err != nil {
		dto.ResponseError(c, 400, err.Error(), nil)
		return
	}
	var agentID uint64
	agentIDStr := strings.TrimSpace(req.AgentID)
	if agentIDStr != "" {
		if agentUUID, err := uuid.Parse(agentIDStr); err == nil {
			exist, err := h.ag.GetByUUID(c.Request.Context(), env, tenantRef, agentUUID)
			if err != nil {
				dto.ResponseError(c, 404, "未找到指定的 Agent", err)
				return
			}
			agentID = exist.ID
		} else {
			id, _ := utils.ParseUintID(agentIDStr)
			agentID = id
		}
	}

	// 支持 session_id（body）或 path :id（uuid/数字）
	var sess *dbmodel.AgentChatSession
	if sessionParam != "" {
		sess, _ = h.resolveSessionByParam(c, env, tenantRef, sessionParam)
		if sess != nil {
			agentID = sess.AgentID
		}
	}
	if sess == nil && strings.TrimSpace(req.SessionID) != "" {
		sess, _ = h.resolveSessionByParam(c, env, tenantRef, strings.TrimSpace(req.SessionID))
	}
	if sess == nil {
		dto.ResponseError(c, 400, "session_id 必填，请先创建会话", nil)
		return
	}
	agentID = sess.AgentID
	if _, err := h.ag.Get(c.Request.Context(), env, tenantRef, agentID); err != nil {
		dto.ResponseError(c, 404, "未找到指定的 Agent", err)
		return
	}
	if sess != nil && strings.TrimSpace(sess.Title) == "" {
		title := runtime.MakeDefaultSessionTitle(msg, 24)
		_ = h.his.RenameSession(c, env, tenantRef, sess.ID, title)
	}

	_, _ = h.his.AppendMessage(c.Request.Context(), env, tenantRef, sess.ID, agentID, "user", msg, "text", 0, 0, false, nil)

	cfg, cfgErr := h.cfgResolver.ResolveForAgentChat(c.Request.Context(), env, tenantRef, agentID, nil)
	if cfgErr != nil {
		dto.ResponseError(c, 400, cfgErr.Error(), nil)
		return
	}
	opt := agent.GetAgentManager().GetContextOptimizerConfig()
	if h.ctxOptSvc != nil {
		debugEnabled := false
		if gc := appcfg.GetGlobalConfig(); gc != nil {
			debugEnabled = gc.LogConfig.AgentDebug.Enable
		}
		if activeOpt, err := h.ctxOptSvc.GetActive(c.Request.Context(), env, tenantRef, opt, debugEnabled); err == nil && activeOpt != nil {
			opt = agent.ContextOptimizerConfig{
				Enabled:                   activeOpt.Config.Enabled,
				MaxPromptTokens:           activeOpt.Config.MaxPromptTokens,
				ReservedCompletionTokens:  activeOpt.Config.ReservedCompletionTokens,
				RecentMessages:            activeOpt.Config.RecentMessages,
				RetrievalTopK:             activeOpt.Config.RetrievalTopK,
				CacheMode:                 activeOpt.Config.CacheMode,
				SummaryRefreshIntervalSec: activeOpt.Config.SummaryRefreshIntervalSec,
			}
		}
	}
	if opt.Enabled {
		cfg.CacheMode = opt.CacheMode
	}

	startedAt := time.Now()
	traceID := strings.TrimSpace(reqctx.GetTraceID(c.Request.Context()))
	if traceID == "" {
		traceID = uuid.NewString()
	}
	baseSink := &agentInvokeSink{}
	histSink := runtime.NewHistorySink(baseSink, h.his, c, env, tenantRef, sess, agentID, true)
	traceSink := newPlannerTraceSink(histSink, h.skillAudit, tenantUUID, traceID)
	_, plan, err := runtime.NewEngine().RunPlanInvoke(c.Request.Context(), msg, cfg, "", traceSink)
	status := "completed"
	if err != nil {
		status = "failed"
	}
	h.recordAgentInvocation(c.Request.Context(), agentInvokeCapability, tenantUUID, agentID, sess.ID, msg, traceID, "rest", status, err, time.Since(startedAt), nil)
	if err != nil {
		dto.ResponseError(c, 502, "agent invoke failed", err)
		return
	}

	reply := baseSink.Reply()
	dto.ResponseSuccess(c, gin.H{
		"session_id": sess.UUID.String(),
		"agent_id":   req.AgentID,
		"reply":      reply,
		"plan_id": func() string {
			if plan == nil {
				return ""
			}
			return strings.TrimSpace(plan.PlanID)
		}(),
	})
}

func (h *AgentChatHandler) resolveSessionByParam(c *gin.Context, env string, tenantRef *string, idParam string) (*dbmodel.AgentChatSession, error) {
	idParam = strings.TrimSpace(idParam)
	if idParam == "" {
		return nil, nil
	}
	if id, parseErr := utils.ParseUintID(idParam); parseErr == nil && id > 0 {
		return h.his.FindSessionByID(c.Request.Context(), env, tenantRef, id)
	}
	return h.his.FindSessionByUUID(c.Request.Context(), env, tenantRef, idParam)
}
func resolveAgentEnv(c *gin.Context, settings *agentSvc.AgentSettingService) (string, error) {
	if c == nil {
		return "", fmt.Errorf("env missing")
	}
	env := strings.TrimSpace(reqctx.GetEnv(c.Request.Context()))
	if strings.EqualFold(env, "default") {
		env = ""
	}
	if env == "" {
		if v := strings.TrimSpace(c.Query("env")); v != "" && !strings.EqualFold(v, "default") {
			env = v
		}
	}
	if env == "" {
		if v := strings.TrimSpace(c.GetHeader("X-PowerX-Env")); v != "" && !strings.EqualFold(v, "default") {
			env = v
		}
	}
	if env == "" {
		tenantCtx, err := requireTenantContext(c)
		if err == nil && settings != nil {
			if v, ok, _ := settings.GetTenantCurrentAIEnv(c.Request.Context(), tenantCtx.UUID()); ok {
				env = v
			}
		}
	}
	if strings.TrimSpace(env) == "" {
		return "", fmt.Errorf("env missing")
	}
	return env, nil
}

/* ---------------- helpers ---------------- */

func setSSEHeaders(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream; charset=utf-8")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
}

const (
	agentInvokeCapability = "com.corex.agent.invoke"
	agentStreamCapability = "com.corex.agent.stream"
	platformPluginID      = "corex.platform"
)

func (h *AgentChatHandler) recordAgentInvocation(ctx context.Context, capabilityID, tenantUUID string, agentID, sessionID uint64, message, traceID, protocol, status string, err error, latency time.Duration, usage map[string]any) {
	if h == nil || h.audit == nil {
		return
	}
	if strings.TrimSpace(tenantUUID) == "" {
		return
	}
	payload := map[string]interface{}{
		"agent_id":   agentID,
		"session_id": sessionID,
		"message":    message,
	}
	response := map[string]interface{}{
		"status": status,
	}
	if len(usage) > 0 {
		response["usage"] = usage
	}
	errorSummary := ""
	if err != nil {
		errorSummary = err.Error()
	}
	h.audit.RecordInvocation(ctx, capservice.InvocationAuditInput{
		TraceID:           traceID,
		TenantUUID:        tenantUUID,
		PluginID:          platformPluginID,
		CapabilityID:      capabilityID,
		PreferredProtocol: protocol,
		ProtocolUsed:      protocol,
		FallbackUsed:      false,
		Status:            status,
		RequestPayload:    payload,
		ResponsePayload:   response,
		ErrorSummary:      errorSummary,
		Latency:           latency,
	})
}
