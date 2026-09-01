// file: internal/app/http/admin/agent/chat_handler.go
package agent

import (
	"context"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	appcfg "github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/ArtisanCloud/PowerX/internal/server/agent"
	agentcfg "github.com/ArtisanCloud/PowerX/internal/server/agent/config"
	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	agentrepo "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/repository"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/runtime"
	agentschema "github.com/ArtisanCloud/PowerX/internal/server/agent/schemas"
	agentSvc "github.com/ArtisanCloud/PowerX/internal/service/agent"
	capservice "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	skillservice "github.com/ArtisanCloud/PowerX/internal/service/skills"
	modelagent "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/agent"
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
	skillBinds  *agentrepo.AgentSkillBindingRepository
	skillStates *agentSvc.SkillStateService
	teams       *agentSvc.TeamService
}

type runtimeSkillStateStore struct {
	service *agentSvc.SkillStateService
}

func (s runtimeSkillStateStore) UpsertSkillState(ctx context.Context, in runtime.SkillStateUpsert) error {
	if s.service == nil {
		return fmt.Errorf("skill state service is not configured")
	}
	_, err := s.service.Upsert(ctx, agentSvc.SkillStateUpsertInput{
		Env:           in.Env,
		TenantUUID:    in.TenantUUID,
		SessionID:     in.SessionID,
		AgentID:       in.AgentID,
		SkillID:       in.SkillID,
		StateKey:      in.StateKey,
		SchemaVersion: in.SchemaVersion,
		Status:        in.Status,
		Action:        in.Action,
		State:         in.State,
		Meta:          in.Meta,
		LastMessageID: in.LastMessageID,
		TTLSeconds:    in.TTLSeconds,
	})
	return err
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
	case dto.EventIntent, dto.EventPlan, dto.EventNodeStart, dto.EventNodeEnd, dto.EventFinal, dto.EventError, dto.EventEnd,
		dto.EventAgentRunStarted, dto.EventAgentRunResponsePlan, dto.EventAgentRunIntentDetected, dto.EventAgentRunPlanCreated,
		dto.EventAgentRunTaskStatus, dto.EventAgentRunTaskStarted, dto.EventAgentRunAwaitingParams, dto.EventAgentRunTaskCompleted,
		dto.EventAgentRunTaskFailed, dto.EventAgentRunFinal, dto.EventAgentRunEnded:
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
		TraceID:        scopedPlannerTraceID(s.traceID, nodeID),
		TenantUUID:     s.tenantUUID,
		SkillID:        nodeRef,
		Version:        "",
		Entrypoint:     nodeID,
		InvokePath:     "agent.invoke.plan",
		ProtocolUsed:   "agent." + nodeKind,
		Status:         status,
		PlanID:         planID,
		NodeID:         nodeID,
		TeamID:         strings.TrimSpace(readStringAny(m["team_id"])),
		HandoffTaskID:  strings.TrimSpace(readStringAny(m["handoff_task_id"])),
		HandoffTraceID: strings.TrimSpace(readStringAny(m["handoff_trace_id"])),
		NodeStatus:     status,
		RetryTrace:     strings.TrimSpace(readStringAny(m["error"])),
		LatencyMS:      0,
		AuthPass:       true,
	})
}

func scopedPlannerTraceID(baseTraceID, nodeID string) string {
	base := strings.TrimSpace(baseTraceID)
	node := strings.TrimSpace(nodeID)
	if base == "" {
		return uuid.NewString()
	}
	if node == "" {
		return base
	}
	candidate := base + ":" + node
	if len(candidate) <= 120 {
		return candidate
	}
	sum := sha1.Sum([]byte(candidate))
	baseShort := base
	if len(baseShort) > 64 {
		baseShort = baseShort[:64]
	}
	return fmt.Sprintf("%s:%x", baseShort, sum[:8])
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
		name := strings.TrimSpace(c.DisplayName)
		if name == "" {
			name = strings.TrimSpace(c.Name)
		}
		if name == "" {
			name = strings.TrimSpace(c.NodeRef)
		}
		if name == "" {
			continue
		}
		desc := strings.TrimSpace(c.Description)
		if desc == "" {
			desc = strings.TrimSpace(c.SemanticText)
		}
		ref := strings.TrimSpace(c.NodeRef)
		if ref == "" {
			ref = strings.TrimSpace(c.FlowID)
		}
		lines := []string{"- " + name}
		if desc != "" {
			lines = append(lines, "  说明: "+trimCandidateRunes(desc, 180))
		}
		if len(c.Actions) > 0 {
			lines = append(lines, "  可用动作: "+strings.Join(c.Actions, ", "))
		}
		if len(c.RequiredArgs) > 0 {
			lines = append(lines, "  必要参数: "+strings.Join(c.RequiredArgs, ", "))
		}
		if len(c.ActionRequiredArgs) > 0 {
			lines = append(lines, "  动作必填参数: "+formatActionArgMap(c.ActionRequiredArgs, 4))
		}
		if len(c.OptionalArgs) > 0 {
			lines = append(lines, "  可选参数: "+strings.Join(c.OptionalArgs, ", "))
		}
		if len(c.ActionOptionalArgs) > 0 {
			lines = append(lines, "  动作可选参数: "+formatActionArgMap(c.ActionOptionalArgs, 4))
		}
		if len(c.Examples) > 0 {
			examples := c.Examples
			if len(examples) > 3 {
				examples = examples[:3]
			}
			lines = append(lines, "  示例问法: "+strings.Join(examples, "；"))
		}
		if len(c.ResponseGuidance) > 0 {
			guidance := c.ResponseGuidance
			if len(guidance) > 5 {
				guidance = guidance[:5]
			}
			lines = append(lines, "  回复规范: "+strings.Join(guidance, "；"))
		}
		buckets[kind] = append(buckets[kind], strings.Join(lines, "\n"))
	}
	parts := make([]string, 0, 4)
	parts = append(parts, strings.Join([]string{
		"下面是当前 Agent 已绑定/可用的能力上下文。",
		"当用户询问 Agent 是什么、能做什么、有哪些能力或如何使用时：",
		"- 请用对话式自然语言回答，像助手在介绍自己，不要机械复述上下文。",
		"- 先用一句话概括当前 Agent 的定位。",
		"- 再用 Markdown 分点说明每项能力能做什么，并给 1-2 个用户可以直接说的示例。",
		"- 不要逐字照抄能力说明；请把能力说明改写成用户容易理解的话。",
		"- 输出中保留自然换行，避免把多个示例挤在同一行。",
		"- 只介绍当前已绑定能力，不要编造未绑定能力。",
		"- 不要只输出 ref、机器 ID、字段名或原始 schema。",
	}, "\n"))
	for _, kind := range []string{"workflow", "skill", "tooling", "llm"} {
		items := buckets[kind]
		if len(items) == 0 {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s(%d):\n%s", kind, len(items), strings.Join(items, "\n")))
	}
	return strings.Join(parts, "\n")
}

func responseCapabilityItemsFromCandidates(cands []agent.ToolCallCandidate) []runtime.CapabilityContextItem {
	out := make([]runtime.CapabilityContextItem, 0, len(cands))
	for _, c := range cands {
		kind := strings.ToLower(strings.TrimSpace(c.NodeKind))
		if kind != "skill" && kind != "tooling" && kind != "workflow" {
			continue
		}
		id := strings.TrimSpace(c.NodeRef)
		if id == "" {
			id = strings.TrimSpace(c.Name)
		}
		if id == "" {
			continue
		}
		title := strings.TrimSpace(c.DisplayName)
		if title == "" {
			title = strings.TrimSpace(c.Name)
		}
		out = append(out, runtime.CapabilityContextItem{
			ID:                 id,
			Title:              title,
			Description:        strings.TrimSpace(c.Description),
			RequiredArgs:       append([]string(nil), c.RequiredArgs...),
			ActionRequiredArgs: copyStringSliceMap(c.ActionRequiredArgs),
			ActionOptionalArgs: copyStringSliceMap(c.ActionOptionalArgs),
			SlotMapping:        copyAnyMap(c.SlotMapping),
			PendingTaskPolicy:  copyAnyMap(c.PendingTaskPolicy),
			StateContract:      copyAnyMap(c.StateContract),
			ResultPresentation: copyAnyMap(c.ResultPresentation),
			OptionalArgs:       append([]string(nil), c.OptionalArgs...),
			Actions:            append([]string(nil), c.Actions...),
			Examples:           append([]string(nil), c.Examples...),
			ResponseGuidance:   append([]string(nil), c.ResponseGuidance...),
			NodeKind:           kind,
			Source:             strings.TrimSpace(c.Source),
		})
	}
	return out
}

func formatActionArgMap(in map[string][]string, maxActions int) string {
	if len(in) == 0 {
		return ""
	}
	keys := make([]string, 0, len(in))
	for key := range in {
		if strings.TrimSpace(key) != "" {
			keys = append(keys, strings.TrimSpace(key))
		}
	}
	sort.Strings(keys)
	if maxActions > 0 && len(keys) > maxActions {
		keys = keys[:maxActions]
	}
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"("+strings.Join(in[key], ", ")+")")
	}
	return strings.Join(parts, "；")
}

func copyAnyMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func copyStringSliceMap(in map[string][]string) map[string][]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string][]string, len(in))
	for k, values := range in {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		out[key] = append([]string(nil), values...)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func responseCapabilityIDs(items []runtime.CapabilityContextItem) []string {
	out := make([]string, 0, len(items))
	seen := map[string]struct{}{}
	for _, item := range items {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			continue
		}
		key := strings.ToLower(id)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, id)
	}
	return out
}

func trimCandidateRunes(s string, maxRunes int) string {
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

func (h *AgentChatHandler) listBoundSkillIDs(ctx context.Context, env string, tenantRef *string, agentID uint64) ([]string, error) {
	if h == nil || h.skillBinds == nil {
		return nil, nil
	}
	rows, err := h.skillBinds.ListByAgent(ctx, env, tenantRef, agentID)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		id := strings.TrimSpace(row.SkillID)
		if id != "" {
			out = append(out, id)
		}
	}
	return out, nil
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
		skillBinds:  agentrepo.NewAgentSkillBindingRepository(dep.DB),
		skillStates: agentSvc.NewSkillStateService(dep.DB),
		teams:       agentSvc.NewTeamService(dep.DB),
	}
}

func (h *AgentChatHandler) resolveTeamRuntimeContext(ctx context.Context, env, tenantUUID string, agentID, teamID, parentAgentID uint64) (map[string]any, error) {
	if teamID == 0 {
		return nil, nil
	}
	if h == nil || h.teams == nil {
		return nil, fmt.Errorf("agent team service is not configured")
	}
	team, err := h.teams.ValidateTeamTenant(ctx, teamID, tenantUUID)
	if err != nil {
		return nil, fmt.Errorf("团队不可用: %w", err)
	}
	if parentAgentID > 0 && parentAgentID != team.ParentAgentID {
		return nil, fmt.Errorf("parent_agent_id 与团队主智能体不一致")
	}
	if agentID != team.ParentAgentID {
		return nil, fmt.Errorf("agent_id 与团队主智能体不一致")
	}
	if !strings.EqualFold(strings.TrimSpace(team.Status), modelagent.TeamStatusActive) {
		return nil, fmt.Errorf("团队未启用")
	}
	spec, err := modelagent.ParseTeamOrchestrationSpec(team.OrchestrationSpec)
	if err != nil {
		return nil, fmt.Errorf("团队编排不可执行: %w", err)
	}
	var orchestration map[string]any
	if err = json.Unmarshal(team.OrchestrationSpec, &orchestration); err != nil || len(orchestration) == 0 {
		return nil, fmt.Errorf("团队编排读取失败")
	}
	members, err := h.teams.ListMembers(ctx, teamID, tenantUUID)
	if err != nil {
		return nil, fmt.Errorf("读取团队成员失败: %w", err)
	}
	if len(members) == 0 {
		return nil, fmt.Errorf("团队没有可用子智能体")
	}
	tenantRef := strings.TrimSpace(tenantUUID)
	outMembers := make([]map[string]any, 0, len(members))
	for _, member := range members {
		child, err := h.ag.Get(ctx, env, &tenantRef, member.ChildAgentID)
		if err != nil {
			return nil, fmt.Errorf("团队子智能体不可用: child_agent_id=%d", member.ChildAgentID)
		}
		skillIDs, skillErr := h.listBoundSkillIDs(ctx, env, &tenantRef, member.ChildAgentID)
		if skillErr != nil {
			return nil, fmt.Errorf("读取团队子智能体技能失败: child_agent_id=%d", member.ChildAgentID)
		}
		outMembers = append(outMembers, map[string]any{
			"child_agent_id":  member.ChildAgentID,
			"child_agent_key": strings.TrimSpace(child.Key),
			"role":            strings.TrimSpace(member.Role),
			"priority":        member.Priority,
			"skill_ids":       skillIDs,
		})
	}
	// Parsing above is intentionally retained even though the runtime parses the
	// map again: invalid configuration fails before the response planner may
	// silently route this message to normal chat.
	_ = spec
	return map[string]any{
		"agent_workspace_mode":   "team",
		"team_id":                fmt.Sprintf("%d", team.ID),
		"team_key":               strings.TrimSpace(team.TeamKey),
		"team_display_name_i18n": json.RawMessage(team.DisplayNameI18n),
		"parent_agent_id":        team.ParentAgentID,
		"team_members":           outMembers,
		"team_orchestration":     orchestration,
	}, nil
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
	originTenantUUID := strings.TrimSpace(getParam("origin_tenant_uuid"))
	if originTenantUUID != "" {
		canonicalOriginTenantUUID, canonicalErr := reqctx.CanonicalTenantUUID(originTenantUUID)
		if canonicalErr != nil {
			dto.ResponseError(c, 400, "origin_tenant_uuid 非法", canonicalErr)
			return
		}
		originTenantUUID = canonicalOriginTenantUUID
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
	teamID, _ := utils.ParseUintID(getParam("team_id"))
	parentAgentID, _ := utils.ParseUintID(getParam("parent_agent_id"))
	teamRuntimeContext, err := h.resolveTeamRuntimeContext(c.Request.Context(), env, tenantCtx.UUID(), agentID, teamID, parentAgentID)
	if err != nil {
		dto.ResponseError(c, 400, err.Error(), err)
		return
	}
	boundSkillIDs, err := h.listBoundSkillIDs(c.Request.Context(), env, tenantRef, agentID)
	if err != nil {
		dto.ResponseError(c, 500, "读取 Agent 绑定技能失败", err)
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
	histSink := runtime.NewHistorySink(baseSink, h.his, c, env, tenantRef, sess, agentID, true).WithSkillStateService(h.skillStates)
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
	for key, value := range teamRuntimeContext {
		debugReqBody[key] = value
	}
	debugMeta := map[string]any{
		"tenant_uuid": strings.TrimSpace(tenantCtx.UUID()),
		"request_id":  strings.TrimSpace(c.GetString("request_id")),
	}
	if originTenantUUID != "" {
		debugMeta["origin_tenant_uuid"] = originTenantUUID
	}
	optCfg := agent.GetAgentManager().GetContextOptimizerConfig()
	plannerCfg := agent.GetAgentManager().GetPlannerOptimizerConfig()
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
			plannerCfg = agent.PlannerOptimizerConfig{
				Enabled:       activeOpt.Config.PlannerEnabled,
				CandidateTopK: activeOpt.Config.PlannerCandidateTopK,
				PerKindQuota: agent.PlannerKindQuota{
					Workflow: activeOpt.Config.PlannerQuotaWorkflow,
					Skill:    activeOpt.Config.PlannerQuotaSkill,
					Tooling:  activeOpt.Config.PlannerQuotaTooling,
					LLM:      activeOpt.Config.PlannerQuotaLLM,
				},
				PromptSlimMode:       activeOpt.Config.PlannerPromptSlimMode,
				DecisionCacheEnabled: activeOpt.Config.PlannerDecisionCacheEnabled,
				DecisionCacheTTLSec:  activeOpt.Config.PlannerDecisionCacheTTLSec,
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

	userMessageID := uint64(0)
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
		userMessageID = msgRec.ID
		_ = debugSink.Emit(dto.EventMeta, map[string]any{
			"session_id":              sess.ID,
			"agent_id":                agentID,
			"regen_from_message_id":   regenFromID,
			"regen_from_message_role": "user",
		})
	} else {
		// 正常发送：先写入 user 消息，并把 DB message_id 回传给前端用于“从此问题重新生成”
		if strings.TrimSpace(q) != "" {
			userMsg, _ := h.his.AppendMessage(c, env, tenantRef, sess.ID, agentID, "user", q, "text", 0, 0, false, nil)
			if userMsg != nil {
				userMessageID = userMsg.ID
				_ = debugSink.Emit(dto.EventMeta, map[string]any{
					"tenant_uuid":       strings.TrimSpace(tenantCtx.UUID()),
					"trace_id":          traceID,
					"session_id":        fmt.Sprintf("%d", sess.ID),
					"session_uuid":      sess.UUID.String(),
					"session_id_num":    sess.ID,
					"agent_id":          agentID,
					"user_message_id":   userMsg.ID,
					"message_id":        fmt.Sprintf("%d", userMsg.ID),
					"client_msg_id":     clientMsgID,
					"user_message_role": "user",
				})
			}
		}
	}

	cfg, cfgErr := h.cfgResolver.ResolveForAgentChat(c.Request.Context(), env, tenantRef, agentID, nil)
	if cfgErr != nil {
		_ = debugSink.Emit(dto.EventError, map[string]any{"message": cfgErr.Error()})
		_ = debugSink.Emit(dto.EventEnd, map[string]any{"success": false})
		return
	}
	modelPolicy := runtime.BuildDefaultNodeModelPolicy(cfg)
	runCtx := reqctx.WithTraceID(c.Request.Context(), traceID)
	// 运行标识必须在 RunStateSink 建立之前生成；否则 SSE/历史消息会缺少
	// run_id，刷新后无法精确打开这条消息对应的 Trace 报告。
	runCtx = context.WithValue(runCtx, "run_id", fmt.Sprintf("run_%d", time.Now().UnixNano()))
	runCtx = context.WithValue(runCtx, "runId", runCtx.Value("run_id"))
	runCtx = context.WithValue(runCtx, "env", env)
	runCtx = context.WithValue(runCtx, "agent_env", env)
	if authz := strings.TrimSpace(c.GetHeader("Authorization")); authz != "" {
		runCtx = context.WithValue(runCtx, "authorization", authz)
	}
	runCtx = context.WithValue(runCtx, "tenant_uuid", strings.TrimSpace(tenantCtx.UUID()))
	if originTenantUUID != "" {
		runCtx = context.WithValue(runCtx, "origin_tenant_uuid", originTenantUUID)
	}
	runCtx = context.WithValue(runCtx, "session_id", fmt.Sprintf("%d", sess.ID))
	runCtx = context.WithValue(runCtx, "sessionId", fmt.Sprintf("%d", sess.ID))
	runCtx = context.WithValue(runCtx, "session_uuid", sess.UUID.String())
	runCtx = context.WithValue(runCtx, "sessionUuid", sess.UUID.String())
	if userMessageID > 0 {
		runCtx = context.WithValue(runCtx, "message_id", fmt.Sprintf("%d", userMessageID))
		runCtx = context.WithValue(runCtx, "messageId", fmt.Sprintf("%d", userMessageID))
	}
	runCtx = context.WithValue(runCtx, "planner_optimizer_enabled", plannerCfg.Enabled)
	runCtx = context.WithValue(runCtx, "planner_optimizer_candidate_top_k", plannerCfg.CandidateTopK)
	runCtx = context.WithValue(runCtx, "planner_optimizer_prompt_slim_mode", plannerCfg.PromptSlimMode)
	runCtx = context.WithValue(runCtx, "planner_optimizer_decision_cache_enabled", plannerCfg.DecisionCacheEnabled)
	runCtx = context.WithValue(runCtx, "planner_optimizer_decision_cache_ttl_sec", plannerCfg.DecisionCacheTTLSec)
	runCtx = context.WithValue(runCtx, "planner_optimizer_quota_workflow", plannerCfg.PerKindQuota.Workflow)
	runCtx = context.WithValue(runCtx, "planner_optimizer_quota_skill", plannerCfg.PerKindQuota.Skill)
	runCtx = context.WithValue(runCtx, "planner_optimizer_quota_tooling", plannerCfg.PerKindQuota.Tooling)
	runCtx = context.WithValue(runCtx, "planner_optimizer_quota_llm", plannerCfg.PerKindQuota.LLM)
	runCtx = context.WithValue(runCtx, "agent_id", fmt.Sprintf("%d", agentID))
	runCtx = context.WithValue(runCtx, "agentId", fmt.Sprintf("%d", agentID))
	for key, value := range teamRuntimeContext {
		runCtx = context.WithValue(runCtx, key, value)
	}
	runCtx = context.WithValue(runCtx, "agent_bound_skill_ids", boundSkillIDs)
	runCtx = context.WithValue(runCtx, "agentBoundSkillIDs", boundSkillIDs)
	locale, localeErr := declaredAgentLocale(map[string]interface{}{"locale": getParam("locale")})
	if localeErr != nil {
		dto.ResponseError(c, 400, localeErr.Error(), nil)
		return
	}
	runCtx = context.WithValue(runCtx, "locale", locale)
	runCtx = runtime.ContextWithSkillStateStore(runCtx, runtimeSkillStateStore{service: h.skillStates})
	var pendingTask map[string]any
	if latestPendingTask, ok, err := h.latestRuntimePendingTask(c.Request.Context(), env, tenantRef, sess.ID, agentID, boundSkillIDs); err == nil && ok {
		pendingTask = map[string]any(latestPendingTask)
		runCtx = context.WithValue(runCtx, "agent_pending_task", pendingTask)
	}
	runCtx = context.WithValue(runCtx, "agent_node_model_policy", modelPolicy)
	runCtx = context.WithValue(runCtx, "agent_model_intent_provider", modelPolicy.Selection(runtime.ModelPolicyNodeIntent).Provider)
	runCtx = context.WithValue(runCtx, "agent_model_intent_model", modelPolicy.Selection(runtime.ModelPolicyNodeIntent).Model)
	runCtx = context.WithValue(runCtx, "agent_model_planner_provider", modelPolicy.Selection(runtime.ModelPolicyNodePlanner).Provider)
	runCtx = context.WithValue(runCtx, "agent_model_planner_model", modelPolicy.Selection(runtime.ModelPolicyNodePlanner).Model)
	runCtx = context.WithValue(runCtx, "agent_model_response_planner_provider", modelPolicy.Selection(runtime.ModelPolicyNodeResponsePlanner).Provider)
	runCtx = context.WithValue(runCtx, "agent_model_response_planner_model", modelPolicy.Selection(runtime.ModelPolicyNodeResponsePlanner).Model)
	runCtx = context.WithValue(runCtx, "agent_model_final_provider", modelPolicy.Selection(runtime.ModelPolicyNodeFinalResponse).Provider)
	runCtx = context.WithValue(runCtx, "agent_model_final_model", modelPolicy.Selection(runtime.ModelPolicyNodeFinalResponse).Model)
	runSink := runtime.NewRunStateSink(runCtx, debugSink)
	_ = runSink.Emit(dto.EventStart, map[string]any{
		"flow_id":      "agent_runtime",
		"execution_id": traceID,
		"status":       "running",
	})
	// 让前端/排障能看到“实际执行用的 provider/model”，避免把模型自报当成事实。
	_ = debugSink.Emit(dto.EventMeta, map[string]any{
		"env":          env,
		"llm_provider": strings.TrimSpace(cfg.Provider),
		"llm_model":    strings.TrimSpace(cfg.ModelName),
		"model_policy": modelPolicy,
		"bound_skills": boundSkillIDs,
	})

	cctx := agent.CandidateBuildContextFromRequest(runCtx)
	candidates := agent.GetAgentManager().BuildToolCallCandidatesWithContext(cctx, 64)
	capabilityItems := responseCapabilityItemsFromCandidates(candidates)
	capabilityIDs := responseCapabilityIDs(capabilityItems)
	recentIntro, _ := h.his.HasRecentCapabilityIntro(c.Request.Context(), env, tenantRef, sess.ID, capabilityIDs, 12)
	_ = runSink.Emit(dto.EventNodeStart, map[string]any{
		"task_id":    "response_planner",
		"node_kind":  dto.NodeKindLLM,
		"node_ref":   "response_planner",
		"stage":      1,
		"action":     "plan_response",
		"agent_id":   fmt.Sprintf("%d", agentID),
		"agent_name": "Response Planner",
	})
	responsePlan, responsePlanErr := runtime.NewResponsePlanner().Plan(runCtx, runtime.ResponsePlanInput{
		UserMessage:           q,
		PlanHasExecutableNode: teamRuntimeContextHasExecutablePlan(teamRuntimeContext),
		AllowedCapabilities:   capabilityItems,
		RecentCapabilityIntro: recentIntro,
		PendingTask:           pendingTask,
		ModelSelection:        modelPolicy.Selection(runtime.ModelPolicyNodeResponsePlanner),
		TenantUUID:            strings.TrimSpace(tenantCtx.UUID()),
		AgentID:               fmt.Sprintf("%d", agentID),
		SessionID:             fmt.Sprintf("%d", sess.ID),
		MessageID:             fmt.Sprintf("%d", userMessageID),
		TraceID:               traceID,
	})
	if responsePlanErr != nil {
		_ = runSink.Emit(dto.EventNodeEnd, map[string]any{
			"task_id":   "response_planner",
			"node_kind": dto.NodeKindLLM,
			"node_ref":  "response_planner",
			"stage":     1,
			"status":    dto.AgentTaskStatusFailed,
			"error":     responsePlanErr.Error(),
		})
		_ = debugSink.Emit(dto.EventError, map[string]any{"code": runtime.ErrCodeResponsePlanInvalid, "message": responsePlanErr.Error()})
		_ = debugSink.Emit(dto.EventEnd, map[string]any{"success": false})
		return
	}
	_ = runSink.Emit(dto.EventNodeEnd, map[string]any{
		"task_id":    "response_planner",
		"node_kind":  dto.NodeKindLLM,
		"node_ref":   "response_planner",
		"stage":      1,
		"status":     dto.AgentTaskStatusCompleted,
		"result":     map[string]any{"response_mode": responsePlan.ResponseMode, "target_capability_ids": responsePlan.TargetCapabilityIDs},
		"agent_id":   fmt.Sprintf("%d", agentID),
		"agent_name": "Response Planner",
	})
	runCtx = context.WithValue(runCtx, "agent_response_plan", responsePlan.ToContextValue())
	_ = runSink.Emit("response_plan", responsePlan.ToDebugEvent())

	// Context 优化：分层上下文 + 预算裁剪（仅增强 system prompt，不改变用户输入）。
	if optCfg.Enabled || responsePlan.UseCapabilityCtx {
		_ = runSink.Emit(dto.EventNodeStart, map[string]any{
			"task_id":    "context_builder",
			"node_kind":  dto.NodeKindLLM,
			"node_ref":   "context_builder",
			"stage":      2,
			"action":     "build_context",
			"agent_id":   fmt.Sprintf("%d", agentID),
			"agent_name": "Context Builder",
		})
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
		candidateSummary := buildCandidateSummary(candidates, 8)
		build, buildErr := h.his.BuildContextForLLMWithResponsePlan(
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
			&agentSvc.ResponseContextOptions{
				ResponseMode:        string(responsePlan.ResponseMode),
				TargetCapabilityIDs: responsePlan.TargetCapabilityIDs,
				IncludeExamples:     responsePlan.IncludeExamples,
				IncludeSchema:       responsePlan.IncludeSchema,
				RepeatFullIntro:     responsePlan.RepeatFullIntro,
			},
		)
		if buildErr == nil && build != nil {
			cfg.SystemPrompt = runtime.BuildModeSpecificSystemPrompt(strings.TrimSpace(build.SystemPrompt), responsePlan)
			runCtx = context.WithValue(runCtx, "agent_response_context_layers", build.UsedContextLayers)
			_ = runSink.Emit(dto.EventNodeEnd, map[string]any{
				"task_id":    "context_builder",
				"node_kind":  dto.NodeKindLLM,
				"node_ref":   "context_builder",
				"stage":      2,
				"status":     dto.AgentTaskStatusCompleted,
				"result":     map[string]any{"used_context_layers": build.UsedContextLayers, "prompt_tokens": build.PromptTokens},
				"agent_id":   fmt.Sprintf("%d", agentID),
				"agent_name": "Context Builder",
			})
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
					"response_mode":           responsePlan.ResponseMode,
					"used_context_layers":     build.UsedContextLayers,
				},
			})
		} else if buildErr != nil {
			_ = runSink.Emit(dto.EventNodeEnd, map[string]any{
				"task_id":   "context_builder",
				"node_kind": dto.NodeKindLLM,
				"node_ref":  "context_builder",
				"stage":     2,
				"status":    dto.AgentTaskStatusFailed,
				"error":     buildErr.Error(),
			})
		} else if build == nil {
			_ = runSink.Emit(dto.EventNodeEnd, map[string]any{
				"task_id":    "context_builder",
				"node_kind":  dto.NodeKindLLM,
				"node_ref":   "context_builder",
				"stage":      2,
				"status":     dto.AgentTaskStatusCompleted,
				"result":     map[string]any{"used_context_layers": []string{}},
				"agent_id":   fmt.Sprintf("%d", agentID),
				"agent_name": "Context Builder",
			})
		}
	} else {
		cfg.SystemPrompt = runtime.BuildModeSpecificSystemPrompt(cfg.SystemPrompt, responsePlan)
	}
	_ = debugSink.Emit(dto.EventMeta, map[string]any{
		"planner_optimizer": map[string]any{
			"enabled":                plannerCfg.Enabled,
			"candidate_top_k":        plannerCfg.CandidateTopK,
			"prompt_slim_mode":       plannerCfg.PromptSlimMode,
			"decision_cache_enabled": plannerCfg.DecisionCacheEnabled,
			"decision_cache_ttl_sec": plannerCfg.DecisionCacheTTLSec,
			"quota_workflow":         plannerCfg.PerKindQuota.Workflow,
			"quota_skill":            plannerCfg.PerKindQuota.Skill,
			"quota_tooling":          plannerCfg.PerKindQuota.Tooling,
			"quota_llm":              plannerCfg.PerKindQuota.LLM,
		},
	})

	err = runtime.NewEngine().Run(runCtx, q, cfg, "", runSink) // explicitFlow 传空，交给意图/plan 选择
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

func declaredAgentLocale(meta map[string]interface{}) (string, error) {
	raw, _ := meta["locale"].(string)
	switch strings.TrimSpace(raw) {
	case "zh", "zh-CN":
		return "zh-CN", nil
	case "en", "en-US":
		return "en-US", nil
	case "ja":
		return "ja", nil
	case "ko":
		return "ko", nil
	default:
		return "", fmt.Errorf("agent_chat_locale_required_or_invalid")
	}
}

func teamRuntimeContextHasExecutablePlan(teamRuntimeContext map[string]any) bool {
	spec, ok := teamRuntimeContext["team_orchestration"].(map[string]any)
	return ok && len(spec) > 0
}

// ---- 核心 ----
func (h *AgentChatHandler) streamCore(c *gin.Context, req dto.StreamChatRequest) {
	msg := strings.TrimSpace(req.Message)
	if msg == "" {
		dto.ResponseError(c, 400, "message 不能为空", nil)
		return
	}
	ctx := c.Request.Context()
	locale, localeErr := declaredAgentLocale(req.Context)
	if localeErr != nil {
		c.SSEvent(dto.EventError, gin.H{"message": localeErr.Error()})
		c.SSEvent(dto.EventEnd, gin.H{"ok": false})
		return
	}

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
			"locale":      locale,
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

	userMsg, _ := h.his.AppendMessage(c.Request.Context(), env, tenantRef, sess.ID, agentID, "user", msg, "text", 0, 0, false, nil)

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
	boundSkillIDs, err := h.listBoundSkillIDs(c.Request.Context(), env, tenantRef, agentID)
	if err != nil {
		dto.ResponseError(c, 500, "读取 Agent 绑定技能失败", err)
		return
	}
	runCtx := reqctx.WithTraceID(c.Request.Context(), traceID)
	runCtx = context.WithValue(runCtx, "run_id", fmt.Sprintf("run_%d", time.Now().UnixNano()))
	runCtx = context.WithValue(runCtx, "runId", runCtx.Value("run_id"))
	runCtx = context.WithValue(runCtx, "env", env)
	runCtx = context.WithValue(runCtx, "agent_env", env)
	if authz := strings.TrimSpace(c.GetHeader("Authorization")); authz != "" {
		runCtx = context.WithValue(runCtx, "authorization", authz)
	}
	runCtx = context.WithValue(runCtx, "tenant_uuid", strings.TrimSpace(tenantUUID))
	runCtx = context.WithValue(runCtx, "session_id", fmt.Sprintf("%d", sess.ID))
	runCtx = context.WithValue(runCtx, "sessionId", fmt.Sprintf("%d", sess.ID))
	runCtx = context.WithValue(runCtx, "session_uuid", sess.UUID.String())
	runCtx = context.WithValue(runCtx, "sessionUuid", sess.UUID.String())
	if userMsg != nil && userMsg.ID > 0 {
		runCtx = context.WithValue(runCtx, "message_id", fmt.Sprintf("%d", userMsg.ID))
		runCtx = context.WithValue(runCtx, "messageId", fmt.Sprintf("%d", userMsg.ID))
	}
	runCtx = context.WithValue(runCtx, "agent_id", fmt.Sprintf("%d", agentID))
	runCtx = context.WithValue(runCtx, "agentId", fmt.Sprintf("%d", agentID))
	runCtx = context.WithValue(runCtx, "agent_bound_skill_ids", boundSkillIDs)
	runCtx = context.WithValue(runCtx, "agentBoundSkillIDs", boundSkillIDs)
	runCtx = runtime.ContextWithSkillStateStore(runCtx, runtimeSkillStateStore{service: h.skillStates})
	if pendingTask, ok, err := h.latestRuntimePendingTask(c.Request.Context(), env, tenantRef, sess.ID, agentID, boundSkillIDs); err == nil && ok {
		runCtx = context.WithValue(runCtx, "agent_pending_task", map[string]any(pendingTask))
	}
	baseSink := &agentInvokeSink{}
	histSink := runtime.NewHistorySink(baseSink, h.his, c, env, tenantRef, sess, agentID, true).WithSkillStateService(h.skillStates)
	traceSink := newPlannerTraceSink(histSink, h.skillAudit, tenantUUID, traceID)
	_, plan, err := runtime.NewEngine().RunPlanInvoke(runCtx, msg, cfg, "", traceSink)
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

func (h *AgentChatHandler) latestRuntimePendingTask(
	ctx context.Context,
	env string,
	tenantRef *string,
	sessionID uint64,
	agentID uint64,
	boundSkillIDs []string,
) (map[string]any, bool, error) {
	if h != nil && h.skillStates != nil {
		if task, ok, err := h.skillStates.LatestPendingTask(ctx, env, tenantRef, sessionID, agentID, boundSkillIDs); err != nil {
			return nil, false, err
		} else if ok {
			return map[string]any(task), true, nil
		}
	}
	return nil, false, nil
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
