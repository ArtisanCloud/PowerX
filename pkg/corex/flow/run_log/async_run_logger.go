package run_log

// pkg/corex/flow/run_log/logger.go

import (
	"context"
	"encoding/json"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/flow"
	"io"
)

// 一个通用异步实现，写入 io.Writer（文件/标准输出），或者发到 EventBus
type AsyncRunLogger struct {
	ch    chan models.AgentTaskEvent
	close chan struct{}
	w     io.Writer // 可以换成你的 EventBus Producer
}

func NewAsyncRunLogger(w io.Writer, buf int) *AsyncRunLogger {
	l := &AsyncRunLogger{ch: make(chan models.AgentTaskEvent, buf), close: make(chan struct{}), w: w}
	go l.loop()
	return l
}

func (l *AsyncRunLogger) loop() {
	enc := json.NewEncoder(l.w)
	for {
		select {
		case e := <-l.ch:
			_ = enc.Encode(e) // 失败可以打 warn 或丢弃，避免阻塞执行路径
		case <-l.close:
			return
		}
	}
}
func (l *AsyncRunLogger) emit(e models.AgentTaskEvent) {
	select {
	case l.ch <- e:
	default: /* drop on full */
	}
}
func (l *AsyncRunLogger) PlanStart(ctx context.Context, e models.AgentTaskEvent) { l.emit(e) }
func (l *AsyncRunLogger) TaskStart(ctx context.Context, e models.AgentTaskEvent) { l.emit(e) }
func (l *AsyncRunLogger) TaskOK(ctx context.Context, e models.AgentTaskEvent)    { l.emit(e) }
func (l *AsyncRunLogger) TaskErr(ctx context.Context, e models.AgentTaskEvent)   { l.emit(e) }
func (l *AsyncRunLogger) PlanEnd(ctx context.Context, e models.AgentTaskEvent)   { l.emit(e) }
