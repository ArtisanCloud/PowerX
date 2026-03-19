// file: internal/app/http/admin/agent/chat_handler.go
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/ArtisanCloud/PowerX/internal/server/agent"
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
	return s.buf.String()
}

type plannerTraceSink struct {
	next       runtime.EventSink
	audit      *skillservice.AuditTraceService
	tenantUUID string
	traceID    string
	planID     string
	mu         sync.Mutex
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
		s.planID = strings.TrimSpace(readStringAny(m["plan_id"]))
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
	flowID := strings.TrimSpace(readStringAny(m["flow_id"]))
	taskID := strings.TrimSpace(readStringAny(m["task_id"]))
	if flowID == "" || taskID == "" {
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
		SkillID:      flowID,
		Version:      "",
		Entrypoint:   taskID,
		InvokePath:   "agent.invoke.plan",
		ProtocolUsed: "agent",
		Status:       status,
		PlanID:       planID,
		NodeID:       taskID,
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

func extractInvokeAssistantText(payload any) string {
	switch m := payload.(type) {
	case map[string]any:
		if d, ok := m["data"].(map[string]any); ok {
			if s, ok := d["content"].(string); ok {
				return s
			}
		}
		if s, ok := m["content"].(string); ok {
			return s
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
	baseSink := runtime.NewSSESink(c)
	histSink := runtime.NewHistorySink(baseSink, h.his, c, env, tenantRef, sess, agentID, true)
	startedAt := time.Now()
	traceID := strings.TrimSpace(reqctx.GetTraceID(c.Request.Context()))
	if traceID == "" {
		traceID = uuid.NewString()
	}

	// 支持“从某条 user 消息重新生成”：裁剪后续消息并以该消息内容作为 prompt
	if regenFromID > 0 {
		msgRec, err := h.his.FindMessageByID(c, env, tenantRef, regenFromID)
		if err != nil || msgRec == nil || msgRec.SessionID != sess.ID || strings.ToLower(strings.TrimSpace(msgRec.Role)) != "user" {
			_ = histSink.Emit(dto.EventError, map[string]any{"message": "regen_from_message_id 无效或不属于该会话"})
			_ = histSink.Emit(dto.EventEnd, map[string]any{"success": false})
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
		_ = histSink.Emit(dto.EventMeta, map[string]any{
			"session_id":              sess.ID,
			"agent_id":                agentID,
			"regen_from_message_id":   regenFromID,
			"regen_from_message_role": "user",
		})
	} else {
		// 正常发送：先写入 user 消息，并把 DB message_id 回传给前端用于“从此问题重新生成”
		clientMsgID := strings.TrimSpace(c.Query("client_msg_id"))
		userMsg, _ := h.his.AppendMessage(c, env, tenantRef, sess.ID, agentID, "user", q, "text", 0, 0, false, nil)
		if userMsg != nil {
			_ = histSink.Emit(dto.EventMeta, map[string]any{
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
		_ = histSink.Emit(dto.EventError, map[string]any{"message": cfgErr.Error()})
		_ = histSink.Emit(dto.EventEnd, map[string]any{"success": false})
		return
	}
	// 让前端/排障能看到“实际执行用的 provider/model”，避免把模型自报当成事实。
	_ = histSink.Emit(dto.EventMeta, map[string]any{
		"env":          env,
		"llm_provider": strings.TrimSpace(cfg.Provider),
		"llm_model":    strings.TrimSpace(cfg.ModelName),
	})

	err = runtime.NewEngine().Run(c.Request.Context(), q, cfg, "", histSink) // explicitFlow 传空，交给意图/plan 选择
	status := "completed"
	if err != nil {
		status = "failed"
	}
	h.recordAgentInvocation(c.Request.Context(), agentStreamCapability, tenantCtx.UUID(), agentID, sess.ID, q, traceID, "rest", status, err, time.Since(startedAt))
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
		h.recordAgentInvocation(ctx, agentStreamCapability, tenantUUID, agentID, sess.ID, msg, traceID, "rest", status, err, time.Since(startedAt))
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
				text = buf.String()
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
	h.recordAgentInvocation(c.Request.Context(), agentInvokeCapability, tenantUUID, agentID, sess.ID, msg, traceID, "rest", status, err, time.Since(startedAt))
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

func (h *AgentChatHandler) recordAgentInvocation(ctx context.Context, capabilityID, tenantUUID string, agentID, sessionID uint64, message, traceID, protocol, status string, err error, latency time.Duration) {
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
