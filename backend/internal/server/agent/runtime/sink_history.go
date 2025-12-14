// internal/server/agent/runtime/sink_history.go
package runtime

import (
	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	agentSvc "github.com/ArtisanCloud/PowerX/internal/service/agent"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"strings"
)

type HistorySink struct {
	next     EventSink
	his      *agentSvc.ChatHistoryService
	ginCtx   *gin.Context
	env      string
	tenantUUID *string
	session  *dbmodel.AgentChatSession
	agentID  uint64
	buf      strings.Builder
	enabled  bool // 允许某些通道不开启历史
}

func NewHistorySink(next EventSink, his *agentSvc.ChatHistoryService, ginCtx *gin.Context,
	env string, tenantUUID *string, session *dbmodel.AgentChatSession, agentID uint64, enabled bool,
) *HistorySink {
	return &HistorySink{next: next, his: his, ginCtx: ginCtx, env: env, tenantUUID: tenantUUID, session: session, agentID: agentID, enabled: enabled}
}

func (h *HistorySink) Emit(event string, payload any) error {
	// 先向下游发送（保证实时性）
	if err := h.next.Emit(event, payload); err != nil {
		return err
	}

	// 侧效：收集/写库（仅在开启时）
	if !h.enabled || h.session == nil {
		return nil
	}
	switch event {
	case dto.EventToken:
		// 统一处理几种可能字段
		switch p := payload.(type) {
		case map[string]any:
			if s, ok := p["delta"].(string); ok && s != "" {
				h.buf.WriteString(s)
			}
		}
	case dto.EventFinal:
		// final 时落库 assistant 文本
		text := extractAssistantText(payload)
		if strings.TrimSpace(text) == "" {
			text = h.buf.String()
		}
		if strings.TrimSpace(text) != "" {
			_, _ = h.his.AppendMessage(h.ginCtx.Request.Context(),
				h.env, h.tenantUUID, h.session.ID, h.agentID, "assistant", text, "text", 0, 0, false, nil)
			_, _ = h.his.SummarizeIfNeeded(h.ginCtx.Request.Context(), h.env, h.tenantUUID, h.session)
		}
	}
	return nil
}

func extractAssistantText(payload any) string {
	// 只处理常见结构：{ data: { content: "..." } } 或 { content: "..." }
	switch m := payload.(type) {
	case map[string]any:
		if d, ok := m["data"].(map[string]any); ok {
			if s, ok := d["content"].(string); ok {
				return s
			}
		}
		if s, ok := m["content"].(string); ok {
			return s
		}
	}
	return ""
}
