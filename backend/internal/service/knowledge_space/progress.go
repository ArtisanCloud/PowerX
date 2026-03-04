package knowledge_space

import (
	"context"
	"time"
)

// IngestionProgressUpdate describes a progress snapshot for a job.
type IngestionProgressUpdate struct {
	TenantUUID   string    `json:"tenant_uuid"`
	SpaceUUID    string    `json:"space_uuid"`
	JobUUID      string    `json:"job_uuid"`
	Status       string    `json:"status"`
	Stage        string    `json:"stage"`
	Progress     int       `json:"progress"`
	ChunkTotal   int       `json:"chunk_total,omitempty"`
	EmbeddingPct float64   `json:"embedding_pct,omitempty"`
	MaskingPct   float64   `json:"masking_pct,omitempty"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// IngestionProgressPublisher provides a hook to publish progress updates.
type IngestionProgressPublisher interface {
	PublishIngestionProgress(ctx context.Context, update IngestionProgressUpdate)
}

// IngestionProgressPublisherFunc adapts a function into a publisher.
type IngestionProgressPublisherFunc func(ctx context.Context, update IngestionProgressUpdate)

func (f IngestionProgressPublisherFunc) PublishIngestionProgress(ctx context.Context, update IngestionProgressUpdate) {
	if f != nil {
		f(ctx, update)
	}
}
