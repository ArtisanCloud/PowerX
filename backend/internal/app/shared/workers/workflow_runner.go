package workers

import (
	"context"
	"sync/atomic"
	"time"

	workflowsvc "github.com/ArtisanCloud/PowerX/internal/service/workflow"
	pxlog "github.com/ArtisanCloud/PowerX/pkg/utils/logger"
)

type WorkflowRunnerService interface {
	DrainDueSteps(ctx context.Context, opts workflowsvc.DrainDueStepsOptions) (workflowsvc.RunDueStepsResult, error)
}

type WorkflowRunnerWorkerOptions struct {
	Service       WorkflowRunnerService
	Interval      time.Duration
	MaxInterval   time.Duration
	LeaseDuration time.Duration
	BatchSize     int
	MaxIterations int
	LeaseOwner    string
	Logger        *pxlog.Logger
}

type WorkflowRunnerWorker struct {
	service       WorkflowRunnerService
	interval      time.Duration
	maxInterval   time.Duration
	leaseDuration time.Duration
	batchSize     int
	maxIterations int
	leaseOwner    string
	logger        *pxlog.Logger
	paused        atomic.Bool
}

func NewWorkflowRunnerWorker(opts WorkflowRunnerWorkerOptions) *WorkflowRunnerWorker {
	if opts.Service == nil {
		return nil
	}
	interval := opts.Interval
	if interval <= 0 {
		interval = 1 * time.Second
	}
	maxInterval := opts.MaxInterval
	if maxInterval <= 0 {
		maxInterval = 10 * time.Second
	}
	if maxInterval < interval {
		maxInterval = interval
	}
	leaseDuration := opts.LeaseDuration
	if leaseDuration <= 0 {
		leaseDuration = 30 * time.Second
	}
	batchSize := opts.BatchSize
	if batchSize <= 0 {
		batchSize = 20
	}
	maxIterations := opts.MaxIterations
	if maxIterations <= 0 {
		maxIterations = 50
	}
	leaseOwner := opts.LeaseOwner
	if leaseOwner == "" {
		leaseOwner = "workflow-runner"
	}
	logger := opts.Logger
	if logger == nil {
		logger = pxlog.GetGlobalLogger()
	}
	return &WorkflowRunnerWorker{
		service:       opts.Service,
		interval:      interval,
		maxInterval:   maxInterval,
		leaseDuration: leaseDuration,
		batchSize:     batchSize,
		maxIterations: maxIterations,
		leaseOwner:    leaseOwner,
		logger:        logger,
	}
}

func (w *WorkflowRunnerWorker) Run(ctx context.Context) {
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
			active := false
			if !w.paused.Load() {
				result, err := w.service.DrainDueSteps(ctx, workflowsvc.DrainDueStepsOptions{
					LeaseOwner:    w.leaseOwner,
					LeaseDuration: w.leaseDuration,
					BatchSize:     w.batchSize,
					MaxIterations: w.maxIterations,
				})
				if err != nil {
					w.logger.WarnF(ctx, "[workflow.runner] drain due steps failed: %v", err)
				}
				active = result.Leased > 0 || result.Completed > 0 || result.Waiting > 0 || result.Failed > 0 || result.Skipped > 0
			}
			if active {
				currentInterval = w.interval
			} else {
				currentInterval = w.nextBackoffInterval(currentInterval)
			}
			timer.Reset(currentInterval)
		}
	}
}

func (w *WorkflowRunnerWorker) Pause() {
	if w == nil {
		return
	}
	w.paused.Store(true)
}

func (w *WorkflowRunnerWorker) Resume() {
	if w == nil {
		return
	}
	w.paused.Store(false)
}

func (w *WorkflowRunnerWorker) IsPaused() bool {
	if w == nil {
		return false
	}
	return w.paused.Load()
}

func (w *WorkflowRunnerWorker) TriggerNow(ctx context.Context) {
	if w == nil || w.service == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if _, err := w.service.DrainDueSteps(ctx, workflowsvc.DrainDueStepsOptions{
		LeaseOwner:    w.leaseOwner,
		LeaseDuration: w.leaseDuration,
		BatchSize:     w.batchSize,
		MaxIterations: w.maxIterations,
	}); err != nil {
		w.logger.WarnF(ctx, "[workflow.runner] manual drain failed: %v", err)
	}
}

func (w *WorkflowRunnerWorker) Interval() time.Duration {
	if w == nil {
		return 0
	}
	return w.interval
}

func (w *WorkflowRunnerWorker) BatchSize() int {
	if w == nil {
		return 0
	}
	return w.batchSize
}

func (w *WorkflowRunnerWorker) nextBackoffInterval(current time.Duration) time.Duration {
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
