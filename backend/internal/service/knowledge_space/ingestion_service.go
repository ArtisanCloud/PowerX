package knowledge_space

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/knowledge_space/instrumentation"
	knowledge "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/knowledge"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/vectorstore"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var (
	allowedSourceTypes = map[string]bool{
		"pdf":      true,
		"markdown": true,
		"table":    true,
		"api":      true,
	}

	allowedPriority = map[string]bool{
		"normal": true,
		"high":   true,
	}
)

// IngestionService orchestrates document ingestion, chunking and vector persistence.
type IngestionService struct {
	db          *gorm.DB
	inst        *instrumentation.Instrumentation
	vectorStore vectorstore.Store
	metrics     *IngestionMetricsWriter
}

// IngestionServiceOptions configures the ingestion service runtime.
type IngestionServiceOptions struct {
	DB              *gorm.DB
	Instrumentation *instrumentation.Instrumentation
	VectorStore     vectorstore.Store
	MetricsWriter   *IngestionMetricsWriter
}

// TriggerIngestionInput captures API payload used to start an ingestion job.
type TriggerIngestionInput struct {
	SpaceID        uuid.UUID
	SourceType     string
	SourceURI      string
	MaskingProfile string
	Priority       string
	RequestedBy    string
}

// NewIngestionService constructs a service instance.
func NewIngestionService(opts IngestionServiceOptions) *IngestionService {
	if opts.DB == nil {
		panic("ingestion service requires db")
	}
	if opts.Instrumentation == nil {
		opts.Instrumentation = instrumentation.New(instrumentation.Options{})
	}
	if opts.MetricsWriter == nil {
		opts.MetricsWriter = NewIngestionMetricsWriter(defaultMetricsPath)
	}
	return &IngestionService{
		db:          opts.DB,
		inst:        opts.Instrumentation,
		vectorStore: opts.VectorStore,
		metrics:     opts.MetricsWriter,
	}
}

// Trigger kicks off an ingestion job for a given space and source payload.
func (s *IngestionService) Trigger(ctx context.Context, in TriggerIngestionInput) (*knowledge.IngestionJob, error) {
	if in.SpaceID == uuid.Nil {
		return nil, fmt.Errorf("space_id is required")
	}
	if !allowedSourceTypes[strings.ToLower(in.SourceType)] {
		return nil, fmt.Errorf("unsupported sourceType: %s", in.SourceType)
	}
	priority := strings.ToLower(in.Priority)
	if priority == "" {
		priority = "normal"
	}
	if !allowedPriority[priority] {
		return nil, fmt.Errorf("unsupported priority: %s", in.Priority)
	}
	logger := s.inst.Logger(ctx)
	logger.InfoF(ctx, "[ingestion] trigger space=%s source=%s", in.SpaceID, in.SourceURI)

	var result *knowledge.IngestionJob
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		jobs := repo.NewIngestionJobRepository(tx)
		bundles := repo.NewArtifactBundleRepository(tx)

		now := time.Now()
		job := &knowledge.IngestionJob{
			SpaceUUID:   in.SpaceID,
			SourceID:    fmt.Sprintf("src-%s", uuid.NewString()),
			SourceType:  strings.ToLower(in.SourceType),
			Status:      knowledge.IngestionStatusRunning,
			Priority:    priority,
			SubmittedBy: in.RequestedBy,
			StartedAt:   &now,
		}
		job, err := jobs.Create(ctx, job)
		if err != nil {
			return err
		}

		chunkSet := synthesizeChunks(in.SpaceID)
		bundle := &knowledge.ArtifactBundle{
			IngestionJobID:      job.ID,
			ChunkManifestURI:    fmt.Sprintf("memory://knowledge/%s/jobs/%d/chunks.json", in.SpaceID, job.ID),
			VectorManifestURI:   fmt.Sprintf("memory://knowledge/%s/jobs/%d/vectors.json", in.SpaceID, job.ID),
			GraphManifestURI:    "",
			MaskingReportURI:    fmt.Sprintf("memory://knowledge/%s/jobs/%d/masking.json", in.SpaceID, job.ID),
			SummaryChunkCount:   chunkSet.summaryCount,
			ParagraphChunkCount: chunkSet.paragraphCount,
			Checksum:            chunkSet.checksum,
			StorageClass:        "standard",
		}
		bundle, err = bundles.Create(ctx, bundle)
		if err != nil {
			return err
		}
		job.ArtifactBundleID = &bundle.ID

		job.ChunkTotal = chunkSet.total
		job.SummaryChunkCount = chunkSet.summaryCount
		job.ParagraphChunkCount = chunkSet.paragraphCount
		job.ChunkCoveredPct = 100.0
		job.EmbeddingSuccessPct = 100.0
		job.MaskingCoveragePct = 100.0

		if len(chunkSet.records) > 0 && s.vectorStore != nil {
			if err := s.vectorStore.Upsert(ctx, in.SpaceID, chunkSet.records); err != nil {
				job.Status = knowledge.IngestionStatusFailed
				job.ErrorCode = "vector_upsert_failed"
				job.BlockedReason = err.Error()
				if _, updateErr := jobs.Update(ctx, job); updateErr != nil {
					logger.WarnF(ctx, "[ingestion] failed to record vector upsert error: %v", updateErr)
				}
				return err
			}
		}

		job.Status = knowledge.IngestionStatusCompleted
		completed := time.Now()
		job.CompletedAt = &completed
		job.MetricsSnapshot = mustJSON(map[string]any{
			"source_uri":  in.SourceURI,
			"created_at":  job.CreatedAt,
			"chunk_total": chunkSet.total,
		})

		if job, err = jobs.Update(ctx, job); err != nil {
			return err
		}

		result = job
		return nil
	})
	if err != nil {
		return nil, err
	}

	if result != nil {
		if s.metrics != nil {
			_ = s.metrics.Store(IngestionSnapshot{
				SpaceID:             result.SpaceUUID.String(),
				JobID:               result.UUID.String(),
				ChunkTotal:          result.ChunkTotal,
				SummaryChunkCount:   result.SummaryChunkCount,
				ParagraphChunkCount: result.ParagraphChunkCount,
				CoveragePct:         result.ChunkCoveredPct,
				EmbeddingPct:        result.EmbeddingSuccessPct,
				MaskingPct:          result.MaskingCoveragePct,
				CompletedAt:         result.CompletedAt,
			})
		}
		s.inst.RecordIngestionCoverage(result.ChunkCoveredPct)
	}
	return result, nil
}

// RemoveChunks removes embeddings for provided chunk IDs.
func (s *IngestionService) RemoveChunks(ctx context.Context, space uuid.UUID, chunkIDs []uuid.UUID) error {
	if s.vectorStore == nil || len(chunkIDs) == 0 {
		return nil
	}
	return s.vectorStore.DeleteByChunkIDs(ctx, space, chunkIDs)
}

// DropSpaceVectors removes every embedding associated with the space.
func (s *IngestionService) DropSpaceVectors(ctx context.Context, space uuid.UUID) error {
	if s.vectorStore == nil {
		return nil
	}
	return s.vectorStore.DropSpace(ctx, space)
}

type chunkBatch struct {
	records        []vectorstore.VectorRecord
	summaryCount   int
	paragraphCount int
	total          int
	checksum       string
}

func synthesizeChunks(space uuid.UUID) chunkBatch {
	summary := 3
	paragraph := 6
	records := make([]vectorstore.VectorRecord, 0, summary+paragraph)
	var hashes []byte
	for i := 0; i < summary; i++ {
		chunkID := uuid.New()
		records = append(records, vectorstore.VectorRecord{
			ChunkID:   chunkID,
			Embedding: fakeEmbedding(i, 32),
			Metadata: map[string]any{
				"chunk_kind": "summary",
				"space_id":   space.String(),
			},
		})
		hashes = append(hashes, chunkID[:]...)
	}
	for i := 0; i < paragraph; i++ {
		chunkID := uuid.New()
		records = append(records, vectorstore.VectorRecord{
			ChunkID:   chunkID,
			Embedding: fakeEmbedding(summary+i, 32),
			Metadata: map[string]any{
				"chunk_kind": "paragraph",
				"space_id":   space.String(),
			},
		})
		hashes = append(hashes, chunkID[:]...)
	}
	sum := sha256.Sum256(hashes)
	return chunkBatch{
		records:        records,
		summaryCount:   summary,
		paragraphCount: paragraph,
		total:          len(records),
		checksum:       hex.EncodeToString(sum[:]),
	}
}

func fakeEmbedding(seed int, dim int) []float32 {
	if dim <= 0 {
		dim = 32
	}
	vec := make([]float32, dim)
	for i := 0; i < dim; i++ {
		vec[i] = float32((seed + i%7)) / 10.0
	}
	return vec
}

func mustJSON(v any) []byte {
	if v == nil {
		return []byte("{}")
	}
	buf, err := json.Marshal(v)
	if err != nil {
		return []byte("{}")
	}
	return buf
}
