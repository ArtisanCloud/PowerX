// dto/sse.go
package dto

import (
	"errors"
	"io"
	"time"

	aschema "github.com/ArtisanCloud/PowerX/internal/server/agent/schemas"
	"github.com/cloudwego/eino/schema"
	"github.com/gin-gonic/gin"
)

// WriteToSSE：安全/健壮的 SSE 写入封装
func WriteToSSE(
	c *gin.Context,
	flowID string,
	execID string,
	sr *schema.StreamReader[*aschema.ExecutionResult],
	heartbeatInterval time.Duration, // <=0 不发心跳
) error {
	defer sr.Close()

	ctx := c.Request.Context()

	// 基础头
	c.Header("Content-Type", "text/event-stream; charset=utf-8")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("X-Accel-Buffering", "no") // 若走 Nginx，禁止缓冲

	send := func(event string, data any) {
		c.SSEvent(event, data)
		c.Writer.Flush()
	}

	// 开始帧
	send(EventStart, map[string]any{
		"flow_id":      flowID,
		"execution_id": execID,
		"message":      "开始执行流程",
	})

	// 心跳
	var hbTicker *time.Ticker
	if heartbeatInterval > 0 {
		hbTicker = time.NewTicker(heartbeatInterval)
		defer hbTicker.Stop()
	}

	finalSent := false

	// 异步 Recv
	type recvResult struct {
		chunk *aschema.ExecutionResult
		err   error
	}
	recvCh := make(chan recvResult, 1)
	go func() {
		defer close(recvCh)
		for {
			ch, err := sr.Recv()
			recvCh <- recvResult{chunk: ch, err: err}
			if err != nil {
				return
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			send(EventError, map[string]any{"success": false, "error": ctx.Err().Error()})
			send(EventEnd, map[string]any{"success": false, "message": "服务端取消"})
			return ctx.Err()

		case <-hbTickerC(hbTicker):
			send(EventHeartbeat, map[string]any{"ts": time.Now().Unix()})

		case rr, ok := <-recvCh:
			if !ok {
				return nil
			}
			if rr.err != nil {
				if errors.Is(rr.err, io.EOF) {
					if !finalSent {
						send(EventFinal, map[string]any{"success": true, "message": "流程执行完成"})
					}
					send(EventEnd, map[string]any{"success": true, "message": "连接结束"})
					return nil
				}
				send(EventError, map[string]any{"success": false, "error": rr.err.Error()})
				send(EventEnd, map[string]any{"success": false, "message": "流程执行失败"})
				return rr.err
			}

			chunk := rr.chunk

			// 增量文本
			if delta, ok := chunk.Metadata["delta_text"].(string); ok && delta != "" {
				send(EventToken, map[string]any{
					"delta":     delta,
					"step_id":   chunk.StepID,
					"timestamp": chunk.Timestamp,
				})
				continue
			}

			// 前端动作（可选）
			if act, ok := chunk.Metadata["action"].(map[string]any); ok && len(act) > 0 {
				send(EventAction, act)
			}

			// 中间数据
			send(EventData, map[string]any{
				"success":   chunk.Success,
				"data":      chunk.Data,
				"step_id":   chunk.StepID,
				"timestamp": chunk.Timestamp,
				"metadata":  chunk.Metadata,
			})

			// 最终帧
			if isFinal, _ := chunk.Metadata["is_final"].(bool); isFinal {
				send(EventFinal, map[string]any{
					"success":   chunk.Success,
					"data":      chunk.Data,
					"metadata":  chunk.Metadata,
					"timestamp": chunk.Timestamp,
				})
				finalSent = true
				send(EventEnd, map[string]any{"success": true, "message": "流程执行完成"})
				return nil
			}
		}
	}
}

func hbTickerC(t *time.Ticker) <-chan time.Time {
	if t == nil {
		return nil
	}
	return t.C
}

// 可选钩子
type SSEHooks struct {
	OnStart           func(flowID, execID string)
	OnDelta           func(delta string, chunk *aschema.ExecutionResult)
	OnData            func(chunk *aschema.ExecutionResult)
	OnFinal           func(final *aschema.ExecutionResult)
	OnError           func(err error)
	HeartbeatInterval time.Duration
}

// 带钩子的 SSE 写法（不破坏旧签名）
func WriteToSSEWithTap(
	c *gin.Context,
	flowID string,
	execID string,
	sr *schema.StreamReader[*aschema.ExecutionResult],
	h SSEHooks,
) error {
	defer sr.Close()

	ctx := c.Request.Context()

	c.Header("Content-Type", "text/event-stream; charset=utf-8")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("X-Accel-Buffering", "no")

	send := func(event string, data any) {
		c.SSEvent(event, data)
		c.Writer.Flush()
	}

	if h.OnStart != nil {
		h.OnStart(flowID, execID)
	}
	send(EventStart, map[string]any{
		"flow_id":      flowID,
		"execution_id": execID,
		"message":      "开始执行流程",
	})

	var hbTicker *time.Ticker
	if h.HeartbeatInterval > 0 {
		hbTicker = time.NewTicker(h.HeartbeatInterval)
		defer hbTicker.Stop()
	}

	finalSent := false

	type recvResult struct {
		chunk *aschema.ExecutionResult
		err   error
	}
	recvCh := make(chan recvResult, 1)
	go func() {
		defer close(recvCh)
		for {
			ch, err := sr.Recv()
			recvCh <- recvResult{chunk: ch, err: err}
			if err != nil {
				return
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			if h.OnError != nil {
				h.OnError(ctx.Err())
			}
			send(EventError, map[string]any{"success": false, "error": ctx.Err().Error()})
			send(EventEnd, map[string]any{"success": false, "message": "服务端取消"})
			return ctx.Err()

		case <-hbTickerC(hbTicker):
			send(EventHeartbeat, map[string]any{"ts": time.Now().Unix()})

		case rr, ok := <-recvCh:
			if !ok {
				return nil
			}
			if rr.err != nil {
				if errors.Is(rr.err, io.EOF) {
					if !finalSent {
						send(EventFinal, map[string]any{"success": true, "message": "流程执行完成"})
					}
					send(EventEnd, map[string]any{"success": true, "message": "连接结束"})
					return nil
				}
				if h.OnError != nil {
					h.OnError(rr.err)
				}
				send(EventError, map[string]any{"success": false, "error": rr.err.Error()})
				send(EventEnd, map[string]any{"success": false, "message": "流程执行失败"})
				return rr.err
			}

			chunk := rr.chunk

			if delta, ok := chunk.Metadata["delta_text"].(string); ok && delta != "" {
				if h.OnDelta != nil {
					h.OnDelta(delta, chunk)
				}
				send(EventToken, map[string]any{
					"delta":     delta,
					"step_id":   chunk.StepID,
					"timestamp": chunk.Timestamp,
				})
				continue
			}

			if h.OnData != nil {
				h.OnData(chunk)
			}
			send(EventData, map[string]any{
				"success":   chunk.Success,
				"data":      chunk.Data,
				"step_id":   chunk.StepID,
				"timestamp": chunk.Timestamp,
				"metadata":  chunk.Metadata,
			})

			if isFinal, _ := chunk.Metadata["is_final"].(bool); isFinal {
				if h.OnFinal != nil {
					h.OnFinal(chunk)
				}
				send(EventFinal, map[string]any{
					"success":   chunk.Success,
					"data":      chunk.Data,
					"metadata":  chunk.Metadata,
					"timestamp": chunk.Timestamp,
				})
				finalSent = true
				send(EventEnd, map[string]any{"success": true, "message": "流程执行完成"})
				return nil
			}
		}
	}
}
