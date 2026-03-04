package agent

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/runtime"
	agentSvc "github.com/ArtisanCloud/PowerX/internal/service/agent"
	capservice "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/utils"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

type AgentWSHandler struct {
	deps  *shared.Deps
	ag    *agentSvc.AgentService
	his   *agentSvc.ChatHistoryService
	audit *capservice.AuditService
}

func NewAgentWSHandler(deps *shared.Deps) *AgentWSHandler {
	return &AgentWSHandler{
		deps:  deps,
		ag:    agentSvc.NewAgentService(deps.DB),
		his:   agentSvc.NewChatHistoryService(deps.DB),
		audit: deps.CapabilityRegistryAudit,
	}
}

var upgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

func (h *AgentWSHandler) StreamWS(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	tenantCtx, err := requireTenantContext(c)
	if err != nil {
		_ = conn.WriteJSON(mustEnv(dto.WSError, dto.ErrorPayload{Code: "missing_tenant", Message: err.Error()}))
		_ = conn.WriteControl(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "missing tenant context"),
			time.Now().Add(2*time.Second))
		return
	}

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
	var activeEnv string
	var activeAgentID uint64
	var activeSessionID uint64

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
			activeEnv = strings.TrimSpace(p.Env)
			if activeEnv == "" {
				activeEnv = strings.TrimSpace(reqctx.GetEnv(c.Request.Context()))
			}
			if activeEnv == "" {
				activeEnv = "default"
			}
			if p.AgentID == 0 {
				_ = conn.WriteJSON(mustEnv(dto.WSError, dto.ErrorPayload{Code: "bad_request", Message: "agent_id 缺失"}))
				continue
			}
			if _, err := h.ag.Get(c.Request.Context(), activeEnv, tenantCtx.UUIDPtr(), p.AgentID); err != nil {
				_ = conn.WriteJSON(mustEnv(dto.WSError, dto.ErrorPayload{Code: "not_found", Message: "未找到指定的 Agent"}))
				continue
			}
			if p.SessionID > 0 {
				if _, err := h.his.FindSessionByID(c.Request.Context(), activeEnv, tenantCtx.UUIDPtr(), p.SessionID); err != nil {
					_ = conn.WriteJSON(mustEnv(dto.WSError, dto.ErrorPayload{Code: "not_found", Message: "未找到指定的 Session"}))
					continue
				}
			}
			activeAgentID = p.AgentID
			activeSessionID = p.SessionID
			// TODO: 这里接入 SessionService，创建/续期/返回会话信息
			_ = conn.WriteJSON(mustEnv(dto.WSAck, dto.AckPayload{OK: true, Message: "joined", SessionID: p.SessionID}))

		case dto.WSChatSend:
			var p dto.ChatSendPayload
			_ = dto.DecodePayload(env, &p)
			go h.handleChatSend(c, conn, p, tenantCtx, activeEnv, activeAgentID, activeSessionID)

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

func mustEnv(t string, payload any) dto.WSEnvelope {
	env, _ := dto.NewEnv(t, payload)
	return env
}

func (h *AgentWSHandler) handleChatSend(ginCtx *gin.Context, conn *websocket.Conn, p dto.ChatSendPayload, tenantCtx *tenantContext, defaultEnv string, defaultAgentID, defaultSessionID uint64) {
	msg := strings.TrimSpace(p.Message)
	if msg == "" {
		_ = conn.WriteJSON(mustEnv(dto.WSError, dto.ErrorPayload{Code: "bad_request", Message: "message 不能为空"}))
		return
	}
	ctx, cancel := context.WithTimeout(ginCtx.Request.Context(), 90*time.Second)
	defer cancel()

	if tenantCtx == nil || strings.TrimSpace(tenantCtx.UUID()) == "" {
		_ = conn.WriteJSON(mustEnv(dto.WSError, dto.ErrorPayload{Code: "missing_tenant", Message: "租户上下文缺失"}))
		return
	}
	env := strings.TrimSpace(defaultEnv)
	if p.Context != nil {
		if envStr := strings.TrimSpace(utils.ToStr(p.Context["env"])); envStr != "" {
			env = envStr
		}
	}
	if env == "" {
		env = strings.TrimSpace(reqctx.GetEnv(ginCtx.Request.Context()))
	}
	if env == "" {
		env = "default"
	}

	agentID := defaultAgentID
	if p.Context != nil {
		if v, ok := utils.AsUint64(p.Context["agent_id"]); ok && v > 0 {
			agentID = v
		}
	}
	if agentID == 0 {
		_ = conn.WriteJSON(mustEnv(dto.WSError, dto.ErrorPayload{Code: "bad_request", Message: "agent_id 缺失"}))
		return
	}
	if _, err := h.ag.Get(ctx, env, tenantCtx.UUIDPtr(), agentID); err != nil {
		_ = conn.WriteJSON(mustEnv(dto.WSError, dto.ErrorPayload{Code: "not_found", Message: "未找到指定的 Agent"}))
		return
	}

	sessionID := p.SessionID
	if sessionID == 0 {
		sessionID = defaultSessionID
	}
	if p.Context != nil {
		if v, ok := utils.AsUint64(p.Context["session_id"]); ok && v > 0 {
			sessionID = v
		}
	}
	if sessionID > 0 {
		if _, err := h.his.FindSessionByID(ctx, env, tenantCtx.UUIDPtr(), sessionID); err != nil {
			_ = conn.WriteJSON(mustEnv(dto.WSError, dto.ErrorPayload{Code: "not_found", Message: "未找到指定的 Session"}))
			return
		}
	}

	baseSink := runtime.NewWSSink(conn)
	startedAt := time.Now()
	traceID := strings.TrimSpace(reqctx.GetTraceID(ginCtx.Request.Context()))
	if traceID == "" {
		traceID = uuid.NewString()
	}
	err := runtime.NewEngine().Run(ctx, msg, p.Config, "", baseSink)
	status := "completed"
	if err != nil {
		status = "failed"
	}
	h.recordAgentInvocation(ctx, agentStreamCapability, tenantCtx.UUID(), agentID, sessionID, msg, traceID, "ws", status, err, time.Since(startedAt))
}

func (h *AgentWSHandler) recordAgentInvocation(ctx context.Context, capabilityID, tenantUUID string, agentID, sessionID uint64, message, traceID, protocol, status string, err error, latency time.Duration) {
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

const (
	agentStreamCapability = "com.corex.agent.stream"
	platformPluginID      = "corex.platform"
)
