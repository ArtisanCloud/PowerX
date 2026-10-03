package agent

import (
	"encoding/json"
	"fmt"
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
	if _, err := h.durable.AuthorizedRun(c.Request.Context(), id); err != nil {
		dto.ResponseError(c, 403, "agent run unavailable", nil)
		return
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
	err := h.durable.Subscribe(c.Request.Context(), id, cursor, func(seq uint64, event string, payload any) error {
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
