package dto

import (
	"errors"
	"io"
	"time"

	aschema "github.com/ArtisanCloud/PowerX/internal/server/agent/schemas"
	"github.com/cloudwego/eino/schema"
	"github.com/gorilla/websocket"
	"golang.org/x/net/context"
)

// WriteToWS：与 WriteToSSE 同步的 WS 输出工具
// - 事件语义与 sse.go 中一致：start/intent/token/action/data/final/end/error/heartbeat
func WriteToWS(
	ctx context.Context,
	conn *websocket.Conn,
	flowID string,
	execID string,
	sr *schema.StreamReader[*aschema.ExecutionResult],
	heartbeatInterval time.Duration,
) error {
	defer sr.Close()

	send := func(msg WSMessage) error {
		msg.Timestamp = time.Now().Unix()
		return conn.WriteJSON(msg)
	}

	// 开始帧
	if err := send(WSMessage{
		Type: EventStart,
		Data: map[string]any{
			"flow_id":      flowID,
			"execution_id": execID,
			"message":      "开始执行流程",
		},
	}); err != nil {
		return err
	}

	// 心跳
	var hb *time.Ticker
	if heartbeatInterval > 0 {
		hb = time.NewTicker(heartbeatInterval)
		defer hb.Stop()
	}

	type recv struct {
		chunk *aschema.ExecutionResult
		err   error
	}
	recvCh := make(chan recv, 1)
	go func() {
		defer close(recvCh)
		for {
			ch, err := sr.Recv()
			recvCh <- recv{chunk: ch, err: err}
			if err != nil {
				return
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			_ = send(WSMessage{Type: EventError, Data: map[string]any{"success": false, "error": ctx.Err().Error()}})
			_ = send(WSMessage{Type: EventEnd, Data: map[string]any{"success": false, "message": "服务端取消"}})
			return ctx.Err()

		case <-hbChan(hb):
			_ = send(WSMessage{Type: EventHeartbeat, Data: map[string]any{"ts": time.Now().Unix()}})

		case rr, ok := <-recvCh:
			if !ok {
				return nil
			}
			if rr.err != nil {
				if errors.Is(rr.err, io.EOF) {
					_ = send(WSMessage{Type: EventFinal, Data: map[string]any{"success": true, "message": "流程执行完成"}})
					_ = send(WSMessage{Type: EventEnd, Data: map[string]any{"success": true, "message": "连接结束"}})
					return nil
				}
				_ = send(WSMessage{Type: EventError, Data: map[string]any{"success": false, "error": rr.err.Error()}})
				_ = send(WSMessage{Type: EventEnd, Data: map[string]any{"success": false, "message": "流程执行失败"}})
				return rr.err
			}

			chunk := rr.chunk

			// 增量文本
			if delta, ok := chunk.Metadata["delta_text"].(string); ok && delta != "" {
				_ = send(WSMessage{Type: EventToken, Data: map[string]any{
					"delta":     delta,
					"step_id":   chunk.StepID,
					"timestamp": chunk.Timestamp,
				}})
				continue
			}

			// 前端动作
			if act, ok := chunk.Metadata["action"].(map[string]any); ok && len(act) > 0 {
				_ = send(WSMessage{Type: EventAction, Data: act})
			}

			// 中间数据
			_ = send(WSMessage{Type: EventData, Data: map[string]any{
				"success":   chunk.Success,
				"data":      chunk.Data,
				"step_id":   chunk.StepID,
				"timestamp": chunk.Timestamp,
				"metadata":  chunk.Metadata,
			}})

			// 最终帧
			if isFinal, _ := chunk.Metadata["is_final"].(bool); isFinal {
				_ = send(WSMessage{Type: EventFinal, Data: map[string]any{
					"success":   chunk.Success,
					"data":      chunk.Data,
					"metadata":  chunk.Metadata,
					"timestamp": chunk.Timestamp,
				}})
				_ = send(WSMessage{Type: EventEnd, Data: map[string]any{"success": true, "message": "流程执行完成"}})
				return nil
			}
		}
	}
}

func hbChan(t *time.Ticker) <-chan time.Time {
	if t == nil {
		return nil
	}
	return t.C
}
