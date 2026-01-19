package knowledge_space

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/knowledge_space/corpus_check"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/knowledge"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type CorpusCheckWorkerOptions struct {
	DB         *gorm.DB
	EventBus   event_bus.EventBus
	EventTopic string
	Clock      func() time.Time
}

type CorpusCheckWorker struct {
	db         *gorm.DB
	bus        event_bus.EventBus
	eventTopic string
	clock      func() time.Time
}

func NewCorpusCheckWorker(opts CorpusCheckWorkerOptions) *CorpusCheckWorker {
	if opts.Clock == nil {
		opts.Clock = time.Now
	}
	topic := strings.TrimSpace(opts.EventTopic)
	if topic == "" {
		topic = "knowledge.corpus_check.run"
	}
	return &CorpusCheckWorker{db: opts.DB, bus: opts.EventBus, eventTopic: topic, clock: opts.Clock}
}

func (w *CorpusCheckWorker) Start() (unsubscribe func()) {
	if w == nil || w.db == nil || w.bus == nil {
		return func() {}
	}
	return w.bus.Subscribe(w.eventTopic, func(evt event_bus.Event) error {
		return w.handle(evt)
	})
}

func (w *CorpusCheckWorker) handle(evt event_bus.Event) error {
	payloadBytes, err := json.Marshal(evt.Payload)
	if err != nil {
		return err
	}
	var raw corpusCheckPayload
	if err := json.Unmarshal(payloadBytes, &raw); err != nil {
		return err
	}
	jobUUID, err := uuid.Parse(strings.TrimSpace(raw.JobUUID))
	if err != nil {
		return err
	}
	spaceID, err := uuid.Parse(strings.TrimSpace(raw.SpaceID))
	if err != nil {
		return err
	}
	return w.run(evt.Ctx, CorpusCheckInput{
		JobUUID:     jobUUID,
		SpaceID:     spaceID,
		RequestedBy: raw.RequestedBy,
	})
}

func (w *CorpusCheckWorker) run(ctx context.Context, input CorpusCheckInput) error {
	if w == nil || w.db == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	now := w.clock().UTC()
	return w.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		jobs := repo.NewCorpusCheckJobRepository(tx)
		job, err := jobs.FindByUUID(ctx, input.JobUUID)
		if err != nil {
			return err
		}
		if job == nil || job.SpaceUUID != input.SpaceID {
			return nil
		}
		if job.Status == models.CorpusCheckStatusCompleted {
			return nil
		}
		job.Status = models.CorpusCheckStatusRunning
		job.StartedAt = &now
		job.ErrorReason = ""
		job.UpdatedAt = now
		if _, err := jobs.Update(ctx, job); err != nil {
			return err
		}
		var sample []models.IngestionJob
		if err := tx.Model(&models.IngestionJob{}).
			Where("space_uuid = ?", input.SpaceID).
			Order("id DESC").
			Limit(50).
			Find(&sample).Error; err != nil {
			return err
		}
		metrics, recs := corpus_check.BuildMetrics(sample)
		job.Metrics = metrics
		job.Recommendations = recs
		job.SampleJobUUIDs = sampleJobUUIDs(sample)
		doneAt := w.clock().UTC()
		job.Status = models.CorpusCheckStatusCompleted
		job.CompletedAt = &doneAt
		job.UpdatedAt = doneAt
		_, err = jobs.Update(ctx, job)
		return err
	})
}

func sampleJobUUIDs(sample []models.IngestionJob) datatypes.JSON {
	out := make([]string, 0, len(sample))
	for _, j := range sample {
		if j.UUID != uuid.Nil {
			out = append(out, j.UUID.String())
		}
	}
	bytes, _ := json.Marshal(out)
	if len(bytes) == 0 {
		return datatypes.JSON([]byte("[]"))
	}
	return datatypes.JSON(bytes)
}

