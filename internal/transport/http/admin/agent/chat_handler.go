package agent

import (
	"context"
	"fmt"
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

// ---- 核心：被 POST/GET 共用 ----
func (h *AgentChatHandler) streamCore(c *gin.Context, req dto.StreamChatRequest) {
	msg := strings.TrimSpace(req.Message)
	if msg == "" {
		dto.ResponseError(c, 400, "message 不能为空", nil)
		return
	}

	ctx := c.Request.Context()
	timeout := time.Duration(60) * time.Second
	if dl, ok := ctx.Deadline(); !ok || time.Until(dl) > timeout {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	// 识别意图：先发一帧 intent（SSE）
	mgr := agent.GetAgentManager()
	intent, _ := mgr.DetectIntent(c, msg)

	// SSE 头
	c.Header("Content-Type", "text/event-stream; charset=utf-8")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	if intent != nil {
		c.SSEvent(dto.EventIntent, intent)
		c.Writer.Flush()
	}

	// flow 选择
	flowID := strings.TrimSpace(req.FlowID)
	if intent != nil && intent.Matched && intent.FlowID != "" {
		flowID = intent.FlowID
	}
	if flowID == "" {
		flowID = "default_chat_flow"
	}

	// 路由 & 执行
	ag, _, err := mgr.GetDefaultRoute()
	if err != nil {
		dto.ResponseError(c, 500, "创建 Agent 失败", err)
		return
	}
	params := flowschema.Context{
		"message": msg,
		"config":  req.Config,
		"context": req.Context,
	}
	execID := fmt.Sprintf("chat_%d", time.Now().UnixNano())
	meta := agentschema.ExecutionMeta{
		RequestID: execID,
		UserID:    "user_123",
		TenantID:  "tenant_123",
		TraceID:   fmt.Sprintf("trace_%d", time.Now().UnixNano()),
		Priority:  1,
		Timeout:   timeout / time.Second,
	}

	stream, err := ag.Stream(ctx, flowID, params, meta)
	if err != nil {
		dto.ResponseError(c, 500, "流式聊天执行失败", err)
		return
	}

	// 交给统一的 SSE 写出
	_ = dto.WriteToSSE(c, flowID, execID, stream, 25*time.Second)
}

// 兼容老路由：POST /agents/stream
func (h *AgentChatHandler) StreamChat(c *gin.Context) {
	var req dto.StreamChatRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	h.streamCore(c, req)
}

// 标准 SSE：GET /agents/stream/sse?q=...&flow_id=...
func (h *AgentChatHandler) StreamSSE(c *gin.Context) {
	q := strings.TrimSpace(c.Query("q"))
	if q == "" {
		dto.ResponseError(c, 400, "缺少 q（消息内容）", nil)
		return
	}
	req := dto.StreamChatRequest{
		Message: q,
		FlowID:  strings.TrimSpace(c.Query("flow_id")),
	}
	h.streamCore(c, req)
}

// 非流式（保留）
func (h *AgentChatHandler) Chat(c *gin.Context) {
	var req dto.ChatRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	if req.Config != nil && req.Config.EnableStream {
		dto.ResponseError(c, 400, "该接口不支持流式，请改用 /agents/stream", nil)
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
