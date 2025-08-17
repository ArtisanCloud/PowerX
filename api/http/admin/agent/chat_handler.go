package agent

import (
	"context"
	"fmt"
	"github.com/ArtisanCloud/PowerX/api/http/admin/dto"
	"github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/ArtisanCloud/PowerX/services/agent"
	"github.com/ArtisanCloud/PowerX/services/agent/config"
	"github.com/ArtisanCloud/PowerX/services/agent/drivers/eino"
	agentschema "github.com/ArtisanCloud/PowerX/services/agent/schemas"
	"time"

	"github.com/gin-gonic/gin"
)

// ChatHandler 基本聊天接口（非流式）
// 命中任务 → 多任务编排与执行；否则 → 普通对话回复。
func ChatHandler(c *gin.Context) {
	//var req dto.ChatRequest
	//if err := dto.ValidateRequestWithContext(c, &req); err != nil {
	//	dto.ResponseValidationError(c, err)
	//	return
	//}
	//if req.Config != nil && req.Config.EnableStream {
	//	dto.ResponseError(c, 400, "该接口不支持流式，请改用 /api/agents/stream", nil)
	//	return
	//}
	//msg := strings.TrimSpace(req.Message)
	//if msg == "" {
	//	dto.ResponseError(c, 400, "message 不能为空", nil)
	//	return
	//}
	//
	//ctx := c.Request.Context() // ✅ 用 context.Context
	//mgr := agent.GetAgentManager()
	//
	//tasks, _ := mgr.DetectTasks(ctx, msg) // ✅ 传 ctx
	//
	//if len(tasks) == 0 {
	//	// —— A. 无意图：兜底走默认聊天 Flow —— //
	//	out, intent, err := mgr.Dispatch(ctx, msg, flowschema.Context{
	//		"message":      msg,
	//		"model_config": req.Config, // 可选：传给兜底 flow
	//	}, agentschema.ExecutionMeta{
	//		RequestID: fmt.Sprintf("req_%d", time.Now().UnixNano()),
	//		Timeout:   30 * time.Second,
	//		Metadata:  map[string]any{"mode": "chat_fallback"},
	//	})
	//	if err != nil {
	//		dto.ResponseError(c, 500, "聊天失败", err)
	//		return
	//	}
	//
	//	reply := fmt.Sprintf("（LLM回复模拟）已收到：%s", msg)
	//	if out != nil && out.Data != nil { // ✅ 用到 out，避免“已声明未使用”
	//		if v, ok := out.Data["content"].(string); ok && v != "" {
	//			reply = v
	//		}
	//	}
	//
	//	dto.ResponseSuccess(c, dto.ChatData{
	//		Content:   reply,
	//		Role:      "assistant",
	//		Metadata:  map[string]any{"framework": "eino", "intent": intent, "mode": "chat_fallback"},
	//		Timestamp: time.Now().Unix(),
	//	})
	//	return
	//}
	//
	//// —— B. 有意图：多任务 → 依赖补全 → 计划 → 执行 —— //
	//tasks = mgr.ExpandWithPrereqs(tasks)
	//plan := mgr.BuildPlan(tasks)
	//
	//out, err := mgr.ExecutePlan(ctx, plan, agentschema.ExecutionMeta{
	//	RequestID: fmt.Sprintf("plan_%d", time.Now().UnixNano()),
	//	Timeout:   60 * time.Second,
	//	Metadata:  map[string]any{"mode": "task_execute"},
	//})
	//if err != nil {
	//	dto.ResponseError(c, 500, "任务执行失败", err)
	//	return
	//}
	//
	//dto.ResponseSuccess(c, gin.H{
	//	"mode":   "task_execute",
	//	"input":  msg,
	//	"plan":   plan,
	//	"result": out,
	//	"debug":  gin.H{"task_count": len(tasks)},
	//})
}

// StreamChatHandler 流式聊天接口（SSE）
// 流程：识别意图 -> 先发 intent 帧 -> 选择 flow -> 开始流式执行 -> 逐帧输出
func StreamChatHandler(c *gin.Context) {
	var req dto.StreamChatRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}

	// 用客户端上下文，确保断开连接能及时取消
	ctx := c.Request.Context()
	// 可选：如需超时，在客户端上下文上再包一层
	timeout := 60 * time.Second
	if dl, ok := ctx.Deadline(); !ok || time.Until(dl) > timeout {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	// 1) 识别意图（非阻塞、尽早反馈给前端）
	mgr := agent.GetAgentManager()
	intent, _ := mgr.DetectIntent(c, req.Message)
	// 先把 intent 推给前端（如果有）
	if intent != nil {
		c.Header("Content-Type", "text/event-stream; charset=utf-8")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("X-Accel-Buffering", "no") // 若经由 Nginx
		c.SSEvent("intent", intent)
		c.Writer.Flush()
	}

	// 2) 选择 flow（优先意图命中，其次请求显式 FlowID，最后降级默认）
	flowID := req.FlowID
	if intent != nil && intent.Matched && intent.FlowID != "" {
		flowID = intent.FlowID
	}
	if flowID == "" {
		flowID = "default_chat_flow"
	}

	// 3) 创建 agent（后续你可以从 Manager 中拿已注册的 agent 实例）
	agentCfg := &config.AgentConfig{
		// TODO: 从 req.Config 映射模型/温度/endpoint 等（按你项目约定）
	}
	ag, err := eino.NewAgent(agentCfg)
	if err != nil {
		dto.ResponseError(c, 500, "创建 Agent 失败", err)
		return
	}

	// 4) 组装 flow 参数/元信息
	params := schemas.Context{
		"message": req.Message,
		"config":  req.Config,
		"context": req.Context,
	}
	meta := agentschema.ExecutionMeta{
		RequestID: fmt.Sprintf("chat_%d", time.Now().UnixNano()),
		UserID:    "user_123",   // TODO: 从认证中取
		TenantID:  "tenant_123", // TODO: 从认证中取
		TraceID:   fmt.Sprintf("trace_%d", time.Now().UnixNano()),
		Priority:  1,
		Timeout:   timeout / time.Second,
	}

	// 5) 开始流式执行
	reader, err := ag.Stream(ctx, flowID, params, meta)
	if err != nil {
		dto.ResponseError(c, 500, "流式聊天执行失败", err)
		return
	}

	// 6) 统一 SSE 写出（带心跳）
	_ = dto.WriteToSSE(c, flowID, meta.RequestID, reader, 25*time.Second)
}
