package testenv

import (
	"context"
	"sync"
	"time"

	workflow "github.com/ArtisanCloud/PowerX/internal/workflow/knowledge_space"
)

// ReprocessPipelineStub records scheduling requests for assertions.
type ReprocessPipelineStub struct {
	mu        sync.Mutex
	lastInput workflow.ReprocessInput
	sequence  uint64
	inner     workflow.ReprocessPipeline
}

func NewReprocessPipelineStub() *ReprocessPipelineStub {
	return &ReprocessPipelineStub{}
}

func (s *ReprocessPipelineStub) WithInner(inner workflow.ReprocessPipeline) *ReprocessPipelineStub {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.inner = inner
	return s
}

func (s *ReprocessPipelineStub) Schedule(ctx context.Context, input workflow.ReprocessInput) (workflow.ReprocessTask, error) {
	s.mu.Lock()
	s.sequence++
	s.lastInput = input
	inner := s.inner
	jobID := s.sequence
	s.mu.Unlock()

	if inner != nil {
		task, err := inner.Schedule(ctx, input)
		if err != nil {
			return workflow.ReprocessTask{}, err
		}
		return task, nil
	}
	return workflow.ReprocessTask{
		JobID:       jobID,
		Status:      "scheduled",
		ScheduledAt: time.Now(),
	}, nil
}

func (s *ReprocessPipelineStub) LastInput() workflow.ReprocessInput {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastInput
}
