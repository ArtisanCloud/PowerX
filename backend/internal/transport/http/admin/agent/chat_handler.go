// file: internal/app/http/admin/agent/chat_handler.go
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/runtime"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/ArtisanCloud/PowerX/internal/server/agent"
	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	agentschema "github.com/ArtisanCloud/PowerX/internal/server/agent/schemas"
	agentSvc "github.com/ArtisanCloud/PowerX/internal/service/agent"
	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	dto "github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/utils"
	"github.com/gin-gonic/gin"
)

type AgentChatHandler struct {
	his *agentSvc.ChatHistoryService
}

func NewAgentChatHandler(dep *shared.Deps) *AgentChatHandler {
	return &AgentChatHandler{
		his: agentSvc.NewChatHistoryService(dep.DB),
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
	env := c.DefaultQuery("env", "default")
	q := strings.TrimSpace(utils.FirstNonEmpty(c.Query("q"), c.Query("message")))
	if q == "" {
		dto.ResponseError(c, 400, "缺少 q（消息内容）", nil)
		return
	}
	agentID, _ := utils.ParseUintID(strings.TrimSpace(c.Query("agent_id")))
	tenantCtx, err := requireTenantContext(c)
	if err != nil {
		dto.ResponseError(c, 400, err.Error(), nil)
		return
	}
	tenantRef := tenantCtx.UUIDPtr()
	uid := reqctx.GetUserID(c.Request.Context())

	// 会话：session_id 优先，否则 sticky
	var sess *dbmodel.AgentChatSession
	if sidStr := strings.TrimSpace(c.Query("session_id")); sidStr != "" {
		if sid, err := utils.ParseUintID(sidStr); err == nil && sid > 0 {
			sess, _ = h.his.FindSessionByID(c, env, tenantRef, sid)
		}
	}
	if sess == nil {
		var err error
		sess, err = h.his.GetOrCreateSession(c, env, tenantRef, agentID, uid, false, nil)
		if err != nil {
			dto.ResponseError(c, 500, "创建会话失败", err)
			return
		}
	}

	// 写入 user 消息
	_, _ = h.his.AppendMessage(c, env, tenantRef, sess.ID, agentID, "user", q, "text", 0, 0, false, nil)

	// 3) 适配器 + Engine
	runtime.SetSSEHeaders(c)
	baseSink := runtime.NewSSESink(c)
	histSink := runtime.NewHistorySink(baseSink, h.his, c, env, tenantRef, sess, agentID, true)

	cfg := &dto.ChatConfig{}                             // 如需从 query/form 取开关可补
	_ = runtime.NewEngine().Run(c, q, cfg, "", histSink) // explicitFlow 传空，交给意图/plan 选择
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
		},
	}
	_ = dto.WriteToSSEWithTap(c, flowID, execID, sr, hooks)
}

// 非流式（保留）
func (h *AgentChatHandler) Invoke(c *gin.Context) {
	var req dto.ChatRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	if req.Config != nil && req.Config.EnableStream {
		dto.ResponseError(c, 400, "该接口不支持流式，请改用 /agents/stream/sse 或 /agents/stream/ws", nil)
		return
	}
	reply := "（非流式）已收到：" + strings.TrimSpace(req.Message)
	dto.ResponseSuccess(c, dto.ChatData{
		Content:   reply,
		Role:      "assistant",
		Metadata:  map[string]any{"framework": "eino"},
		Timestamp: time.Now().Unix(),
	})
}

/* ---------------- helpers ---------------- */

func setSSEHeaders(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream; charset=utf-8")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
}
