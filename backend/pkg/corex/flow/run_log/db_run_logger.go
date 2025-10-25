package run_log

import (
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/flow"
	"strconv"
	"time"
)

// pkg/corex/flow/run_log/db_run_logger.go

import (
	"context"
	"encoding/json"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/flow"
)

// DBRunLogger：把运行日志落到 DB（通过 repository）
type DBRunLogger struct {
	planRepo  *repo.AgentPlanRunRepository
	eventRepo *repo.AgentTaskEventRepository

	ch    chan models.AgentTaskEvent
	close chan struct{}
}

// buf 建议 >= 1024，防止执行热点阶段堵塞。
func NewDBRunLogger(planRepo *repo.AgentPlanRunRepository, eventRepo *repo.AgentTaskEventRepository, buf int) *DBRunLogger {
	if buf <= 0 {
		buf = 1024
	}
	l := &DBRunLogger{
		planRepo:  planRepo,
		eventRepo: eventRepo,
		ch:        make(chan models.AgentTaskEvent, buf),
		close:     make(chan struct{}),
	}
	go l.loop()
	return l
}

func (l *DBRunLogger) loop() {
	for {
		select {
		case e := <-l.ch:
			l.handle(e)
		case <-l.close:
			return
		}
	}
}

// 非阻塞投递；满了就丢弃，避免阻塞执行链路
func (l *DBRunLogger) emit(e models.AgentTaskEvent) {
	select {
	case l.ch <- e:
	default: /* drop on full */
	}
}

func (l *DBRunLogger) PlanStart(ctx context.Context, e models.AgentTaskEvent) { l.emit(e) }
func (l *DBRunLogger) TaskStart(ctx context.Context, e models.AgentTaskEvent) { l.emit(e) }
func (l *DBRunLogger) TaskOK(ctx context.Context, e models.AgentTaskEvent)    { l.emit(e) }
func (l *DBRunLogger) TaskErr(ctx context.Context, e models.AgentTaskEvent)   { l.emit(e) }
func (l *DBRunLogger) PlanEnd(ctx context.Context, e models.AgentTaskEvent)   { l.emit(e) }

// 可选：进程退出时调用
func (l *DBRunLogger) Close() { close(l.close) }

func (l *DBRunLogger) handle(e models.AgentTaskEvent) {
	switch e.Kind {
	case "plan.start":
		reqID := metaString(e.Meta, "request_id")
		traceID := metaString(e.Meta, "trace_id")
		status := metaString(e.Meta, "status")
		if status == "" {
			status = "running"
		}
		_ = l.planRepo.UpsertStart(context.Background(), &models.AgentPlanRun{
			PlanID:     e.PlanID,
			RequestID:  reqID,
			TraceID:    traceID,
			TenantID:   e.TenantID,
			UserID:     e.UserID,
			CustomerID: e.CustomerID,
			Status:     status,
			StartedAt:  nzTime(e.Ts, time.Now()),
			Meta:       e.Meta, // 原样保存 meta（包含 request_id/trace_id/status）
		})

	case "plan.end":
		status := metaString(e.Meta, "status")
		if status == "" {
			status = "completed"
		}
		_ = l.planRepo.MarkEnd(
			context.Background(),
			e.PlanID,
			status,
			nzTime(e.Ts, time.Now()),
			e.Meta, // 可包含 error 等信息
		)

	case "task.start", "task.ok", "task.err":
		// 如需确保 plan 头存在，可在此冪等 UpsertStart 一次（可选）
		// _ = l.planRepo.UpsertStart(context.Background(), &models.AgentPlanRun{
		//   PlanID: e.PlanID, TenantID: e.TenantID, UserID: e.UserID, CustomerID: e.CustomerID,
		//   Status: "running", StartedAt: nzTime(e.Ts, time.Now()),
		// })
		_ = l.eventRepo.Insert(context.Background(), &e)

	default:
		// ignore
	}
}

func nzTime(t time.Time, def time.Time) time.Time {
	if t.IsZero() {
		return def
	}
	return t
}

// 注意：这里参数用 datatypes.JSON（你的模型里就是这个类型），内部显式转成 []byte 再反序列化
func metaString(rawJSON []byte, key string) string {
	if len(rawJSON) == 0 {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal(rawJSON, &m); err != nil {
		return ""
	}
	v, ok := m[key]
	if !ok {
		return ""
	}
	switch vv := v.(type) {
	case string:
		return vv
	case float64:
		return strconv.FormatFloat(vv, 'f', -1, 64)
	case bool:
		if vv {
			return "true"
		}
		return "false"
	default:
		b, _ := json.Marshal(vv)
		return string(b)
	}
}
