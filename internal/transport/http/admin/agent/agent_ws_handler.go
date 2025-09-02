package agent

import (
	"context"
	"fmt"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/contract"
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
	var chosen string
	key := http.CanonicalHeaderKey(contract.HeaderKeySecWebSocketProtocol)
	if vals, ok := c.Request.Header[key]; ok && len(vals) > 0 {
		for _, v := range vals {
			for _, p := range strings.Split(v, ",") {
				p = strings.TrimSpace(p)
				if strings.HasPrefix(strings.ToLower(p), "bearer.") {
					chosen = p
					break
				}
			}
			if chosen != "" {
				break
			}
		}
	}

	// === 升级时带上回显的协议 ===
	respHeader := http.Header{}
	if chosen != "" {
		respHeader.Set(contract.HeaderKeySecWebSocketProtocol, chosen)
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, respHeader)
	if err != nil {
		return
	}
	defer conn.Close()

	// 下面保持你的 probe/q 流程不变……
	probe := strings.EqualFold(c.Query("probe"), "1") || strings.EqualFold(c.Query("probe"), "true")
	if probe {
		_ = conn.WriteJSON(map[string]any{
			"type": "ack",
			"data": map[string]any{"ok": true, "ts": time.Now().Unix(), "note": "ws probe only, no compute"},
		})
		_ = conn.WriteControl(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, "probe done"),
			time.Now().Add(2*time.Second))
		return
	}

	q := strings.TrimSpace(c.Query("q"))
	if q == "" {
		_ = conn.WriteJSON(dto.WSMessage{Type: dto.EventError, Data: map[string]any{"message": "缺少 q（消息内容）"}, Timestamp: time.Now().Unix()})
		return
	}
	flowID := strings.TrimSpace(c.Query("flow_id"))

	// ctx/timeout
	ctx := c.Request.Context()
	timeout := 60 * time.Second
	if dl, ok := ctx.Deadline(); !ok || time.Until(dl) > timeout {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	// 意图帧
	mgr := agent.GetAgentManager()
	intent, _ := mgr.DetectIntent(c, q)
	if intent != nil {
		_ = conn.WriteJSON(dto.WSMessage{Type: dto.EventIntent, Data: map[string]any{"intent": intent}, Timestamp: time.Now().Unix()})
	}
	if intent != nil && intent.Matched && intent.FlowID != "" {
		flowID = intent.FlowID
	}
	if flowID == "" {
		flowID = "chat" // ← 用你真实存在的 flow
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

	// 统一 WS 写出
	_ = dto.WriteToWS(ctx, conn, flowID, execID, reader, 25*time.Second)
}
