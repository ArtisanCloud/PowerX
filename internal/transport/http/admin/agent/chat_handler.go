// file: internal/app/http/admin/agent/chat_handler.go
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
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

// 标准 SSE：GET /api/agents/stream/sse?q=...&flow_id=...&probe=1[&session_id=...&agent_id=...&env=...]
func (h *AgentChatHandler) StreamSSE(c *gin.Context) {
	// 探针
	probe := strings.EqualFold(c.Query("probe"), "1") || strings.EqualFold(c.Query("probe"), "true")
	if probe {
		setSSEHeaders(c)
		c.SSEvent("ack", gin.H{"ok": true, "ts": time.Now().Unix(), "note": "sse probe only, no compute"})
		c.SSEvent("end", gin.H{"ok": true})
		return
	}

	env := c.DefaultQuery("env", "default")
	q := strings.TrimSpace(firstNonEmpty(c.Query("q"), c.Query("message")))
	if q == "" {
		dto.ResponseError(c, 400, "缺少 q（消息内容）", nil)
		return
	}
	flowID := strings.TrimSpace(c.Query("flow_id"))
	agentIDStr := strings.TrimSpace(c.Query("agent_id"))
	sessionIDStr := strings.TrimSpace(c.Query("session_id"))

	agentID, _ := utils.ParseUintID(agentIDStr)
	_, _ = utils.ParseUintID(sessionIDStr) // 允许传但不强求（我们内部会 GetOrCreate）

	tid, _ := reqctx.RequireTenantIDFromGin(c)
	uid := reqctx.GetUserID(c.Request.Context())

	// 先切到 SSE 头
	setSSEHeaders(c)

	ctxMap := map[string]any{
		"env":       env,
		"tenant_id": tid,
		"agent_id":  agentID,
		"user_id":   uid,
	}
	if sessionIDStr != "" {
		ctxMap["session_id"] = sessionIDStr
	}

	req := dto.StreamChatRequest{
		Message: q,
		FlowID:  flowID,
		Config:  nil,
		Context: ctxMap,
		Route:   nil,
		Exec:    nil,
	}

	h.streamCore(c, req)
}

// ---- 核心：单 Flow 流式 + 会话入库闭环 ----
func (h *AgentChatHandler) streamCore(c *gin.Context, req dto.StreamChatRequest) {
	msg := strings.TrimSpace(req.Message)
	if msg == "" {
		dto.ResponseError(c, 400, "message 不能为空", nil)
		return
	}
	ctx := c.Request.Context()

	// 上下文解析
	env := "default"
	if v, ok := req.Context["env"].(string); ok && v != "" {
		env = v
	}
	var tid uint64
	tid = req.Context["tenant_id"].(uint64)

	var agentID uint64
	agentID = req.Context["agent_id"].(uint64)

	userID, _ := req.Context["user_id"].(uint64)

	// 会话：取或建（不再依赖 MakeDefaultSessionDef）
	var sess *dbmodel.AgentChatSession
	// 若传了 session_id，优先按该 id 取；否则按 (env, tenant, agent, user) sticky 取/建
	if sidStr, ok := req.Context["session_id"].(string); ok && strings.TrimSpace(sidStr) != "" {
		if sid, err := utils.ParseUintID(sidStr); err == nil && sid > 0 {
			// 允许你在 Service 里实现 GetByIDOrCreateFallback；这里先复用通用入口
			sess, _ = h.his.FindSessionByID(ctx, env, &tid, sid)
		}
	}
	if sess == nil {
		// def 传 nil，默认标题由 Service 内部根据首条消息自动生成即可
		var err error
		sess, err = h.his.GetOrCreateSession(ctx, env, &tid, agentID, userID, false, nil)
		if err != nil {
			c.SSEvent(dto.EventError, gin.H{"message": "创建会话失败", "detail": err.Error()})
			c.SSEvent(dto.EventEnd, gin.H{"ok": false})
			return
		}
	}

	// 先写入 user 消息
	if _, err := h.his.AppendMessage(ctx, env, &tid, sess.ID, agentID, "user", msg, "text", 0, 0, false, nil); err != nil {
		c.SSEvent(dto.EventError, gin.H{"message": "写入用户消息失败", "detail": err.Error()})
		c.SSEvent(dto.EventEnd, gin.H{"ok": false})
		return
	}

	// 意图 → 计划（未显式 flow_id 时）
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

		rawPlan := mgr.BuildPlan(tasks)
		switch v := any(rawPlan).(type) {
		case flowschema.ExecutionPlan:
			plan = &v
		case *flowschema.ExecutionPlan:
			plan = v
		default:
			b, _ := json.Marshal(rawPlan)
			var tmp flowschema.ExecutionPlan
			if err := json.Unmarshal(b, &tmp); err == nil && len(tmp.Tasks) > 0 {
				plan = &tmp
			}
		}
		if plan != nil {
			c.SSEvent("plan", plan)
		} else {
			c.SSEvent("plan", rawPlan)
		}
		c.Writer.Flush()

		flowID = pickFirstFlowID(plan)
	}

	// 路由 & 兜底 flow
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

	// 执行单 flow
	params := flowschema.Context{
		"message": msg,
		"context": map[string]any{
			"env":        env,
			"tenant_id":  tid,
			"agent_id":   agentID,
			"session_id": sess.ID,
			"user_id":    userID,
		},
		"plan": plan,
	}
	execID := fmt.Sprintf("sess_%d_%d", sess.ID, time.Now().UnixNano())
	meta := agentschema.ExecutionMeta{
		RequestID: execID,
		UserID:    userID,
		TenantID:  tid,
		TraceID:   fmt.Sprintf("trace_%d", time.Now().UnixNano()),
		Priority:  1,
		Timeout:   60,
	}

	runCtx := withTimeout(ctx, 60*time.Second)
	sr, err := ag.Stream(runCtx, flowID, params, meta)
	if err != nil {
		c.SSEvent(dto.EventError, gin.H{"message": "流式聊天执行失败", "detail": err.Error()})
		c.SSEvent(dto.EventEnd, gin.H{"ok": false})
		return
	}

	// Tap：增量聚合 / 最终入库 / 开始前发 meta（带 session_id）
	var buf strings.Builder
	hooks := dto.SSEHooks{
		HeartbeatInterval: 25 * time.Second,
		OnStart: func(fid, eid string) {
			// 在 start 之前补一帧 meta 让前端拿到 session_id
			c.SSEvent("meta", gin.H{"session_id": sess.ID, "agent_id": agentID})
			c.Writer.Flush()
		},
		OnDelta: func(delta string, _ *agentschema.ExecutionResult) {
			buf.WriteString(delta)
		},
		OnFinal: func(final *agentschema.ExecutionResult) {
			text := extractAssistantText(final)
			if strings.TrimSpace(text) == "" {
				text = buf.String()
			}
			if strings.TrimSpace(text) != "" {
				_, _ = h.his.AppendMessage(c.Request.Context(), env, &tid, sess.ID, agentID, "assistant", text, "text", 0, 0, false, nil)
			}
			_, _ = h.his.SummarizeIfNeeded(c.Request.Context(), env, &tid, sess)
		},
		OnError: func(err error) {
			// 可加打点
		},
	}

	_ = dto.WriteToSSEWithTap(c, flowID, execID, sr, hooks)
}

// 非流式（简版）
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

func withTimeout(ctx context.Context, d time.Duration) context.Context {
	if dl, ok := ctx.Deadline(); !ok || time.Until(dl) > d {
		nctx, _ := context.WithTimeout(ctx, d)
		return nctx
	}
	return ctx
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

func pickFirstFlowID(plan *flowschema.ExecutionPlan) string {
	if plan == nil || len(plan.Tasks) == 0 {
		return ""
	}
	tasks := make([]flowschema.PlanTask, len(plan.Tasks))
	copy(tasks, plan.Tasks)
	sort.SliceStable(tasks, func(i, j int) bool {
		if tasks[i].Stage == tasks[j].Stage {
			return i < j
		}
		return tasks[i].Stage < tasks[j].Stage
	})
	if id := strings.TrimSpace(tasks[0].FlowID); id != "" {
		return id
	}
	if id := strings.TrimSpace(tasks[0].TaskID); id != "" {
		return id
	}
	return ""
}

func extractAssistantText(chunk *agentschema.ExecutionResult) string {
	if chunk == nil || chunk.Data == nil {
		return ""
	}
	if res, ok := chunk.Data["result"].(map[string]any); ok {
		if s, ok := res["content"].(string); ok {
			return s
		}
	}
	if s, ok := chunk.Data["content"].(string); ok {
		return s
	}
	return ""
}
