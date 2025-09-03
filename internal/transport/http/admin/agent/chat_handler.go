package agent

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/ArtisanCloud/PowerX/internal/server/agent"
	agentschema "github.com/ArtisanCloud/PowerX/internal/server/agent/schemas"
	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

type AgentChatHandler struct{}

func NewAgentChatHandler(_ *shared.Deps) *AgentChatHandler {
	return &AgentChatHandler{}
}

// 多 flow 串行的 streamCore
func (h *AgentChatHandler) streamCore(c *gin.Context, req dto.StreamChatRequest) {
	msg := strings.TrimSpace(req.Message)
	if msg == "" {
		dto.ResponseError(c, 400, "message 不能为空", nil)
		return
	}

	ctx := c.Request.Context()
	mgr := agent.GetAgentManager()

	// -------- 1) 生成计划 / 选择要执行的 flow --------
	var (
		plan   *flowschema.ExecutionPlan
		flowID = strings.TrimSpace(req.FlowID)
	)

	if flowID == "" {
		// 多任务识别
		tasks, err := mgr.DetectTasks(c, msg)
		if err != nil {
			c.SSEvent(dto.EventError, gin.H{"message": "意图识别失败", "detail": err.Error()})
			c.SSEvent(dto.EventEnd, gin.H{"ok": false})
			return
		}
		// 推送 intent（多任务）
		c.SSEvent(dto.EventIntent, gin.H{"mode": "intent_multi", "tasks": tasks})
		c.Writer.Flush()

		// 构建计划（可能返回强类型，也可能是 map）
		rawPlan := mgr.BuildPlan(tasks)

		switch v := any(rawPlan).(type) {
		case flowschema.ExecutionPlan:
			plan = &v
		case *flowschema.ExecutionPlan:
			plan = v
		default:
			// 回落：把任意结构转成 ExecutionPlan（若能成功）
			b, _ := json.Marshal(rawPlan)
			var tmp flowschema.ExecutionPlan
			if err := json.Unmarshal(b, &tmp); err == nil && len(tmp.Tasks) > 0 {
				plan = &tmp
			}
		}

		// 向前端回显计划（优先强类型）
		if plan != nil {
			c.SSEvent("plan", plan)
		} else {
			c.SSEvent("plan", rawPlan)
		}
		c.Writer.Flush()

		// 选定执行的 flow（当前阶段先取第一条；多 flow 同连要改 writer，下一步做）
		flowID = pickFirstFlowID(plan)
	}

	ag, fallbackFlowID, err := mgr.GetDefaultRoute()
	if err != nil {
		c.SSEvent(dto.EventError, gin.H{"message": "创建 Agent 失败", "detail": err.Error()})
		c.SSEvent(dto.EventEnd, gin.H{"ok": false})
		return
	}

	// 如果上面识别/计划没有挑出 flowID，使用默认兜底
	if strings.TrimSpace(flowID) == "" {
		flowID = strings.TrimSpace(fallbackFlowID)
		if flowID == "" {
			c.SSEvent(dto.EventError, gin.H{"code": "no_fallback_flow", "message": "未配置默认兜底 flow"})
			c.SSEvent(dto.EventEnd, gin.H{"ok": false})
			return
		}
	}

	params := flowschema.Context{
		"message": msg,
		"config":  req.Config,
		"context": req.Context,
		"plan":    plan, // 给节点使用
	}

	execID := fmt.Sprintf("chat_%d", time.Now().UnixNano())
	meta := agentschema.ExecutionMeta{
		RequestID: execID,
		UserID:    "user_123",
		TenantID:  "tenant_123",
		TraceID:   fmt.Sprintf("trace_%d", time.Now().UnixNano()),
		Priority:  1,
	}

	stream, err := ag.Stream(ctx, flowID, params, meta)
	if err != nil {
		c.SSEvent(dto.EventError, gin.H{"message": "流式聊天执行失败", "detail": err.Error()})
		c.SSEvent(dto.EventEnd, gin.H{"ok": false})
		return
	}

	// 统一由 WriteToSSE 写出（它会发 start/token/data/final/end/heartbeat）
	_ = dto.WriteToSSE(c, flowID, execID, stream, 25*time.Second)
}

// 取 ExecutionPlan 中“第一个要跑的 flow”
// - 先按 Stage 升序；同 Stage 按原顺序
// - 优先 FlowID，其次 TaskID
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

// 标准 SSE：GET /agents/stream/sse?q=...&flow_id=...&probe=1[&session_id=...]
func (h *AgentChatHandler) StreamSSE(c *gin.Context) {
	// 1) 探针：仅返回一个短链路的 SSE 确认
	probe := strings.EqualFold(c.Query("probe"), "1") || strings.EqualFold(c.Query("probe"), "true")
	if probe {
		c.Writer.Header().Set("Content-Type", "text/event-stream")
		c.Writer.Header().Set("Cache-Control", "no-cache")
		c.Writer.Header().Set("Connection", "keep-alive")
		c.Writer.Header().Set("X-Accel-Buffering", "no")
		c.SSEvent("ack", gin.H{"ok": true, "ts": time.Now().Unix(), "note": "sse probe only, no compute"})
		c.SSEvent("end", gin.H{"ok": true})
		return
	}

	// 2) 读取参数：q 为消息文本；flow_id 可选（传了就跳过意图识别由 streamCore 使用）
	//    为了容错，也兼容 message 参数
	q := strings.TrimSpace(c.Query("q"))
	if q == "" {
		q = strings.TrimSpace(c.Query("message"))
	}
	if q == "" {
		dto.ResponseError(c, 400, "缺少 q（消息内容）", nil)
		return
	}

	flowID := strings.TrimSpace(c.Query("flow_id"))
	sessionID := strings.TrimSpace(c.Query("session_id")) // 可选，前端已有会话就带上

	// 3) 组装请求；将 session_id 透传到 context，便于 streamCore/执行层关联会话（后续写库）
	ctxMap := map[string]any{}
	if sessionID != "" {
		ctxMap["session_id"] = sessionID
	}

	req := dto.StreamChatRequest{
		Message: q,
		FlowID:  flowID, // 为空则在 streamCore 内进行意图识别 & 选择 flow
		Config:  nil,    // 如需在 GET 上传更多选项，可以用 query 解析后塞这里
		Context: ctxMap, // 透传上下文（session_id 等）
		Route:   nil,    // 预留：以后如果要定制路由策略可在此扩展
		Exec:    nil,    // 预留：例如 dry-run/并发度/优先级等
	}

	// 4) 交给核心处理（设置 SSE 头、意图识别/plan/执行/事件写出 都在 streamCore 完成）
	h.streamCore(c, req)
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
	reply := "（LLM回复模拟）已收到：" + strings.TrimSpace(req.Message)
	dto.ResponseSuccess(c, dto.ChatData{
		Content:   reply,
		Role:      "assistant",
		Metadata:  map[string]any{"framework": "eino"},
		Timestamp: time.Now().Unix(),
	})
}
