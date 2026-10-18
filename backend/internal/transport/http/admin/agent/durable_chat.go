package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ArtisanCloud/PowerX/internal/service/agent_run"
	"strconv"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/runtime"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

func (h *AgentChatHandler) runtimeSnapshotLifetime() time.Duration {
	if h.durable != nil {
		return h.durable.Deadline + time.Minute
	}
	return 15 * time.Minute
}

func (h *AgentChatHandler) subscribeDurableRun(c *gin.Context, id string) {
	row, err := h.durable.AuthorizedRun(c.Request.Context(), id)
	if err != nil {
		dto.ResponseError(c, 403, "agent run unavailable", nil)
		return
	}
	// 按 Run 续订仍保留原会话归属，不能通过切换页面订阅另一工作区的结果。
	if c.Query("session_type") != "" || c.Query("team_id") != "" {
		tenant := row.TenantUUID.String()
		session, err := h.his.FindSessionByID(c.Request.Context(), row.Env, &tenant, row.SessionID)
		if err != nil || session == nil {
			dto.ResponseError(c, 404, "agent.session_not_found", nil)
			return
		}
		if !validateSessionQuery(c, session, false) {
			return
		}
	}
	raw := strings.TrimSpace(c.GetHeader("Last-Event-ID"))
	if raw == "" {
		raw = strings.TrimSpace(c.Query("after_seq"))
	}
	var cursor uint64
	if raw != "" {
		value, err := strconv.ParseUint(raw, 10, 64)
		if err != nil || strconv.FormatUint(value, 10) != raw {
			dto.ResponseError(c, 400, "invalid run event cursor", nil)
			return
		}
		cursor = value
	}
	if err := h.durable.ValidateSubscription(c.Request.Context(), id, cursor); err != nil {
		dto.ResponseError(c, 409, "run state or event cursor unavailable", nil)
		return
	}
	runtime.SetSSEHeaders(c)
	err = h.durable.Subscribe(c.Request.Context(), id, cursor, func(seq uint64, event string, payload any) error {
		raw, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		if seq > 0 {
			if _, err = fmt.Fprintf(c.Writer, "id: %d\n", seq); err != nil {
				return err
			}
		}
		if _, err = fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", event, raw); err != nil {
			return err
		}
		c.Writer.Flush()
		return nil
	})
	if err != nil && c.Request.Context().Err() == nil {
		// 订阅失败只关闭连接，客户端用相同 Run 和游标续订。
		c.SSEvent("agent_run.subscription_interrupted", gin.H{"run_id": id, "retryable": true})
		c.Writer.Flush()
	}
}

func archiveAdmissionError(err error) (string, string, int) {
	switch {
	case errors.Is(err, agent_run.ErrArchiveBackpressure):
		return agent_run.ErrArchiveBackpressure.Error(), "运行归档积压已达到容量阈值，请稍后重试。", 429
	case errors.Is(err, agent_run.ErrArchiveHealthStale):
		return agent_run.ErrArchiveHealthStale.Error(), "运行存储健康状态暂不可用，请稍后重试。", 503
	default:
		return "agent_run.admission_failed", "运行受理失败，请查看运行追踪。", 503
	}
}
