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
}

func NewReprocessPipelineStub() *ReprocessPipelineStub {
	return &ReprocessPipelineStub{}
}

func (s *ReprocessPipelineStub) Schedule(ctx context.Context, input workflow.ReprocessInput) (workflow.ReprocessTask, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sequence++
	s.lastInput = input
	return workflow.ReprocessTask{
		JobID:       s.sequence,
		Status:      "scheduled",
		ScheduledAt: time.Now(),
	}, nil
}

func (s *ReprocessPipelineStub) LastInput() workflow.ReprocessInput {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastInput
}
