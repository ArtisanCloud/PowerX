package agent

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/ArtisanCloud/PowerX/internal/server/agent"
	agentschema "github.com/ArtisanCloud/PowerX/internal/server/agent/schemas"
	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type AgentWSHandler struct{}

func NewAgentWSHandler(_ *shared.Deps) *AgentWSHandler { return &AgentWSHandler{} }

var upgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

func (h *AgentWSHandler) StreamWS(c *gin.Context) {
	q := strings.TrimSpace(c.Query("q"))
	if q == "" {
		dto.ResponseError(c, 400, "缺少 q（消息内容）", nil)
		return
	}
	flowID := strings.TrimSpace(c.Query("flow_id"))

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	// ctx/timeout
	ctx := c.Request.Context()
	timeout := 60 * time.Second
	if dl, ok := ctx.Deadline(); !ok || time.Until(dl) > timeout {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	// intent 先发一帧
	mgr := agent.GetAgentManager()
	intent, _ := mgr.DetectIntent(c, q)
	if intent != nil {
		_ = conn.WriteJSON(dto.WSMessage{Type: dto.EventIntent, Data: map[string]any{"intent": intent}, Timestamp: time.Now().Unix()})
	}
	if intent != nil && intent.Matched && intent.FlowID != "" {
		flowID = intent.FlowID
	}
	if flowID == "" {
		flowID = "default_chat_flow"
	}

	// 路由 + 执行
	ag, _, err := mgr.GetDefaultRoute()
	if err != nil {
		_ = conn.WriteJSON(dto.WSMessage{Type: dto.EventError, Data: map[string]any{"message": "创建 Agent 失败"}, Timestamp: time.Now().Unix()})
		return
	}
	params := flowschema.Context{"message": q}
	execID := fmt.Sprintf("ws_%d", time.Now().UnixNano())
	meta := agentschema.ExecutionMeta{RequestID: execID, Timeout: timeout / time.Second}

	reader, err := ag.Stream(ctx, flowID, params, meta)
	if err != nil {
		_ = conn.WriteJSON(dto.WSMessage{Type: dto.EventError, Data: map[string]any{"message": "流式执行失败"}, Timestamp: time.Now().Unix()})
		return
	}

	// 交给统一 WS 写出
	_ = dto.WriteToWS(ctx, conn, flowID, execID, reader, 25*time.Second)
}
