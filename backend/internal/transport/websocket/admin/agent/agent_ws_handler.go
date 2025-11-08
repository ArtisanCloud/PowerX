package agent

import (
	"context"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/runtime"
	"net/http"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type AgentWSHandler struct {
	deps *shared.Deps
}

func NewAgentWSHandler(deps *shared.Deps) *AgentWSHandler { return &AgentWSHandler{deps: deps} }

var upgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

func (h *AgentWSHandler) StreamWS(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	// welcome
	_ = conn.WriteJSON(mustEnv(dto.WSWelcome, dto.WelcomePayload{
		Protocol:     dto.ProtocolVersion,
		Server:       "powerx-agent",
		HeartbeatSec: 25,
	}))

	// probe（可选）
	if strings.EqualFold(c.Query("probe"), "1") || strings.EqualFold(c.Query("probe"), "true") {
		_ = conn.WriteJSON(mustEnv(dto.WSAck, dto.AckPayload{OK: true, Message: "ws probe ok"}))
		_ = conn.WriteControl(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, "probe done"),
			time.Now().Add(2*time.Second))
		return
	}

	// 读循环
	for {
		var env dto.WSEnvelope
		if err := conn.ReadJSON(&env); err != nil {
			return
		}
		switch env.Type {
		case dto.WSHello:
			var p dto.HelloPayload
			_ = dto.DecodePayload(env, &p)
			_ = conn.WriteJSON(mustEnv(dto.WSAck, dto.AckPayload{OK: true, Message: "hello ok"}))

		case dto.WSJoinSession:
			var p dto.JoinSessionPayload
			_ = dto.DecodePayload(env, &p)
			// TODO: 这里接入 SessionService，创建/续期/返回会话信息
			_ = conn.WriteJSON(mustEnv(dto.WSAck, dto.AckPayload{OK: true, Message: "joined", SessionID: p.SessionID}))

		case dto.WSChatSend:
			var p dto.ChatSendPayload
			_ = dto.DecodePayload(env, &p)
			go h.handleChatSend(c, conn, p)

		case dto.WSPing:
			var p dto.PingPayload
			_ = dto.DecodePayload(env, &p)
			_ = conn.WriteJSON(mustEnv(dto.WSPong, dto.PongPayload{Seq: p.Seq}))

		case dto.WSCancel:
			// TODO: 按 executionID 取消（保存 execID->cancelFn 即可）
			_ = conn.WriteJSON(mustEnv(dto.WSAck, dto.AckPayload{OK: true, Message: "cancel not-implemented"}))

		default:
			_ = conn.WriteJSON(mustEnv(dto.WSError, dto.ErrorPayload{Code: "unsupported_type", Message: env.Type}))
		}
	}
}

func (h *AgentWSHandler) handleChatSend(ginCtx *gin.Context, conn *websocket.Conn, p dto.ChatSendPayload) {
	msg := strings.TrimSpace(p.Message)
	if msg == "" {
		_ = conn.WriteJSON(mustEnv(dto.WSError, dto.ErrorPayload{Code: "bad_request", Message: "message 不能为空"}))
		return
	}
	ctx, cancel := context.WithTimeout(ginCtx.Request.Context(), 90*time.Second)
	defer cancel()

	// 可选：从 payload 中获取/创建 session，写入 user 消息
	// 这里演示简化版（如你需要持久化，也可注入 ChatHistoryService 与 SSE 一致）
	baseSink := runtime.NewWSSink(conn)
	// 如果也需要写历史：histSink := runtime.NewHistorySink(baseSink, h.his, ginCtx, env, &tid, sess, agentID, true)

	// 告知 start 事件交由 engine 统一发，你也可以先发个 ack
	_ = runtime.NewEngine().Run(ctx, msg, p.Config, "", baseSink)
}

func mustEnv(t string, payload any) dto.WSEnvelope {
	env, _ := dto.NewEnv(t, payload)
	return env
}
