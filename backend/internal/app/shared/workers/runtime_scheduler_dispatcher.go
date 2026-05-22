package workers

import (
	"context"
	"sync/atomic"
	"time"

	runtimescheduler "github.com/ArtisanCloud/PowerX/internal/service/runtime_scheduler"
	pxlog "github.com/ArtisanCloud/PowerX/pkg/utils/logger"
)

type RuntimeSchedulerDispatcherOptions struct {
	Service     *runtimescheduler.Service
	Interval    time.Duration
	MaxInterval time.Duration
	BatchSize   int
	Logger      *pxlog.Logger
	Clock       func() time.Time
}

type RuntimeSchedulerDispatcher struct {
	service     *runtimescheduler.Service
	interval    time.Duration
	maxInterval time.Duration
	batchSize   int
	logger      *pxlog.Logger
	clock       func() time.Time
	paused      atomic.Bool
}

func NewRuntimeSchedulerDispatcher(opts RuntimeSchedulerDispatcherOptions) *RuntimeSchedulerDispatcher {
	if opts.Service == nil {
		return nil
	}
	interval := opts.Interval
	if interval <= 0 {
		interval = 5 * time.Second
	}
	maxInterval := opts.MaxInterval
	if maxInterval <= 0 {
		maxInterval = 30 * time.Second
	}
	if maxInterval < interval {
		maxInterval = interval
	}
	batchSize := opts.BatchSize
	if batchSize <= 0 {
		batchSize = 100
	}
	logger := opts.Logger
	if logger == nil {
		logger = pxlog.GetGlobalLogger()
	}
	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}
	return &RuntimeSchedulerDispatcher{
		service:     opts.Service,
		interval:    interval,
		maxInterval: maxInterval,
		batchSize:   batchSize,
		logger:      logger,
		clock:       clock,
	}
}

func (w *RuntimeSchedulerDispatcher) Run(ctx context.Context) {
	if w == nil || w.service == nil {
		return
	}
	currentInterval := w.interval
	timer := time.NewTimer(0)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			dueCount := 0
			if !w.paused.Load() {
				result, err := w.service.DispatchDue(ctx, runtimescheduler.DispatchDueInput{
					Now:   w.clock().UTC(),
					Limit: w.batchSize,
				})
				if err != nil {
					w.logger.WarnF(ctx, "[runtime_scheduler.dispatch] dispatch due jobs failed: %v", err)
				}
				if result != nil {
					dueCount = result.DueCount
				}
			}
			if dueCount > 0 {
				currentInterval = w.interval
			} else {
				currentInterval = w.nextBackoffInterval(currentInterval)
			}
			timer.Reset(currentInterval)
		}
	}
}

func (w *RuntimeSchedulerDispatcher) Pause() {
	if w == nil {
		return
	}
	w.paused.Store(true)
}

func (w *RuntimeSchedulerDispatcher) Resume() {
	if w == nil {
		return
	}
	w.paused.Store(false)
}

func (w *RuntimeSchedulerDispatcher) IsPaused() bool {
	if w == nil {
		return false
	}
	return w.paused.Load()
}

func (w *RuntimeSchedulerDispatcher) TriggerNow(ctx context.Context) {
	if w == nil || w.service == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if _, err := w.service.DispatchDue(ctx, runtimescheduler.DispatchDueInput{
		Now:   w.clock().UTC(),
		Limit: w.batchSize,
	}); err != nil {
		w.logger.WarnF(ctx, "[runtime_scheduler.dispatch] manual dispatch failed: %v", err)
	}
}

func (w *RuntimeSchedulerDispatcher) Interval() time.Duration {
	if w == nil {
		return 0
	}
	return w.interval
}

func (w *RuntimeSchedulerDispatcher) BatchSize() int {
	if w == nil {
		return 0
	}
	return w.batchSize
}

func (w *RuntimeSchedulerDispatcher) nextBackoffInterval(current time.Duration) time.Duration {
	if current < w.interval {
		current = w.interval
	}
	next := current * 2
	if next < w.interval {
		next = w.interval
	}
	if next > w.maxInterval {
		return w.maxInterval
	}
	return next
}
