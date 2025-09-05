// internal/server/agent/runtime/sink_sse.go
package runtime

import (
	"github.com/gin-gonic/gin"
)

type SSESink struct{ c *gin.Context }

func NewSSESink(c *gin.Context) *SSESink { return &SSESink{c: c} }

func (s *SSESink) Emit(event string, payload any) error {
	s.c.SSEvent(event, payload)
	s.c.Writer.Flush()
	return nil
}

// 辅助：设定 SSE 头（可被 handler 复用）
func SetSSEHeaders(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream; charset=utf-8")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	// 可选：c.Writer.WriteString("retry: 2000\n\n")
}
