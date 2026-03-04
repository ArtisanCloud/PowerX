package knowledge_space

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/google/uuid"
)

// CorpusCheckInput captures scheduling metadata.
type CorpusCheckInput struct {
	JobUUID     uuid.UUID
	SpaceID     uuid.UUID
	RequestedBy string
}

// CorpusCheckTask contains scheduling details.
type CorpusCheckTask struct {
	JobUUID      uuid.UUID
	Status       string
	ScheduledAt  time.Time
	RollbackHint string
}

// CorpusCheckPipeline defines scheduling behavior.
type CorpusCheckPipeline interface {
	Schedule(ctx context.Context, input CorpusCheckInput) (CorpusCheckTask, error)
}

type CorpusCheckPipelineOptions struct {
	EventBus   event_bus.EventBus
	EventTopic string
	Clock      func() time.Time
}

type defaultCorpusCheckPipeline struct {
	bus        event_bus.EventBus
	eventTopic string
	mu         sync.Mutex
	clock      func() time.Time
}

func NewCorpusCheckPipeline(opts CorpusCheckPipelineOptions) CorpusCheckPipeline {
	if opts.Clock == nil {
		opts.Clock = time.Now
	}
	topic := strings.TrimSpace(opts.EventTopic)
	if topic == "" {
		topic = "knowledge.corpus_check.run"
	}
	return &defaultCorpusCheckPipeline{bus: opts.EventBus, eventTopic: topic, clock: opts.Clock}
}

func (p *defaultCorpusCheckPipeline) Schedule(ctx context.Context, input CorpusCheckInput) (CorpusCheckTask, error) {
	task := CorpusCheckTask{
		JobUUID:     input.JobUUID,
		Status:      "scheduled",
		ScheduledAt: p.clock(),
	}
	if p.bus != nil {
		payload := map[string]any{
			"job_uuid":     input.JobUUID.String(),
			"space_id":     input.SpaceID.String(),
			"requestedBy":  input.RequestedBy,
			"scheduled_at": task.ScheduledAt.UTC().Format(time.RFC3339Nano),
		}
		p.bus.Publish(p.eventTopic, payload, ctx)
	}
	return task, nil
}

