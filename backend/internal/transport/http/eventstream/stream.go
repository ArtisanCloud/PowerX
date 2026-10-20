// Package eventstream 提供只读 SSE 传输；心跳只保活，不查询业务状态。
package eventstream

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/infra/opswatch"
)

type Event struct {
	Name string
	Data any
}

// Serve 先发送当前状态，再按变更通知发送；最长一分钟后重新鉴权连接。
// 订阅必须在初次读取之前建立，避免初始查询与监听之间的事件丢失。
func Serve(ctx context.Context, w http.ResponseWriter, sub *opswatch.Subscription, snapshot func([]string) (Event, error)) error {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	controller := http.NewResponseController(w)
	send := func(event Event) error {
		raw, e := json.Marshal(event.Data)
		if e != nil {
			return e
		}
		_ = controller.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if _, e = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Name, raw); e != nil {
			return e
		}
		e = controller.Flush()
		_ = controller.SetWriteDeadline(time.Time{})
		return e
	}
	write := func(names []string) error {
		event, e := snapshot(names)
		if e != nil {
			return e
		}
		return send(event)
	}
	if e := write(nil); e != nil {
		return e
	}
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	reauthorize := time.NewTimer(time.Minute)
	defer reauthorize.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-reauthorize.C:
			return send(Event{Name: "reconnect", Data: map[string]string{"reason": "reauthorize"}})
		case e, ok := <-sub.Errors:
			if !ok {
				return nil
			}
			return e
		case names, ok := <-sub.Changes:
			if !ok {
				return nil
			}
			if e := write(names); e != nil {
				return e
			}
		case <-heartbeat.C:
			_ = controller.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if _, e := fmt.Fprint(w, ": keepalive\n\n"); e != nil {
				return e
			}
			if e := controller.Flush(); e != nil {
				return e
			}
			_ = controller.SetWriteDeadline(time.Time{})
		}
	}
}
