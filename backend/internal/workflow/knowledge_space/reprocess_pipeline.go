package knowledge_space

import (
	"context"
	"sync"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/google/uuid"
)

// ReprocessInput captures the feedback metadata passed to the pipeline.
type ReprocessInput struct {
	SpaceID     uuid.UUID
	CaseID      uuid.UUID
	Severity    string
	IssueType   string
	ChunkIDs    []uuid.UUID
	RequestedBy string
}

// ReprocessTask contains scheduling details for a reprocess job.
type ReprocessTask struct {
	JobID        uint64
	Status       string
	ScheduledAt  time.Time
	RollbackHint string
}

// ReprocessPipeline defines scheduling behavior for hot update workflows.
type ReprocessPipeline interface {
	Schedule(ctx context.Context, input ReprocessInput) (ReprocessTask, error)
}

// defaultReprocessPipeline emits scheduling events onto the event bus.
type defaultReprocessPipeline struct {
	bus     event_bus.EventBus
	mu      sync.Mutex
	counter uint64
	clock   func() time.Time
}

// NewReprocessPipeline builds a default pipeline backed by the event bus.
func NewReprocessPipeline(bus event_bus.EventBus, clock func() time.Time) ReprocessPipeline {
	if clock == nil {
		clock = time.Now
	}
	return &defaultReprocessPipeline{
		bus:   bus,
		clock: clock,
	}
}

func (p *defaultReprocessPipeline) Schedule(ctx context.Context, input ReprocessInput) (ReprocessTask, error) {
	p.mu.Lock()
	p.counter++
	jobID := p.counter
	p.mu.Unlock()

	task := ReprocessTask{
		JobID:        jobID,
		Status:       "scheduled",
		ScheduledAt:  p.clock(),
		RollbackHint: "previous_bundle",
	}

	if p.bus != nil {
		payload := map[string]any{
			"job_id":      jobID,
			"space_id":    input.SpaceID.String(),
			"case_id":     input.CaseID.String(),
			"severity":    input.Severity,
			"issue_type":  input.IssueType,
			"chunk_ids":   stringifyChunks(input.ChunkIDs),
			"requestedBy": input.RequestedBy,
		}
		p.bus.Publish("knowledge.feedback.reprocess", payload, ctx)
	}

	return task, nil
}

func stringifyChunks(chunkIDs []uuid.UUID) []string {
	out := make([]string, 0, len(chunkIDs))
	for _, c := range chunkIDs {
		out = append(out, c.String())
	}
	return out
}
