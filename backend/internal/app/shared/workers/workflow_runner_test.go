package workers

import (
	"context"
	"testing"
	"time"

	workflowsvc "github.com/ArtisanCloud/PowerX/internal/service/workflow"
)

type workflowRunnerServiceStub struct {
	calls int
	opts  workflowsvc.DrainDueStepsOptions
	err   error
}

func (s *workflowRunnerServiceStub) DrainDueSteps(_ context.Context, opts workflowsvc.DrainDueStepsOptions) (workflowsvc.RunDueStepsResult, error) {
	s.calls++
	s.opts = opts
	return workflowsvc.RunDueStepsResult{Leased: 1, Completed: 1}, s.err
}

func TestWorkflowRunnerWorkerTriggerNowDrainsDueSteps(t *testing.T) {
	service := &workflowRunnerServiceStub{}
	worker := NewWorkflowRunnerWorker(WorkflowRunnerWorkerOptions{
		Service:       service,
		Interval:      50 * time.Millisecond,
		LeaseDuration: 3 * time.Second,
		BatchSize:     7,
		MaxIterations: 9,
		LeaseOwner:    "test-owner",
	})
	if worker == nil {
		t.Fatal("worker should not be nil")
	}

	worker.TriggerNow(context.Background())

	if service.calls != 1 {
		t.Fatalf("expected one drain call, got %d", service.calls)
	}
	if service.opts.LeaseOwner != "test-owner" || service.opts.BatchSize != 7 || service.opts.MaxIterations != 9 || service.opts.LeaseDuration != 3*time.Second {
		t.Fatalf("unexpected drain options: %#v", service.opts)
	}
}

func TestWorkflowRunnerWorkerNilService(t *testing.T) {
	if worker := NewWorkflowRunnerWorker(WorkflowRunnerWorkerOptions{}); worker != nil {
		t.Fatalf("expected nil worker without service, got %#v", worker)
	}
}
