package agent

import (
	"context"
	"fmt"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
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

	mgr := agent.GetAgentManager()

	// 1) 意图识别 → 先告诉前端
	var flowID string
	if intent, _ := mgr.DetectIntent(ginCtx, msg); intent != nil {
		_ = conn.WriteJSON(mustEnv(dto.WSIntent, dto.IntentPayload{
			Matched:  intent.Matched,
			Strategy: intent.Strategy,
			Reason:   intent.Reason,
			Score:    intent.Score,
			AgentID:  intent.AgentID,
			FlowID:   intent.FlowID,
		}))

		if intent.Matched && strings.TrimSpace(intent.FlowID) != "" {
			flowID = strings.TrimSpace(intent.FlowID)
		}
	}

	// 2) 可选覆盖：从 Context 里读取 flow_id（仅用于调试/显式指定）
	if flowID == "" && p.Context != nil {
		if v, ok := p.Context["flow_id"].(string); ok && strings.TrimSpace(v) != "" {
			flowID = strings.TrimSpace(v)
		}
	}

	// 3) 路由 agent，并兜底
	ag, fallbackFlowID, err := mgr.GetDefaultRoute()
	if err != nil {
		_ = conn.WriteJSON(mustEnv(dto.WSError, dto.ErrorPayload{Code: "agent_not_found", Message: err.Error()}))
		return
	}
	if flowID == "" {
		flowID = strings.TrimSpace(fallbackFlowID) // e.g. "base_flow"
		if flowID == "" {
			_ = conn.WriteJSON(mustEnv(dto.WSError, dto.ErrorPayload{Code: "no_fallback_flow", Message: "未配置默认兜底 flow"}))
			return
		}
	}

	// 4) 组织入参
	params := flowschema.Context{"message": msg}
	if p.Config != nil {
		params["config"] = p.Config
	}
	if p.Context != nil {
		params["context"] = p.Context
	}

	execID := fmt.Sprintf("exec_%d", time.Now().UTC().UnixNano())
	userId := reqctx.GetUserID(ctx)
	tenantId := reqctx.GetTenantID(ctx)
	meta := agentschema.ExecutionMeta{
		RequestID: execID,
		UserID:    userId,
		TenantID:  tenantId,
		TraceID:   fmt.Sprintf("trace_%d", time.Now().UTC().UnixNano()),
		Priority:  1,
		Timeout:   90,
	}

	// 5) 开流
	sr, err := ag.Stream(ctx, flowID, params, meta)
	if err != nil {
		_ = conn.WriteJSON(mustEnv(dto.WSError, dto.ErrorPayload{Code: "stream_error", Message: err.Error()}))
		return
	}

	// 6) start + 统一写出
	_ = conn.WriteJSON(mustEnv(dto.WSStart, dto.StartPayload{
		FlowID:      flowID,
		ExecutionID: execID,
		SessionID:   p.SessionID,
	}))
	_ = dto.WriteToWS(ctx, conn, flowID, execID, sr, 25*time.Second)
}

func mustEnv(t string, payload any) dto.WSEnvelope {
	env, _ := dto.NewEnv(t, payload)
	return env
}
