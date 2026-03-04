package knowledge_space

import (
	"context"
	"strings"
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

type ReprocessPipelineOptions struct {
	EventBus   event_bus.EventBus
	EventTopic string
	Clock      func() time.Time
}

// defaultReprocessPipeline emits scheduling events onto the event bus and runs a minimal reprocess flow.
type defaultReprocessPipeline struct {
	bus        event_bus.EventBus
	eventTopic string
	mu         sync.Mutex
	counter    uint64
	clock      func() time.Time
}

// NewReprocessPipeline builds a default pipeline backed by the event bus.
func NewReprocessPipeline(opts ReprocessPipelineOptions) ReprocessPipeline {
	if opts.Clock == nil {
		opts.Clock = time.Now
	}
	topic := strings.TrimSpace(opts.EventTopic)
	if topic == "" {
		topic = "knowledge.feedback.reprocess"
	}
	return &defaultReprocessPipeline{
		bus:        opts.EventBus,
		eventTopic: topic,
		clock:      opts.Clock,
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
		p.bus.Publish(p.eventTopic, payload, ctx)
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
