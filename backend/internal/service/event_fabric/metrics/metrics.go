package metrics

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	pxlog "github.com/ArtisanCloud/PowerX/pkg/utils/logger"
)

// Recorder 用于记录事件骨干的关键运行指标。
type Recorder interface {
	ObserveDelivery(ctx context.Context, success bool, latency time.Duration)
	ObserveRetry(ctx context.Context, delay time.Duration)
	ObserveDLQChange(ctx context.Context, delta int64)
	ObserveReplay(ctx context.Context, duration time.Duration, err error)
	ObserveAuthorizationEvaluation(ctx context.Context, decision string, cacheHit bool, latency time.Duration)
	ObserveTaskDriverInit(ctx context.Context, driver string, supportsBlocking bool)
	Snapshot() Snapshot
}

// Snapshot 描述当前指标快照，方便对接 Prometheus 或日志采集。
type Snapshot struct {
	DeliveriesTotal    uint64
	DeliveriesSuccess  uint64
	DeliveriesFailed   uint64
	DeliverySuccess    float64
	AvgDeliveryLatency time.Duration

	RetriesScheduled uint64
	AvgRetryDelay    time.Duration
	MaxRetryDelay    time.Duration

	DLQBacklog int64

	ReplayTotal        uint64
	ReplayFailed       uint64
	AvgReplayLatency   time.Duration
	LastReplayLatency  time.Duration
	LastReplayErrorMsg string

	AuthorizationEvaluations  uint64
	AuthorizationAllows       uint64
	AuthorizationBlocks       uint64
	AuthorizationChallenges   uint64
	AuthorizationCacheHits    uint64
	AuthorizationCacheHitRate float64
	AvgAuthorizationLatency   time.Duration

	TaskDriverInitTotal     uint64
	TaskDriverBlockingTotal uint64
	LastTaskDriver          string
}

// NewRecorder 构建指标记录器。
func NewRecorder() *RecorderImpl {
	return &RecorderImpl{
		logger: pxlog.GetGlobalLogger(),
		now:    time.Now,
	}
}

// NewNoop 返回空实现，便于测试或未启用指标时复用。
func NewNoop() Recorder {
	return noopRecorder{}
}

// RecorderImpl 为事件骨干提供线程安全的指标收集实现。
type RecorderImpl struct {
	logger *pxlog.Logger
	now    func() time.Time

	totalDeliveries   atomic.Uint64
	successDeliveries atomic.Uint64
	failedDeliveries  atomic.Uint64
	deliveryLatencyNS atomic.Int64

	retryCount    atomic.Uint64
	retryDelayNS  atomic.Int64
	maxRetryDelay atomic.Int64

	dlqBacklog atomic.Int64

	replayCount        atomic.Uint64
	replayFailureCount atomic.Uint64
	replayLatencyNS    atomic.Int64
	lastReplayLatency  atomic.Int64
	lastReplayErr      atomic.Value

	authorizationEvaluations atomic.Uint64
	authorizationAllows      atomic.Uint64
	authorizationBlocks      atomic.Uint64
	authorizationChallenges  atomic.Uint64
	authorizationCacheHits   atomic.Uint64
	authorizationLatencyNS   atomic.Int64

	taskDriverInitTotal     atomic.Uint64
	taskDriverBlockingTotal atomic.Uint64
	lastTaskDriver          atomic.Value
}

// ObserveDelivery 记录单次投递结果与耗时。
func (r *RecorderImpl) ObserveDelivery(ctx context.Context, success bool, latency time.Duration) {
	total := r.totalDeliveries.Add(1)
	r.deliveryLatencyNS.Add(latency.Nanoseconds())

	if success {
		r.successDeliveries.Add(1)
		if total%500 == 0 {
			r.logger.InfoF(ctx, "[event_fabric.metrics] delivery success ratio=%.4f total=%d",
				r.deliverySuccessRatio(), total)
		}
		return
	}

	r.failedDeliveries.Add(1)
	r.logger.WarnF(ctx, "[event_fabric.metrics] delivery failure detected ratio=%.4f latency=%s total=%d",
		r.deliverySuccessRatio(), latency, total)
}

// ObserveRetry 记录重试调度延迟。
func (r *RecorderImpl) ObserveRetry(ctx context.Context, delay time.Duration) {
	if delay < 0 {
		delay = 0
	}
	count := r.retryCount.Add(1)
	r.retryDelayNS.Add(delay.Nanoseconds())
	r.updateMaxDelay(delay)

	if delay >= 5*time.Second {
		r.logger.WarnF(ctx, "[event_fabric.metrics] retry delay high delay=%s count=%d", delay, count)
	}
}

// ObserveDLQChange 更新死信积压量。
func (r *RecorderImpl) ObserveDLQChange(ctx context.Context, delta int64) {
	backlog := r.dlqBacklog.Add(delta)
	if backlog < 0 {
		r.dlqBacklog.Store(0)
		backlog = 0
	}
	if backlog > 0 && backlog%100 == 0 {
		r.logger.WarnF(ctx, "[event_fabric.metrics] dlq backlog=%d", backlog)
	}
}

// ObserveReplay 记录回放耗时与错误。
func (r *RecorderImpl) ObserveReplay(ctx context.Context, duration time.Duration, err error) {
	count := r.replayCount.Add(1)
	r.replayLatencyNS.Add(duration.Nanoseconds())
	r.lastReplayLatency.Store(duration.Nanoseconds())

	if err != nil {
		r.replayFailureCount.Add(1)
		msg := fmt.Sprintf("%v", err)
		if len(msg) > 128 {
			msg = msg[:128]
		}
		r.lastReplayErr.Store(msg)
		r.logger.WarnF(ctx, "[event_fabric.metrics] replay failed duration=%s error=%s count=%d", duration, msg, count)
		return
	}
	r.lastReplayErr.Store("")
	if duration > 30*time.Second {
		r.logger.WarnF(ctx, "[event_fabric.metrics] replay duration high duration=%s count=%d", duration, count)
	}
}

// ObserveAuthorizationEvaluation 记录授权评估指标。
func (r *RecorderImpl) ObserveAuthorizationEvaluation(ctx context.Context, decision string, cacheHit bool, latency time.Duration) {
	r.authorizationEvaluations.Add(1)
	if cacheHit {
		r.authorizationCacheHits.Add(1)
	}
	if latency < 0 {
		latency = 0
	}
	r.authorizationLatencyNS.Add(latency.Nanoseconds())

	switch strings.ToLower(strings.TrimSpace(decision)) {
	case "allow":
		r.authorizationAllows.Add(1)
	case "block":
		r.authorizationBlocks.Add(1)
	case "challenge":
		r.authorizationChallenges.Add(1)
	default:
		if decision != "" {
			r.logger.WarnF(ctx, "[event_fabric.metrics] unknown authorization decision=%s", decision)
		}
	}
}

// ObserveTaskDriverInit 记录任务驱动初始化信息。
func (r *RecorderImpl) ObserveTaskDriverInit(ctx context.Context, driver string, supportsBlocking bool) {
	r.taskDriverInitTotal.Add(1)
	if supportsBlocking {
		r.taskDriverBlockingTotal.Add(1)
	}
	r.lastTaskDriver.Store(strings.TrimSpace(driver))
	r.logger.InfoF(ctx, "[event_fabric.metrics] task driver initialized driver=%s blocking=%t", driver, supportsBlocking)
}

// Snapshot 返回指标快照。
func (r *RecorderImpl) Snapshot() Snapshot {
	total := r.totalDeliveries.Load()
	success := r.successDeliveries.Load()
	failed := r.failedDeliveries.Load()
	retries := r.retryCount.Load()
	replayTotal := r.replayCount.Load()
	replayFailed := r.replayFailureCount.Load()
	authTotal := r.authorizationEvaluations.Load()
	authAllows := r.authorizationAllows.Load()
	authBlocks := r.authorizationBlocks.Load()
	authChallenges := r.authorizationChallenges.Load()
	authHits := r.authorizationCacheHits.Load()
	taskInit := r.taskDriverInitTotal.Load()
	taskBlocking := r.taskDriverBlockingTotal.Load()

	var avgDelivery time.Duration
	if total > 0 {
		avgDelivery = time.Duration(r.deliveryLatencyNS.Load() / int64(total))
	}

	var avgRetry time.Duration
	if retries > 0 {
		avgRetry = time.Duration(r.retryDelayNS.Load() / int64(retries))
	}

	var avgReplay time.Duration
	if replayTotal > 0 {
		avgReplay = time.Duration(r.replayLatencyNS.Load() / int64(replayTotal))
	}

	var avgAuthLatency time.Duration
	if authTotal > 0 {
		avgAuthLatency = time.Duration(r.authorizationLatencyNS.Load() / int64(authTotal))
	}

	var authHitRate float64
	if authTotal > 0 {
		authHitRate = float64(authHits) / float64(authTotal)
	}

	lastLatency := time.Duration(r.lastReplayLatency.Load())
	lastErr, _ := r.lastReplayErr.Load().(string)
	lastTaskDriver, _ := r.lastTaskDriver.Load().(string)

	return Snapshot{
		DeliveriesTotal:           total,
		DeliveriesSuccess:         success,
		DeliveriesFailed:          failed,
		DeliverySuccess:           r.deliverySuccessRatio(),
		AvgDeliveryLatency:        avgDelivery,
		RetriesScheduled:          retries,
		AvgRetryDelay:             avgRetry,
		MaxRetryDelay:             time.Duration(r.maxRetryDelay.Load()),
		DLQBacklog:                r.dlqBacklog.Load(),
		ReplayTotal:               replayTotal,
		ReplayFailed:              replayFailed,
		AvgReplayLatency:          avgReplay,
		LastReplayLatency:         lastLatency,
		LastReplayErrorMsg:        lastErr,
		AuthorizationEvaluations:  authTotal,
		AuthorizationAllows:       authAllows,
		AuthorizationBlocks:       authBlocks,
		AuthorizationChallenges:   authChallenges,
		AuthorizationCacheHits:    authHits,
		AuthorizationCacheHitRate: authHitRate,
		AvgAuthorizationLatency:   avgAuthLatency,
		TaskDriverInitTotal:       taskInit,
		TaskDriverBlockingTotal:   taskBlocking,
		LastTaskDriver:            lastTaskDriver,
	}
}

func (r *RecorderImpl) deliverySuccessRatio() float64 {
	total := r.totalDeliveries.Load()
	if total == 0 {
		return 1
	}
	return float64(r.successDeliveries.Load()) / float64(total)
}

func (r *RecorderImpl) updateMaxDelay(delay time.Duration) {
	ns := delay.Nanoseconds()
	if ns <= 0 {
		return
	}
	for {
		prev := r.maxRetryDelay.Load()
		if ns <= prev {
			return
		}
		if r.maxRetryDelay.CompareAndSwap(prev, ns) {
			return
		}
	}
}

type noopRecorder struct{}

func (noopRecorder) ObserveDelivery(context.Context, bool, time.Duration)                        {}
func (noopRecorder) ObserveRetry(context.Context, time.Duration)                                 {}
func (noopRecorder) ObserveDLQChange(context.Context, int64)                                     {}
func (noopRecorder) ObserveReplay(context.Context, time.Duration, error)                         {}
func (noopRecorder) ObserveAuthorizationEvaluation(context.Context, string, bool, time.Duration) {}
func (noopRecorder) ObserveTaskDriverInit(context.Context, string, bool)                          {}
func (noopRecorder) Snapshot() Snapshot                                                          { return Snapshot{} }

// EncodeSnapshot 将快照转换为十六进制字符串，便于写入日志或指标系统。
func EncodeSnapshot(s Snapshot) string {
	payload := fmt.Sprintf("total:%d,success:%d,failed:%d,ratio:%.4f,dlq:%d,retries:%d,maxRetry:%s,replay:%d/%d",
		s.DeliveriesTotal,
		s.DeliveriesSuccess,
		s.DeliveriesFailed,
		s.DeliverySuccess,
		s.DLQBacklog,
		s.RetriesScheduled,
		s.MaxRetryDelay,
		s.ReplayTotal,
		s.ReplayFailed,
	)
	return hex.EncodeToString([]byte(payload))
}
