package dto

import (
	aschema "github.com/ArtisanCloud/PowerX/internal/server/agent/schemas"
	"github.com/cloudwego/eino/schema"
	"github.com/gin-gonic/gin"
	"time"
)

import (
	"errors"
	"io"
)

// api/http/dto/sse.go

// WS 消息 Envelope（后端 → 前端）
type WSMessage struct {
	Type      string                 `json:"type" example:"intent|token|final|error|action|tool_call|tool_result|heartbeat"`
	Data      map[string]interface{} `json:"data,omitempty"`
	Timestamp int64                  `json:"timestamp"`
}

// WriteToSSE：安全/健壮的 SSE 写入封装
// 事件约定：
// - "start": 开始，携带 flow_id/execution_id
// - "intent": 可选，若上游先推送意图
// - "token": 逐字/增量输出（如果有）
// - "action": 后端指令下发（如果有）
// - "data": 中间步骤结果（兼容旧事件名）
// - "final": 最终结果（单次）
// - "end": 结束（一定会发）
// - "error": 错误
func WriteToSSE(
	c *gin.Context,
	flowID string,
	execID string,
	sr *schema.StreamReader[*aschema.ExecutionResult],
	// 可选：心跳间隔，<=0 表示不发心跳
	heartbeatInterval time.Duration,
) error {
	defer sr.Close()

	ctx := c.Request.Context()

	// 基础头
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Content-Type", "text/event-stream; charset=utf-8")
	c.Header("X-Accel-Buffering", "no") // 若走 Nginx，禁止缓冲

	// 可选：告诉浏览器自动重连间隔（秒）。非标准但很多客户端支持
	// c.Writer.WriteString("retry: 2000\n\n")

	send := func(event string, data any) {
		c.SSEvent(event, data)
		c.Writer.Flush()
	}

	// 开始帧
	send("start", map[string]any{
		"flow_id":      flowID,
		"execution_id": execID,
		"message":      "开始执行流程",
	})

	// 心跳与取消
	var hbTicker *time.Ticker
	if heartbeatInterval > 0 {
		hbTicker = time.NewTicker(heartbeatInterval)
		defer hbTicker.Stop()
	}

	finalSent := false

	// 将 sr.Recv() 放到 goroutine，避免阻塞事件循环
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
			if err != nil { // 出错或 EOF 都退出
				return
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			send("error", map[string]any{"success": false, "error": ctx.Err().Error()})
			send("end", map[string]any{"success": false, "message": "服务端取消"})
			return ctx.Err()

		case <-hbTickerC(hbTicker):
			send("heartbeat", map[string]any{"ts": time.Now().Unix()})

		case rr, ok := <-recvCh:
			if !ok {
				// goroutine 结束（正常或异常已在 rr 分支处理）
				return nil
			}
			if rr.err != nil {
				if errors.Is(rr.err, io.EOF) {
					if !finalSent {
						send("final", map[string]any{"success": true, "message": "流程执行完成"})
					}
					send("end", map[string]any{"success": true, "message": "连接结束"})
					return nil
				}
				send("error", map[string]any{"success": false, "error": rr.err.Error()})
				send("end", map[string]any{"success": false, "message": "流程执行失败"})
				return rr.err
			}

			chunk := rr.chunk

			// 增量文本
			if delta, ok := chunk.Metadata["delta_text"].(string); ok && delta != "" {
				send("token", map[string]any{
					"delta":     delta,
					"step_id":   chunk.StepID,
					"timestamp": chunk.Timestamp,
				})
				continue
			}

			// 前端动作
			if act, ok := chunk.Metadata["action"].(map[string]any); ok && len(act) > 0 {
				send("action", act)
			}

			// 中间数据
			send("data", map[string]any{
				"success":   chunk.Success,
				"data":      chunk.Data,
				"step_id":   chunk.StepID,
				"timestamp": chunk.Timestamp,
				"metadata":  chunk.Metadata,
			})

			// 最终帧
			if isFinal, _ := chunk.Metadata["is_final"].(bool); isFinal {
				send("final", map[string]any{
					"success":   chunk.Success,
					"data":      chunk.Data,
					"metadata":  chunk.Metadata,
					"timestamp": chunk.Timestamp,
				})
				finalSent = true
				send("end", map[string]any{"success": true, "message": "流程执行完成"})
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
	// 在发送 start 前（或正好发送时）触发
	OnStart func(flowID, execID string)
	// 每次增量 token（如果 chunk.Metadata["delta_text"] 存在）
	OnDelta func(delta string, chunk *aschema.ExecutionResult)
	// 每个中间数据块
	OnData func(chunk *aschema.ExecutionResult)
	// 收到最终帧（is_final=true）
	OnFinal func(final *aschema.ExecutionResult)
	// 流错误（非 EOF）
	OnError func(err error)
	// 心跳间隔；<=0 表示不发心跳
	HeartbeatInterval time.Duration
}

// 不影响旧签名的情况下，提供带回调的版本
func WriteToSSEWithTap(
	c *gin.Context,
	flowID string,
	execID string,
	sr *schema.StreamReader[*aschema.ExecutionResult],
	h SSEHooks,
) error {
	defer sr.Close()

	ctx := c.Request.Context()

	// 头
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Content-Type", "text/event-stream; charset=utf-8")
	c.Header("X-Accel-Buffering", "no")

	send := func(event string, data any) {
		c.SSEvent(event, data)
		c.Writer.Flush()
	}

	// start
	if h.OnStart != nil {
		h.OnStart(flowID, execID)
	}
	send("start", map[string]any{
		"flow_id":      flowID,
		"execution_id": execID,
		"message":      "开始执行流程",
	})

	// 心跳
	var hbTicker *time.Ticker
	if h.HeartbeatInterval > 0 {
		hbTicker = time.NewTicker(h.HeartbeatInterval)
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
			if h.OnError != nil {
				h.OnError(ctx.Err())
			}
			send("error", map[string]any{"success": false, "error": ctx.Err().Error()})
			send("end", map[string]any{"success": false, "message": "服务端取消"})
			return ctx.Err()

		case <-hbTickerC(hbTicker):
			send("heartbeat", map[string]any{"ts": time.Now().Unix()})

		case rr, ok := <-recvCh:
			if !ok {
				return nil
			}
			if rr.err != nil {
				if errors.Is(rr.err, io.EOF) {
					if !finalSent {
						send("final", map[string]any{"success": true, "message": "流程执行完成"})
					}
					send("end", map[string]any{"success": true, "message": "连接结束"})
					return nil
				}
				if h.OnError != nil {
					h.OnError(rr.err)
				}
				send("error", map[string]any{"success": false, "error": rr.err.Error()})
				send("end", map[string]any{"success": false, "message": "流程执行失败"})
				return rr.err
			}

			chunk := rr.chunk

			// 增量
			if delta, ok := chunk.Metadata["delta_text"].(string); ok && delta != "" {
				if h.OnDelta != nil {
					h.OnDelta(delta, chunk)
				}
				send("token", map[string]any{
					"delta":     delta,
					"step_id":   chunk.StepID,
					"timestamp": chunk.Timestamp,
				})
				continue
			}

			// 中间数据
			if h.OnData != nil {
				h.OnData(chunk)
			}
			send("data", map[string]any{
				"success":   chunk.Success,
				"data":      chunk.Data,
				"step_id":   chunk.StepID,
				"timestamp": chunk.Timestamp,
				"metadata":  chunk.Metadata,
			})

			// 最终
			if isFinal, _ := chunk.Metadata["is_final"].(bool); isFinal {
				if h.OnFinal != nil {
					h.OnFinal(chunk)
				}
				send("final", map[string]any{
					"success":   chunk.Success,
					"data":      chunk.Data,
					"metadata":  chunk.Metadata,
					"timestamp": chunk.Timestamp,
				})
				finalSent = true
				send("end", map[string]any{"success": true, "message": "流程执行完成"})
				return nil
			}
		}
	}
}
