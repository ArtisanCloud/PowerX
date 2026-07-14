package knowledge_space

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	agentSvc "github.com/ArtisanCloud/PowerX/internal/service/agent"
	"github.com/ArtisanCloud/PowerX/internal/service/knowledge_space/instrumentation"
	knowledge "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/knowledge"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/vectorstore"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var (
	allowedFormats = map[string]bool{
		"pdf":      true,
		"docx":     true,
		"xlsx":     true,
		"csv":      true,
		"markdown": true,
		"html":     true,
		"sql":      true,
		"image":    true,
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

	processors    *ProcessorRegistry
	artifactStore *ArtifactStore
	maxRetries    int

	agentSettings     *agentSvc.AgentSettingService
	vectorDimension   int
	progressPublisher IngestionProgressPublisher
}

func (s *IngestionService) GetJob(ctx context.Context, spaceID uuid.UUID, jobUUID uuid.UUID) (*knowledge.IngestionJob, error) {
	if spaceID == uuid.Nil || jobUUID == uuid.Nil {
		return nil, ErrInvalidInput
	}
	j, err := repo.NewIngestionJobRepository(s.db).FindByUUID(ctx, jobUUID)
	if err != nil {
		return nil, err
	}
	if j == nil || j.SpaceUUID != spaceID {
		return nil, nil
	}
	return j, nil
}

func (s *IngestionService) ListJobs(ctx context.Context, spaceID uuid.UUID, limit int) ([]knowledge.IngestionJob, error) {
	if spaceID == uuid.Nil {
		return nil, ErrInvalidInput
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}
	var jobs []knowledge.IngestionJob
	err := s.db.WithContext(ctx).
		Where("space_uuid = ?", spaceID).
		Order("created_at DESC").
		Limit(limit).
		Find(&jobs).Error
	return jobs, err
}

// IngestionServiceOptions configures the ingestion service runtime.
type IngestionServiceOptions struct {
	DB              *gorm.DB
	Instrumentation *instrumentation.Instrumentation
	VectorStore     vectorstore.Store
	MetricsWriter   *IngestionMetricsWriter

	Processors    *ProcessorRegistry
	ArtifactStore *ArtifactStore
	MaxRetries    int

	AgentSettings     *agentSvc.AgentSettingService
	VectorDimension   int
	ProgressPublisher IngestionProgressPublisher
}

// TriggerIngestionInput captures API payload used to start an ingestion job.
type TriggerIngestionInput struct {
	SpaceID uuid.UUID
	// Format is the preferred field. SourceType is kept for backward compatibility.
	Format           string
	SourceType       string
	SourceURI        string
	IngestionProfile string
	ProcessorProfile string
	OCRRequired      bool
	MaskingProfile   string
	Priority         string
	RequestedBy      string
	DocUUID          string
	// L1/L2/L3 snapshot (best-effort).
	RagSceneKey         string
	RagBundleKey        string
	RagPrimary          string
	SegmentMode         string
	ChunkSize           int
	ChunkOverlap        int
	SegmentSizePolicy   string
	SegmentOrder        []string
	Separators          []string
	PagePriority        bool
	AnchorHeadingPath   bool
	AnchorClauseID      bool
	AnchorRowNumber     bool
	AnchorSpeaker       bool
	AnchorSentenceIndex bool
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
		opts.MetricsWriter = NewIngestionMetricsWriter("")
	}
	if opts.Processors == nil {
		opts.Processors = NewProcessorRegistry()
	}
	if opts.ArtifactStore == nil {
		opts.ArtifactStore = NewArtifactStore(ArtifactStoreOptions{})
	}
	maxRetries := opts.MaxRetries
	if maxRetries < 0 {
		maxRetries = 0
	}
	return &IngestionService{
		db:            opts.DB,
		inst:          opts.Instrumentation,
		vectorStore:   opts.VectorStore,
		metrics:       opts.MetricsWriter,
		processors:    opts.Processors,
		artifactStore: opts.ArtifactStore,
		maxRetries:    maxRetries,
		agentSettings: opts.AgentSettings,
		vectorDimension: func() int {
			if opts.VectorDimension > 0 {
				return opts.VectorDimension
			}
			return 0
		}(),
		progressPublisher: opts.ProgressPublisher,
	}
}

// Trigger kicks off an ingestion job for a given space and source payload.
func (s *IngestionService) Trigger(ctx context.Context, in TriggerIngestionInput) (*knowledge.IngestionJob, error) {
	if in.SpaceID == uuid.Nil || strings.TrimSpace(in.SourceURI) == "" {
		return nil, ErrInvalidInput
	}
	format := strings.ToLower(strings.TrimSpace(in.Format))
	if format == "" {
		format = strings.ToLower(strings.TrimSpace(in.SourceType))
	}
	if !allowedFormats[format] {
		return nil, ErrInvalidInput
	}
	priority := strings.ToLower(strings.TrimSpace(in.Priority))
	if priority == "" {
		priority = "normal"
	}
	if !allowedPriority[priority] {
		return nil, ErrInvalidInput
	}

	logger := s.inst.Logger(ctx)
	logger.InfoF(ctx, "[ingestion] trigger space=%s source=%s format=%s", in.SpaceID, in.SourceURI, format)

	spaceRepo := repo.NewKnowledgeSpaceRepository(s.db)
	space, err := spaceRepo.FindByUUID(ctx, in.SpaceID)
	if err != nil {
		return nil, err
	}
	if space == nil || space.Status == knowledge.KnowledgeSpaceStatusRetired {
		return nil, ErrSpaceNotFound
	}
	if err := s.ensureEmbeddingReady(ctx, space); err != nil {
		return nil, err
	}

	now := time.Now()
	job := &knowledge.IngestionJob{
		SpaceUUID:   in.SpaceID,
		SourceID:    stableSourceID(in.SourceURI),
		SourceType:  format,
		Status:      knowledge.IngestionStatusRunning,
		Priority:    priority,
		SubmittedBy: in.RequestedBy,
		StartedAt:   &now,
	}
	bundle := &knowledge.ArtifactBundle{
		IngestionJobID:    0,
		ChunkManifestURI:  "minio://powerx-knowledge/pending/chunks.json",
		VectorManifestURI: "minio://powerx-knowledge/pending/vectors.json",
		MaskingReportURI:  "minio://powerx-knowledge/pending/masking.json",
		Checksum:          strings.Repeat("0", 64),
		StorageClass:      "standard",
		Status:            knowledge.ArtifactBundleStatusActive,
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		jobs := repo.NewIngestionJobRepository(tx)
		bundles := repo.NewArtifactBundleRepository(tx)

		createdJob, err := jobs.Create(ctx, job)
		if err != nil {
			return err
		}

		bundle.IngestionJobID = createdJob.ID
		createdBundle, err := bundles.Create(ctx, bundle)
		if err != nil {
			return err
		}
		createdJob.ArtifactBundleID = &createdBundle.ID
		updatedJob, err := jobs.Update(ctx, createdJob)
		if err != nil {
			return err
		}
		job = updatedJob
		bundle = createdBundle
		return nil
	})
	if err != nil {
		return nil, err
	}

	outcome, chunks, vectorRecords, ocrArtifacts := s.runPipeline(ctx, pipelineInput{
		space:               space,
		job:                 job,
		bundle:              bundle,
		format:              format,
		sourceURI:           in.SourceURI,
		docUUID:             in.DocUUID,
		ingestionProfile:    in.IngestionProfile,
		processorProfile:    in.ProcessorProfile,
		ocrRequired:         in.OCRRequired,
		maskingProfile:      in.MaskingProfile,
		ragSceneKey:         strings.TrimSpace(in.RagSceneKey),
		ragBundleKey:        strings.TrimSpace(in.RagBundleKey),
		ragPrimary:          strings.TrimSpace(in.RagPrimary),
		segmentMode:         in.SegmentMode,
		chunkSize:           in.ChunkSize,
		chunkOverlap:        in.ChunkOverlap,
		segmentSizePolicy:   in.SegmentSizePolicy,
		segmentOrder:        in.SegmentOrder,
		separators:          in.Separators,
		anchorHeadingPath:   in.AnchorHeadingPath,
		anchorClauseID:      in.AnchorClauseID,
		anchorRowNumber:     in.AnchorRowNumber,
		anchorSpeaker:       in.AnchorSpeaker,
		anchorSentenceIndex: in.AnchorSentenceIndex,
	})
	return s.finalizeIngestion(ctx, space, job, bundle, format, in, outcome, chunks, vectorRecords, ocrArtifacts)
}

// TriggerWithDocUnits runs ingestion using already-normalized document units (e.g. API connectors).
// This is used by SpaceSyncJob runners so external sources can reuse the same chunking/masking/vectorstore pipeline.
func (s *IngestionService) TriggerWithDocUnits(ctx context.Context, in TriggerIngestionInput, docUnits []DocumentUnit) (*knowledge.IngestionJob, error) {
	if in.SpaceID == uuid.Nil || strings.TrimSpace(in.SourceURI) == "" {
		return nil, ErrInvalidInput
	}
	format := strings.ToLower(strings.TrimSpace(in.Format))
	if format == "" {
		format = strings.ToLower(strings.TrimSpace(in.SourceType))
	}
	if !allowedFormats[format] {
		return nil, ErrInvalidInput
	}
	priority := strings.ToLower(strings.TrimSpace(in.Priority))
	if priority == "" {
		priority = "normal"
	}
	if !allowedPriority[priority] {
		return nil, ErrInvalidInput
	}

	logger := s.inst.Logger(ctx)
	logger.InfoF(ctx, "[ingestion] trigger(units) space=%s source=%s format=%s units=%d", in.SpaceID, in.SourceURI, format, len(docUnits))

	spaceRepo := repo.NewKnowledgeSpaceRepository(s.db)
	space, err := spaceRepo.FindByUUID(ctx, in.SpaceID)
	if err != nil {
		return nil, err
	}
	if space == nil || space.Status == knowledge.KnowledgeSpaceStatusRetired {
		return nil, ErrSpaceNotFound
	}
	if err := s.ensureEmbeddingReady(ctx, space); err != nil {
		return nil, err
	}

	now := time.Now()
	job := &knowledge.IngestionJob{
		SpaceUUID:   in.SpaceID,
		SourceID:    stableSourceID(in.SourceURI),
		SourceType:  format,
		Status:      knowledge.IngestionStatusRunning,
		Priority:    priority,
		SubmittedBy: in.RequestedBy,
		StartedAt:   &now,
	}
	bundle := &knowledge.ArtifactBundle{
		IngestionJobID:    0,
		ChunkManifestURI:  "minio://powerx-knowledge/pending/chunks.json",
		VectorManifestURI: "minio://powerx-knowledge/pending/vectors.json",
		MaskingReportURI:  "minio://powerx-knowledge/pending/masking.json",
		Checksum:          strings.Repeat("0", 64),
		StorageClass:      "standard",
		Status:            knowledge.ArtifactBundleStatusActive,
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		jobs := repo.NewIngestionJobRepository(tx)
		bundles := repo.NewArtifactBundleRepository(tx)

		createdJob, err := jobs.Create(ctx, job)
		if err != nil {
			return err
		}
		bundle.IngestionJobID = createdJob.ID
		createdBundle, err := bundles.Create(ctx, bundle)
		if err != nil {
			return err
		}
		createdJob.ArtifactBundleID = &createdBundle.ID
		updatedJob, err := jobs.Update(ctx, createdJob)
		if err != nil {
			return err
		}
		job = updatedJob
		bundle = createdBundle
		return nil
	})
	if err != nil {
		return nil, err
	}

	outcome, chunks, vectorRecords := s.runPipelineFromUnits(ctx, pipelineUnitsInput{
		space:               space,
		job:                 job,
		bundle:              bundle,
		format:              format,
		sourceURI:           in.SourceURI,
		docUUID:             in.DocUUID,
		docUnits:            docUnits,
		maskingProfile:      in.MaskingProfile,
		ocrRequired:         in.OCRRequired,
		ragSceneKey:         strings.TrimSpace(in.RagSceneKey),
		ragBundleKey:        strings.TrimSpace(in.RagBundleKey),
		ragPrimary:          strings.TrimSpace(in.RagPrimary),
		segmentMode:         in.SegmentMode,
		chunkSize:           in.ChunkSize,
		chunkOverlap:        in.ChunkOverlap,
		segmentSizePolicy:   in.SegmentSizePolicy,
		segmentOrder:        in.SegmentOrder,
		separators:          in.Separators,
		pagePriority:        in.PagePriority,
		anchorHeadingPath:   in.AnchorHeadingPath,
		anchorClauseID:      in.AnchorClauseID,
		anchorRowNumber:     in.AnchorRowNumber,
		anchorSpeaker:       in.AnchorSpeaker,
		anchorSentenceIndex: in.AnchorSentenceIndex,
	})
	return s.finalizeIngestion(ctx, space, job, bundle, format, in, outcome, chunks, vectorRecords, nil)
}

// TriggerAsync creates an ingestion job and runs the pipeline in background.
// It is intended for HTTP/API usage so UI can poll job status asynchronously.
func (s *IngestionService) TriggerAsync(ctx context.Context, in TriggerIngestionInput) (*knowledge.IngestionJob, error) {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("POWERX_INGESTION_SYNC")), "1") ||
		strings.EqualFold(strings.TrimSpace(os.Getenv("POWERX_INGESTION_SYNC")), "true") {
		return s.Trigger(ctx, in)
	}
	if in.SpaceID == uuid.Nil || strings.TrimSpace(in.SourceURI) == "" {
		return nil, ErrInvalidInput
	}
	format := strings.ToLower(strings.TrimSpace(in.Format))
	if format == "" {
		format = strings.ToLower(strings.TrimSpace(in.SourceType))
	}
	if !allowedFormats[format] {
		return nil, ErrInvalidInput
	}
	priority := strings.ToLower(strings.TrimSpace(in.Priority))
	if priority == "" {
		priority = "normal"
	}
	if !allowedPriority[priority] {
		return nil, ErrInvalidInput
	}

	spaceRepo := repo.NewKnowledgeSpaceRepository(s.db)
	space, err := spaceRepo.FindByUUID(ctx, in.SpaceID)
	if err != nil {
		return nil, err
	}
	if space == nil || space.Status == knowledge.KnowledgeSpaceStatusRetired {
		return nil, ErrSpaceNotFound
	}
	if err := s.ensureEmbeddingReady(ctx, space); err != nil {
		return nil, err
	}

	job := &knowledge.IngestionJob{
		SpaceUUID:   in.SpaceID,
		SourceID:    stableSourceID(in.SourceURI),
		SourceType:  format,
		Status:      knowledge.IngestionStatusPending,
		Priority:    priority,
		SubmittedBy: in.RequestedBy,
	}
	bundle := &knowledge.ArtifactBundle{
		IngestionJobID:    0,
		ChunkManifestURI:  "minio://powerx-knowledge/pending/chunks.json",
		VectorManifestURI: "minio://powerx-knowledge/pending/vectors.json",
		MaskingReportURI:  "minio://powerx-knowledge/pending/masking.json",
		Checksum:          strings.Repeat("0", 64),
		StorageClass:      "standard",
		Status:            knowledge.ArtifactBundleStatusActive,
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		jobs := repo.NewIngestionJobRepository(tx)
		bundles := repo.NewArtifactBundleRepository(tx)

		createdJob, err := jobs.Create(ctx, job)
		if err != nil {
			return err
		}
		bundle.IngestionJobID = createdJob.ID
		createdBundle, err := bundles.Create(ctx, bundle)
		if err != nil {
			return err
		}
		createdJob.ArtifactBundleID = &createdBundle.ID
		updatedJob, err := jobs.Update(ctx, createdJob)
		if err != nil {
			return err
		}
		job = updatedJob
		bundle = createdBundle
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Run ingestion in background. Do not inherit request context cancellation.
	go func() {
		bg := context.Background()
		logger := s.inst.Logger(bg)
		logger.InfoF(bg, "[ingestion] async start space=%s job=%s source=%s format=%s", in.SpaceID, job.UUID, in.SourceURI, format)
		// Mark running early so UI doesn't stay in pending while processing.
		now := time.Now()
		job.Status = knowledge.IngestionStatusRunning
		job.StartedAt = &now
		_, _ = repo.NewIngestionJobRepository(s.db).Update(bg, job)
		s.emitProgress(bg, job, "start", 0, 0, 0, 0, space.TenantUUID)
		// Run the same pipeline and update job in DB.
		outcome, chunks, vectors, ocrArtifacts := s.runPipeline(bg, pipelineInput{
			space:               space,
			job:                 job,
			bundle:              bundle,
			format:              format,
			sourceURI:           in.SourceURI,
			docUUID:             in.DocUUID,
			ingestionProfile:    in.IngestionProfile,
			processorProfile:    in.ProcessorProfile,
			ocrRequired:         in.OCRRequired,
			maskingProfile:      in.MaskingProfile,
			ragSceneKey:         strings.TrimSpace(in.RagSceneKey),
			ragBundleKey:        strings.TrimSpace(in.RagBundleKey),
			ragPrimary:          strings.TrimSpace(in.RagPrimary),
			segmentMode:         in.SegmentMode,
			chunkSize:           in.ChunkSize,
			chunkOverlap:        in.ChunkOverlap,
			segmentSizePolicy:   in.SegmentSizePolicy,
			segmentOrder:        in.SegmentOrder,
			separators:          in.Separators,
			pagePriority:        in.PagePriority,
			anchorHeadingPath:   in.AnchorHeadingPath,
			anchorClauseID:      in.AnchorClauseID,
			anchorRowNumber:     in.AnchorRowNumber,
			anchorSpeaker:       in.AnchorSpeaker,
			anchorSentenceIndex: in.AnchorSentenceIndex,
		})
		if _, err := s.finalizeIngestion(bg, space, job, bundle, format, in, outcome, chunks, vectors, ocrArtifacts); err != nil {
			logger.ErrorF(bg, "[ingestion] async finalize failed job=%s err=%v", job.UUID, err)
		}
	}()

	return job, nil
}

var ErrIngestionDegraded = errors.New("ingestion degraded")

func (s *IngestionService) finalizeIngestion(
	ctx context.Context,
	space *knowledge.KnowledgeSpace,
	job *knowledge.IngestionJob,
	bundle *knowledge.ArtifactBundle,
	format string,
	in TriggerIngestionInput,
	outcome pipelineOutcome,
	chunks []IngestionChunk,
	vectorRecords []vectorstore.VectorRecord,
	ocrArtifacts *OCRArtifacts,
) (*knowledge.IngestionJob, error) {
	if job == nil {
		return nil, ErrInvalidInput
	}
	if job.StartedAt == nil {
		now := time.Now()
		job.StartedAt = &now
	}
	if strings.TrimSpace(job.Status) == "" || job.Status == knowledge.IngestionStatusPending {
		job.Status = knowledge.IngestionStatusRunning
		_, _ = repo.NewIngestionJobRepository(s.db).Update(ctx, job)
	}

	s.writeIngestionSegmentLog(format, in, outcome, chunks, job)

	// Persist online chunk store (best-effort). This is the editable truth source for chunk text + metadata.
	// If the chunk store is not enabled in this environment, ingestion should continue (manifest remains available).
	if len(chunks) > 0 {
		now := time.Now()
		rows := make([]knowledge.KnowledgeChunk, 0, len(chunks))
		for i := range chunks {
			ch := &chunks[i]
			meta := make(map[string]any, len(ch.Metadata)+3)
			for k, v := range ch.Metadata {
				meta[k] = v
			}
			meta["job_uuid"] = job.UUID.String()
			meta["masked"] = ch.Masked
			meta["confidence"] = ch.Confidence

			metaBytes, err := json.Marshal(meta)
			if err != nil {
				metaBytes = []byte(`{}`)
			}
			rows = append(rows, knowledge.KnowledgeChunk{
				SpaceUUID: in.SpaceID,
				ChunkUUID: ch.ID,
				JobUUID:   &job.UUID,
				Kind:      ch.Kind,
				Content:   ch.Content,
				Metadata:  metaBytes,
				CreatedAt: now,
				UpdatedAt: now,
			})
			// Reflect back to in-memory chunk metadata so manifests and vector metadata stay aligned.
			ch.Metadata = meta
		}
		if err := repo.NewKnowledgeChunkRepository(s.db).UpsertMany(ctx, rows); err != nil {
			if !isUndefinedTableError(err) {
				if s.inst != nil {
					s.inst.Logger(ctx).WarnF(ctx, "[ingestion] upsert knowledge_chunks failed: %v", err)
				}
			}
		}
	}

	if s.artifactStore != nil && bundle != nil {
		if artifactUpdate, err := s.artifactStore.Write(ctx, ArtifactWriteInput{
			SpaceID:        in.SpaceID,
			JobUUID:        job.UUID,
			JobID:          job.ID,
			Format:         format,
			SourceURI:      in.SourceURI,
			Chunks:         chunks,
			VectorRecords:  vectorRecords,
			MaskingProfile: in.MaskingProfile,
			Outcome:        outcome,
			OCRArtifacts:   ocrArtifacts,
		}); err == nil {
			bundle.ChunkManifestURI = artifactUpdate.ChunkManifestURI
			bundle.VectorManifestURI = artifactUpdate.VectorManifestURI
			bundle.MaskingReportURI = artifactUpdate.MaskingReportURI
			bundle.OCRPageImagesURI = artifactUpdate.OCRPageImagesURI
			bundle.OCRRawManifestURI = artifactUpdate.OCRRawManifestURI
			bundle.OCRSearchablePDFURI = artifactUpdate.OCRSearchablePDFURI
			bundle.Checksum = artifactUpdate.Checksum
			bundle.SummaryChunkCount = outcome.summaryCount
			bundle.ParagraphChunkCount = outcome.chunkCount
			_, _ = repo.NewArtifactBundleRepository(s.db).Update(ctx, bundle)
		}
	}

	vectorErr := s.persistWithRetry(ctx, in.SpaceID, vectorRecords, job, outcome, space.TenantUUID)
	s.emitProgress(ctx, job, "persist", 95, outcome.totalChunks, outcome.embeddingPct, outcome.maskingPct, space.TenantUUID)
	if errors.Is(vectorErr, ErrVectorIndexNotActivated) && !outcome.degraded {
		outcome.degraded = true
		if strings.TrimSpace(outcome.errorCode) == "" {
			outcome.errorCode = "vector_index_not_activated"
		}
		if strings.TrimSpace(outcome.reason) == "" {
			outcome.reason = "no_active_vector_index"
		}
	}
	completed := time.Now()
	job.CompletedAt = &completed

	job.ChunkTotal = outcome.totalChunks
	job.SummaryChunkCount = outcome.summaryCount
	job.ParagraphChunkCount = outcome.chunkCount
	job.ChunkCoveredPct = outcome.coveragePct
	job.MaskingCoveragePct = outcome.maskingPct
	job.EmbeddingSuccessPct = outcome.embeddingPct

	if outcome.status == knowledge.IngestionStatusBlocked {
		job.Status = knowledge.IngestionStatusBlocked
		job.ErrorCode = outcome.errorCode
		job.BlockedReason = outcome.reason
		job.EmbeddingSuccessPct = 0
		job.MetricsSnapshot = mustJSON(outcome.snapshot(completed))
		_, _ = repo.NewIngestionJobRepository(s.db).Update(ctx, job)
		s.emitProgress(ctx, job, "finalize", 100, outcome.totalChunks, outcome.embeddingPct, outcome.maskingPct, space.TenantUUID)
		s.emitMetrics(job, outcome)
		return job, nil
	}

	if errors.Is(vectorErr, ErrVectorIndexNotActivated) {
		job.Status = knowledge.IngestionStatusBlocked
		job.ErrorCode = outcome.errorCode
		if strings.TrimSpace(job.ErrorCode) == "" {
			job.ErrorCode = "vector_index_not_activated"
		}
		job.BlockedReason = outcome.reason
		if strings.TrimSpace(job.BlockedReason) == "" {
			job.BlockedReason = "no_active_vector_index"
		}
		job.EmbeddingSuccessPct = 0
		job.MetricsSnapshot = mustJSON(outcome.snapshot(completed))
		_, _ = repo.NewIngestionJobRepository(s.db).Update(ctx, job)
		s.emitProgress(ctx, job, "finalize", 100, outcome.totalChunks, 0, outcome.maskingPct, space.TenantUUID)
		s.emitMetrics(job, outcome)
		return job, nil
	}

	if vectorErr != nil && !errors.Is(vectorErr, ErrIngestionDegraded) && !errors.Is(vectorErr, ErrVectorIndexNotActivated) {
		job.Status = knowledge.IngestionStatusFailed
		job.ErrorCode = "vector_upsert_failed"
		job.BlockedReason = vectorErr.Error()
		job.EmbeddingSuccessPct = 0
	}
	if job.Status != knowledge.IngestionStatusFailed {
		job.Status = knowledge.IngestionStatusCompleted
		if outcome.degraded {
			job.ErrorCode = outcome.errorCode
			job.BlockedReason = outcome.reason
		} else {
			job.ErrorCode = ""
			job.BlockedReason = ""
		}
	}

	job.MetricsSnapshot = mustJSON(outcome.snapshot(completed))

	if _, err := repo.NewIngestionJobRepository(s.db).Update(ctx, job); err != nil {
		return nil, err
	}
	s.emitProgress(ctx, job, "finalize", 100, outcome.totalChunks, outcome.embeddingPct, outcome.maskingPct, space.TenantUUID)
	s.emitMetrics(job, outcome)

	if vectorErr != nil && !errors.Is(vectorErr, ErrIngestionDegraded) && !errors.Is(vectorErr, ErrVectorIndexNotActivated) {
		return job, vectorErr
	}
	return job, nil
}

func (s *IngestionService) writeIngestionSegmentLog(format string, in TriggerIngestionInput, outcome pipelineOutcome, chunks []IngestionChunk, job *knowledge.IngestionJob) {
	if job == nil {
		return
	}
	baseDir := filepath.Join("backend", "logs", "ingestion_jobs")
	if wd, err := os.Getwd(); err == nil && strings.HasSuffix(wd, string(os.PathSeparator)+"backend") {
		baseDir = filepath.Join("logs", "ingestion_jobs")
	}
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return
	}
	path := filepath.Join(baseDir, job.UUID.String()+".log")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()

	anchorFlags := []string{}
	if in.AnchorHeadingPath {
		anchorFlags = append(anchorFlags, "heading_path")
	}
	if in.AnchorClauseID {
		anchorFlags = append(anchorFlags, "clause_id")
	}
	if in.AnchorRowNumber {
		anchorFlags = append(anchorFlags, "row_number")
	}
	if in.AnchorSpeaker {
		anchorFlags = append(anchorFlags, "speaker")
	}
	if in.AnchorSentenceIndex {
		anchorFlags = append(anchorFlags, "sentence_idx")
	}

	kindCounts := map[string]int{}
	segmentCounts := map[int]int{}
	pageCounts := map[int]int{}
	for _, ch := range chunks {
		kind := strings.TrimSpace(ch.Kind)
		if kind == "" {
			kind = "unknown"
		}
		kindCounts[kind]++
		if mi := ch.Metadata; mi != nil {
			if v, ok := mi["segment_part"]; ok {
				if n := parseAnyInt(v); n > 0 {
					segmentCounts[n]++
				}
			}
			if prov, ok := mi["provenance"].(map[string]any); ok {
				if n := parseAnyInt(prov["page"]); n > 0 {
					pageCounts[n]++
				} else if pages, ok := prov["pages"].([]any); ok {
					for _, p := range pages {
						if pm, ok := p.(map[string]any); ok {
							if pn := parseAnyInt(pm["page_number"]); pn > 0 {
								pageCounts[pn]++
							}
						}
					}
				}
			}
		}
	}

	ts := time.Now().Format(time.RFC3339)
	_, _ = fmt.Fprintf(
		f,
		"[%s] job=%s status=%s format=%s source=%s\n",
		ts,
		job.UUID,
		job.Status,
		strings.TrimSpace(format),
		strings.TrimSpace(in.SourceURI),
	)
	_, _ = fmt.Fprintf(
		f,
		"segment: page_priority=%t order=%v mode=%s size_policy=%s chunk_size=%d overlap=%d separators=%v anchors=%v\n",
		in.PagePriority,
		in.SegmentOrder,
		strings.TrimSpace(in.SegmentMode),
		normalizeSegmentSizePolicy(in.SegmentSizePolicy, in.ChunkSize),
		in.ChunkSize,
		in.ChunkOverlap,
		in.Separators,
		anchorFlags,
	)
	_, _ = fmt.Fprintf(
		f,
		"outcome: status=%s chunks=%d content_chunks=%d summary_chunks=%d coverage=%.2f%%\n",
		outcome.status,
		outcome.totalChunks,
		outcome.chunkCount,
		outcome.summaryCount,
		outcome.coveragePct,
	)
	_, _ = fmt.Fprintf(f, "chunk_kinds: %v\n", kindCounts)
	if len(segmentCounts) > 0 {
		_, _ = fmt.Fprintf(f, "segment_parts: %v\n", segmentCounts)
	}
	if len(pageCounts) > 0 {
		_, _ = fmt.Fprintf(f, "pages: %v\n", pageCounts)
	}
	_, _ = fmt.Fprintln(f, "----")
}

func parseAnyInt(v any) int {
	switch t := v.(type) {
	case int:
		return t
	case int64:
		return int(t)
	case float64:
		return int(t)
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(t))
		return n
	default:
		return 0
	}
}

func (s *IngestionService) emitProgress(ctx context.Context, job *knowledge.IngestionJob, stage string, progress int, chunkTotal int, embeddingPct float64, maskingPct float64, tenantUUID string) {
	if s == nil || s.progressPublisher == nil || job == nil {
		return
	}
	if progress < 0 {
		progress = 0
	}
	if progress > 100 {
		progress = 100
	}
	update := IngestionProgressUpdate{
		TenantUUID:   strings.TrimSpace(tenantUUID),
		SpaceUUID:    job.SpaceUUID.String(),
		JobUUID:      job.UUID.String(),
		Status:       job.Status,
		Stage:        strings.TrimSpace(stage),
		Progress:     progress,
		ChunkTotal:   chunkTotal,
		EmbeddingPct: embeddingPct,
		MaskingPct:   maskingPct,
		UpdatedAt:    time.Now().UTC(),
	}
	s.progressPublisher.PublishIngestionProgress(ctx, update)
}

func (s *IngestionService) persistWithRetry(ctx context.Context, space uuid.UUID, records []vectorstore.VectorRecord, job *knowledge.IngestionJob, outcome pipelineOutcome, tenantUUID string) error {
	if outcome.status == knowledge.IngestionStatusBlocked || s.vectorStore == nil || len(records) == 0 {
		return nil
	}
	const (
		persistProgressStart = 85
		persistProgressEnd   = 95
		persistProgressStep  = 2
		defaultBatchSize     = 128
	)
	batchSize := defaultBatchSize
	if batchSize <= 0 {
		batchSize = 128
	}
	var lastErr error
	for attempt := 0; attempt <= s.maxRetries; attempt++ {
		if attempt > 0 && job != nil {
			job.RetryCount = attempt
			job.Status = knowledge.IngestionStatusRetrying
			_, _ = repo.NewIngestionJobRepository(s.db).Update(ctx, job)
		}
		lastPersistProgress := -1
		emitPersistProgress := func(done, total int) {
			if job == nil || total <= 0 {
				return
			}
			if done > total {
				done = total
			}
			progress := persistProgressStart + int(float64(persistProgressEnd-persistProgressStart)*float64(done)/float64(total)+0.5)
			if progress < persistProgressStart {
				progress = persistProgressStart
			}
			if progress > persistProgressEnd {
				progress = persistProgressEnd
			}
			if lastPersistProgress >= 0 && progress-lastPersistProgress < persistProgressStep && done < total {
				return
			}
			lastPersistProgress = progress
			s.emitProgress(ctx, job, "persist", progress, outcome.totalChunks, outcome.embeddingPct, outcome.maskingPct, strings.TrimSpace(tenantUUID))
		}
		emitPersistProgress(0, len(records))

		done := 0
		for start := 0; start < len(records); start += batchSize {
			end := start + batchSize
			if end > len(records) {
				end = len(records)
			}
			if err := s.vectorStore.Upsert(ctx, space, records[start:end]); err != nil {
				// Space 未激活 dense index：允许入库完成，但标记为 degraded（不写向量）。
				if errors.Is(err, ErrVectorIndexNotActivated) {
					return ErrVectorIndexNotActivated
				}
				lastErr = err
				if attempt < s.maxRetries {
					time.Sleep(10 * time.Millisecond)
					goto retry
				}
				// Best-effort compensation.
				var chunkIDs []uuid.UUID
				for _, rec := range records {
					chunkIDs = append(chunkIDs, rec.ChunkID)
				}
				_ = s.vectorStore.DeleteByChunkIDs(ctx, space, chunkIDs)
				return err
			}
			done += end - start
			emitPersistProgress(done, len(records))
		}
		lastErr = nil
		break
	retry:
	}

	if outcome.degraded {
		return ErrIngestionDegraded
	}
	return lastErr
}

func (s *IngestionService) emitMetrics(job *knowledge.IngestionJob, outcome pipelineOutcome) {
	if job == nil {
		return
	}
	if s.metrics != nil {
		_ = s.metrics.Store(IngestionSnapshot{
			SpaceID:              job.SpaceUUID.String(),
			JobID:                job.UUID.String(),
			Status:               job.Status,
			RetryCount:           job.RetryCount,
			ChunkTotal:           job.ChunkTotal,
			SummaryChunkCount:    job.SummaryChunkCount,
			ParagraphChunkCount:  job.ParagraphChunkCount,
			CoveragePct:          job.ChunkCoveredPct,
			EmbeddingPct:         job.EmbeddingSuccessPct,
			MaskingPct:           job.MaskingCoveragePct,
			OCRRequired:          outcome.ocrRequired,
			OCRUsed:              outcome.ocrUsed,
			OCRCoveragePct:       outcome.ocrCoveragePct,
			OCRConfidenceBuckets: outcome.ocrConfidenceBuckets,
			OCRLatencyMs:         outcome.ocrLatencyMs,
			OCRPages:             outcome.ocrPageCount,
			OCRFailedPages:       outcome.ocrFailedPages,
			OCRBboxCoveragePct:   outcome.ocrBboxCoveragePct,
			Degraded:             outcome.degraded,
			ErrorCode:            job.ErrorCode,
			Reason:               job.BlockedReason,
			CompletedAt:          job.CompletedAt,
		})
	}
	s.inst.RecordIngestionCoverage(job.ChunkCoveredPct)
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

type DeleteIngestionJobResult struct {
	Deleted          bool `json:"deleted"`
	DeletedChunks    int  `json:"deletedChunks"`
	DeletedVectors   int  `json:"deletedVectors"`
	DeletedArtifacts bool `json:"deletedArtifacts"`
}

// DeleteJobPurge removes an ingestion job and best-effort clears derived data:
// - knowledge_chunks rows for the job (when table exists)
// - vector records for those chunks (when vector store is enabled)
// - artifact bundle record
// - local artifact directory (filesystem-backed ArtifactStore)
//
// This is intended for admin-only tooling / UI cleanup.
func (s *IngestionService) DeleteJobPurge(ctx context.Context, spaceID uuid.UUID, jobUUID uuid.UUID) (DeleteIngestionJobResult, error) {
	if s == nil || s.db == nil {
		return DeleteIngestionJobResult{}, errors.New("service unavailable")
	}
	if spaceID == uuid.Nil || jobUUID == uuid.Nil {
		return DeleteIngestionJobResult{}, ErrInvalidInput
	}

	job, err := s.GetJob(ctx, spaceID, jobUUID)
	if err != nil {
		return DeleteIngestionJobResult{}, err
	}
	if job == nil {
		return DeleteIngestionJobResult{Deleted: false}, nil
	}

	out := DeleteIngestionJobResult{Deleted: false}

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1) Collect chunk IDs (best effort).
		var chunkIDs []uuid.UUID
		if err := tx.Model(&knowledge.KnowledgeChunk{}).
			Select("chunk_uuid").
			Where("space_uuid = ? AND job_uuid = ?", spaceID, jobUUID).
			Scan(&chunkIDs).Error; err != nil {
			// If the table doesn't exist (index backend not provisioned), keep going.
			msg := strings.ToLower(err.Error())
			if !strings.Contains(msg, "does not exist") && !strings.Contains(msg, "relation") {
				return err
			}
		}

		// 2) Delete vectors first (best effort).
		if s.vectorStore != nil && len(chunkIDs) > 0 {
			if err := s.vectorStore.DeleteByChunkIDs(ctx, spaceID, chunkIDs); err == nil {
				out.DeletedVectors = len(chunkIDs)
			}
		}

		// 3) Delete chunk rows (ignore missing table).
		if err := tx.Where("space_uuid = ? AND job_uuid = ?", spaceID, jobUUID).Delete(&knowledge.KnowledgeChunk{}).Error; err != nil {
			msg := strings.ToLower(err.Error())
			if !strings.Contains(msg, "does not exist") && !strings.Contains(msg, "relation") {
				return err
			}
		} else {
			out.DeletedChunks = len(chunkIDs)
		}

		// 4) Delete artifact bundle record (by ingestion_job_id).
		_ = tx.Where("ingestion_job_id = ?", job.ID).Delete(&knowledge.ArtifactBundle{}).Error

		// 5) Delete job record.
		if err := tx.Where("uuid = ? AND space_uuid = ?", jobUUID, spaceID).Delete(&knowledge.IngestionJob{}).Error; err != nil {
			return err
		}

		out.Deleted = true
		return nil
	})
	if err != nil {
		return DeleteIngestionJobResult{}, err
	}

	// 6) Remove local artifacts after DB deletion (best-effort).
	if out.Deleted && s.artifactStore != nil {
		if ok, err := s.artifactStore.DeleteJobArtifacts(spaceID, jobUUID); err == nil {
			out.DeletedArtifacts = ok
		}
	}

	return out, nil
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

func sanitizeSeparators(raw []string) []string {
	if len(raw) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(raw))
	out := make([]string, 0, len(raw))
	for _, s := range raw {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if len([]rune(s)) > 16 {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
		if len(out) >= 32 {
			break
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func defaultSeparatorsFor(format string, mode string) []string {
	f := strings.ToLower(strings.TrimSpace(format))
	m := strings.ToLower(strings.TrimSpace(mode))
	// 通用分隔符：段落/换行优先，其次中文/英文句末标点，再到分号/冒号与 bullet。
	base := []string{"\n\n", "\n", "。", "！", "？", ".", "!", "?", "；", ";", "：", ":", "•"}
	if f == "sql" || m == "code_block" {
		return []string{"\n\n", "\n", ";", "}", "。"}
	}
	if f == "csv" || f == "xlsx" || f == "table" || m == "table_row" {
		return nil
	}
	return base
}

func normalizeSegmentSizePolicy(policy string, chunkSize int) string {
	p := strings.ToLower(strings.TrimSpace(policy))
	if p == "target" || p == "cap" {
		return p
	}
	if chunkSize > 0 {
		return "target"
	}
	return ""
}

type pipelineInput struct {
	space               *knowledge.KnowledgeSpace
	job                 *knowledge.IngestionJob
	bundle              *knowledge.ArtifactBundle
	format              string
	sourceURI           string
	docUUID             string
	ingestionProfile    string
	processorProfile    string
	ocrRequired         bool
	maskingProfile      string
	ragSceneKey         string
	ragBundleKey        string
	ragPrimary          string
	segmentMode         string
	chunkSize           int
	chunkOverlap        int
	segmentSizePolicy   string
	segmentOrder        []string
	separators          []string
	pagePriority        bool
	anchorHeadingPath   bool
	anchorClauseID      bool
	anchorRowNumber     bool
	anchorSpeaker       bool
	anchorSentenceIndex bool
}

type pipelineUnitsInput struct {
	space               *knowledge.KnowledgeSpace
	job                 *knowledge.IngestionJob
	bundle              *knowledge.ArtifactBundle
	format              string
	sourceURI           string
	docUUID             string
	docUnits            []DocumentUnit
	maskingProfile      string
	ocrRequired         bool
	ragSceneKey         string
	ragBundleKey        string
	ragPrimary          string
	segmentMode         string
	chunkSize           int
	chunkOverlap        int
	segmentSizePolicy   string
	segmentOrder        []string
	separators          []string
	pagePriority        bool
	anchorHeadingPath   bool
	anchorClauseID      bool
	anchorRowNumber     bool
	anchorSpeaker       bool
	anchorSentenceIndex bool
}

type pipelineOutcome struct {
	status                  string
	degraded                bool
	errorCode               string
	reason                  string
	totalChunks             int
	summaryCount            int
	chunkCount              int
	coveragePct             float64
	embeddingPct            float64
	embeddingMaxInputTokens int
	embeddingProvider       string
	embeddingModel          string
	maskingPct              float64
	language                string
	ocrRequired             bool
	ocrNeeded               bool
	ocrUsed                 bool
	ocrCoveragePct          float64
	ocrConfidenceBuckets    map[string]int
	ocrLatencyMs            int64
	ocrPageCount            int
	ocrFailedPages          int
	ocrBboxCoveragePct      float64
	// config snapshot (best-effort, for audit/debug)
	ragSceneKey       string
	ragBundleKey      string
	ragPrimary        string
	pagePriority      bool
	segmentOrder      []string
	segmentSizePolicy string
	segmentMode       string
	chunkSize         int
	chunkOverlap      int
	separators        []string
	chunkAnchors      map[string]bool
}

func (o pipelineOutcome) snapshot(completed time.Time) map[string]any {
	return map[string]any{
		"status":                     o.status,
		"degraded":                   o.degraded,
		"error_code":                 o.errorCode,
		"reason":                     o.reason,
		"chunk_total":                o.totalChunks,
		"summary_chunks":             o.summaryCount,
		"content_chunks":             o.chunkCount,
		"coverage_pct":               o.coveragePct,
		"embedding_pct":              o.embeddingPct,
		"embedding_max_input_tokens": o.embeddingMaxInputTokens,
		"embedding_provider":         o.embeddingProvider,
		"embedding_model":            o.embeddingModel,
		"masking_pct":                o.maskingPct,
		"language":                   o.language,
		"ocr_required":               o.ocrRequired,
		"ocr_needed":                 o.ocrNeeded,
		"ocr_used":                   o.ocrUsed,
		"ocr_coverage_pct":           o.ocrCoveragePct,
		"ocr_confidence":             o.ocrConfidenceBuckets,
		"ocr_latency_ms":             o.ocrLatencyMs,
		"ocr_pages":                  o.ocrPageCount,
		"ocr_failed_pages":           o.ocrFailedPages,
		"ocr_bbox_pct":               o.ocrBboxCoveragePct,
		"rag_scene_key":              o.ragSceneKey,
		"rag_bundle_key":             o.ragBundleKey,
		"rag_primary":                o.ragPrimary,
		"page_priority":              o.pagePriority,
		"segment_order":              o.segmentOrder,
		"segment_size_policy":        o.segmentSizePolicy,
		"segment_mode":               o.segmentMode,
		"chunk_size":                 o.chunkSize,
		"chunk_overlap":              o.chunkOverlap,
		"separators":                 o.separators,
		"chunk_anchors":              o.chunkAnchors,
		"completed":                  completed,
	}
}

type IngestionChunk struct {
	ID         uuid.UUID
	Kind       string
	Content    string
	Metadata   map[string]any
	Confidence float64
	Masked     bool
}

func (s *IngestionService) runPipeline(ctx context.Context, in pipelineInput) (pipelineOutcome, []IngestionChunk, []vectorstore.VectorRecord, *OCRArtifacts) {
	format := strings.ToLower(strings.TrimSpace(in.format))
	sourceURI := strings.TrimSpace(in.sourceURI)
	separators := sanitizeSeparators(in.separators)
	mode := strings.ToLower(strings.TrimSpace(in.segmentMode))
	sizePolicy := normalizeSegmentSizePolicy(in.segmentSizePolicy, in.chunkSize)
	if mode == "" {
		mode = "unit"
	}
	// 当调用方未显式传 separators 且启用了 chunkSize 窗口切分时，给一组“安全默认分隔符”，
	// 以便窗口边界尽量对齐句子/换行，避免硬截断。
	if in.chunkSize > 0 && mode != "table_row" && len(separators) == 0 {
		separators = defaultSeparatorsFor(format, mode)
	}
	out := pipelineOutcome{
		status:               knowledge.IngestionStatusCompleted,
		coveragePct:          100,
		embeddingPct:         100,
		maskingPct:           100,
		ocrRequired:          in.ocrRequired,
		ocrConfidenceBuckets: map[string]int{"0.0-0.5": 0, "0.5-0.8": 0, "0.8-1.0": 0},
		ragSceneKey:          strings.TrimSpace(in.ragSceneKey),
		ragBundleKey:         strings.TrimSpace(in.ragBundleKey),
		ragPrimary:           strings.TrimSpace(in.ragPrimary),
		pagePriority:         in.pagePriority,
		segmentOrder:         in.segmentOrder,
		segmentSizePolicy:    sizePolicy,
		segmentMode:          strings.TrimSpace(in.segmentMode),
		chunkSize:            in.chunkSize,
		chunkOverlap:         in.chunkOverlap,
		separators:           separators,
		chunkAnchors: map[string]bool{
			"heading_path": in.anchorHeadingPath,
			"clause_id":    in.anchorClauseID,
			"row_number":   in.anchorRowNumber,
			"speaker":      in.anchorSpeaker,
			"sentence_idx": in.anchorSentenceIndex,
		},
	}

	needsOCR := in.ocrRequired
	if format == "image" {
		needsOCR = true
	}
	if format == "pdf" && strings.Contains(strings.ToLower(sourceURI), "scan") {
		needsOCR = true
	}

	processor, resolution := s.processors.Resolve(format, needsOCR, in.ocrRequired, in.processorProfile)
	out.ocrNeeded = needsOCR
	out.ocrUsed = resolution.OCRUsed
	if resolution.Decision == ProcessorDecisionBlocked {
		out.status = knowledge.IngestionStatusBlocked
		out.errorCode = resolution.ErrorCode
		out.reason = resolution.Reason
		out.coveragePct = 0
		out.embeddingPct = 0
		out.maskingPct = 0
		s.emitProgress(ctx, in.job, "extract", 1, 0, out.embeddingPct, out.maskingPct, in.space.TenantUUID)
		return out, nil, nil, nil
	}
	if resolution.Decision == ProcessorDecisionDegraded {
		out.degraded = true
		out.errorCode = resolution.ErrorCode
		out.reason = resolution.Reason
		out.coveragePct = 40
	}

	pagePriority := in.pagePriority && format == "pdf"
	res, err := processor.Process(ctx, DocumentProcessInput{
		SpaceID:      in.space.UUID.String(),
		JobID:        in.job.UUID.String(),
		Format:       format,
		SourceURI:    sourceURI,
		NeedOCR:      needsOCR,
		OCRAvailable: resolution.OCRAvailable,
		PagePriority: pagePriority,
	})
	if err != nil {
		// PDF 处理器的优先级：如果选择了 pdftotext，但 sourceURI scheme 不支持（例如 s3://、minio://），
		// 则回退到 builtin/pdf（合成内容），避免在“可用二进制存在但 URI 不可达”时误判为 degraded。
		if format == "pdf" && !needsOCR && errors.Is(err, ErrUnsupportedSourceURIScheme) {
			res, err = (PDFProcessor{}).Process(ctx, DocumentProcessInput{
				SpaceID:      in.space.UUID.String(),
				JobID:        in.job.UUID.String(),
				Format:       format,
				SourceURI:    sourceURI,
				NeedOCR:      needsOCR,
				OCRAvailable: resolution.OCRAvailable,
				PagePriority: pagePriority,
			})
		}
	}
	if err != nil {
		if needsOCR && in.ocrRequired {
			out.status = knowledge.IngestionStatusBlocked
			out.errorCode = "ocr_failed"
			out.reason = err.Error()
			out.coveragePct = 0
			out.embeddingPct = 0
			out.maskingPct = 0
			s.emitProgress(ctx, in.job, "extract", 3, 0, out.embeddingPct, out.maskingPct, in.space.TenantUUID)
			return out, nil, nil, nil
		}
		out.degraded = true
		if out.errorCode == "" {
			out.errorCode = "degraded"
		}
		if out.reason == "" {
			out.reason = "processor_failed"
		}
		out.coveragePct = 40
	}
	docUnits := res.Units
	out.ocrCoveragePct = res.OCR.CoveragePct
	out.ocrConfidenceBuckets = res.OCR.ConfidenceBuckets
	out.ocrLatencyMs = res.OCR.LatencyMs
	out.ocrPageCount = res.OCR.PageCount
	out.ocrFailedPages = res.OCR.FailedPages
	out.ocrBboxCoveragePct = res.OCR.BboxCoveragePct
	s.emitProgress(ctx, in.job, "extract", 5, 0, out.embeddingPct, out.maskingPct, in.space.TenantUUID)

	lastChunkProgress := -1
	emitChunkProgress := func(done, total float64) {
		if total <= 0 || in.job == nil {
			return
		}
		if done > total {
			done = total
		}
		const chunkProgressStart = 5
		const chunkProgressEnd = 15
		const chunkProgressStep = 2
		progress := chunkProgressStart + int(float64(chunkProgressEnd-chunkProgressStart)*done/total+0.5)
		if progress < chunkProgressStart {
			progress = chunkProgressStart
		}
		if progress > chunkProgressEnd {
			progress = chunkProgressEnd
		}
		if lastChunkProgress >= 0 && progress-lastChunkProgress < chunkProgressStep && done < total {
			return
		}
		lastChunkProgress = progress
		s.emitProgress(ctx, in.job, "chunk", progress, 0, out.embeddingPct, out.maskingPct, in.space.TenantUUID)
	}
	chunks := ChunkDocument(in.space.UUID, format, sourceURI, docUnits, ChunkingOptions{
		Mode:         in.segmentMode,
		SizePolicy:   sizePolicy,
		PagePriority: in.pagePriority,
		DocUUID:      in.docUUID,
		ChunkSize:    in.chunkSize,
		ChunkOverlap: in.chunkOverlap,
		SegmentOrder: in.segmentOrder,
		Separators:   separators,
		Anchors: ChunkAnchors{
			HeadingPath:   in.anchorHeadingPath,
			ClauseID:      in.anchorClauseID,
			RowNumber:     in.anchorRowNumber,
			Speaker:       in.anchorSpeaker,
			SentenceIndex: in.anchorSentenceIndex,
		},
	}, emitChunkProgress)
	if len(chunks) == 0 || !hasContentChunks(chunks) {
		chunks = append(chunks, IngestionChunk{
			ID:      uuid.NewSHA1(in.space.UUID, []byte("section_summary|placeholder|"+format+"|"+sourceURI)),
			Kind:    "section_summary",
			Content: "Section 1 summary (placeholder)",
			Metadata: map[string]any{
				"format":     format,
				"source_uri": sourceURI,
				"provenance": map[string]any{},
				"section":    1,
			},
		})
		chunks = append(chunks, IngestionChunk{
			ID:      uuid.NewSHA1(in.space.UUID, []byte("chunk|placeholder|"+format+"|"+sourceURI)),
			Kind:    "chunk",
			Content: "content unavailable",
			Metadata: map[string]any{
				"format":     format,
				"source_uri": sourceURI,
				"provenance": map[string]any{},
			},
		})
		out.degraded = true
		if out.errorCode == "" {
			out.errorCode = "degraded"
		}
		if out.reason == "" {
			out.reason = "empty_content"
		}
		out.coveragePct = 0
	}

	// Masking.
	masker := NewMasker(in.maskingProfile)
	maskedChunks, maskingPct, maskBlock := masker.Apply(chunks)
	out.maskingPct = maskingPct
	if maskBlock {
		out.status = knowledge.IngestionStatusBlocked
		out.errorCode = "masking_required"
		out.reason = "masking_blocked"
		out.coveragePct = 0
		out.embeddingPct = 0
		s.emitProgress(ctx, in.job, "chunk", 15, len(maskedChunks), out.embeddingPct, out.maskingPct, in.space.TenantUUID)
		return out, maskedChunks, nil, res.Artifacts
	}

	out.language = detectLanguage(maskedChunks)
	summaryCount, contentCount := countChunkKinds(maskedChunks)
	out.summaryCount = summaryCount
	out.chunkCount = contentCount
	out.totalChunks = len(maskedChunks)
	s.emitProgress(ctx, in.job, "chunk", 15, out.totalChunks, out.embeddingPct, out.maskingPct, in.space.TenantUUID)

	// Make job linkage explicit in chunk metadata (used by online chunk store + UI/API filtering).
	for i := range maskedChunks {
		if maskedChunks[i].Metadata == nil {
			maskedChunks[i].Metadata = map[string]any{}
		}
		maskedChunks[i].Metadata["job_uuid"] = in.job.UUID.String()
	}

	lastEmbedProgress := -1
	emitEmbedProgress := func(done, total int) {
		if total <= 0 || in.job == nil {
			return
		}
		if done > total {
			done = total
		}
		const embedProgressStart = 15
		const embedProgressEnd = 85
		const embedProgressStep = 2
		progress := embedProgressStart + int(float64(embedProgressEnd-embedProgressStart)*float64(done)/float64(total)+0.5)
		if progress < embedProgressStart {
			progress = embedProgressStart
		}
		if progress > embedProgressEnd {
			progress = embedProgressEnd
		}
		if lastEmbedProgress >= 0 && progress-lastEmbedProgress < embedProgressStep && done < total {
			return
		}
		lastEmbedProgress = progress
		embeddingPct := 100.0 * float64(done) / float64(total)
		s.emitProgress(ctx, in.job, "embed", progress, out.totalChunks, embeddingPct, out.maskingPct, in.space.TenantUUID)
	}
	records, embeddingPct, embedDegraded, embedErrCode, embedReason, embedMaxInput, embedProvider, embedModel := s.buildVectorRecords(
		ctx,
		in.space,
		maskedChunks,
		emitEmbedProgress,
	)
	out.embeddingPct = embeddingPct
	out.embeddingMaxInputTokens = embedMaxInput
	out.embeddingProvider = strings.TrimSpace(embedProvider)
	out.embeddingModel = strings.TrimSpace(embedModel)
	if embedDegraded {
		out.degraded = true
		if out.errorCode == "" {
			out.errorCode = embedErrCode
		}
		if out.reason == "" {
			out.reason = embedReason
		}
		if embedErrCode == "vector_index_not_activated" {
			out.status = knowledge.IngestionStatusBlocked
		}
	}
	s.emitProgress(ctx, in.job, "embed", 85, out.totalChunks, out.embeddingPct, out.maskingPct, in.space.TenantUUID)

	return out, maskedChunks, records, res.Artifacts
}

func (s *IngestionService) runPipelineFromUnits(ctx context.Context, in pipelineUnitsInput) (pipelineOutcome, []IngestionChunk, []vectorstore.VectorRecord) {
	format := strings.ToLower(strings.TrimSpace(in.format))
	sourceURI := strings.TrimSpace(in.sourceURI)
	separators := sanitizeSeparators(in.separators)
	mode := strings.ToLower(strings.TrimSpace(in.segmentMode))
	sizePolicy := normalizeSegmentSizePolicy(in.segmentSizePolicy, in.chunkSize)
	if mode == "" {
		mode = "unit"
	}
	if in.chunkSize > 0 && mode != "table_row" && len(separators) == 0 {
		separators = defaultSeparatorsFor(format, mode)
	}
	out := pipelineOutcome{
		status:               knowledge.IngestionStatusCompleted,
		coveragePct:          100,
		embeddingPct:         100,
		maskingPct:           100,
		ocrRequired:          in.ocrRequired,
		ocrConfidenceBuckets: map[string]int{"0.0-0.5": 0, "0.5-0.8": 0, "0.8-1.0": 0},
		ragSceneKey:          strings.TrimSpace(in.ragSceneKey),
		ragBundleKey:         strings.TrimSpace(in.ragBundleKey),
		ragPrimary:           strings.TrimSpace(in.ragPrimary),
		pagePriority:         in.pagePriority,
		segmentOrder:         in.segmentOrder,
		segmentSizePolicy:    sizePolicy,
		segmentMode:          strings.TrimSpace(in.segmentMode),
		chunkSize:            in.chunkSize,
		chunkOverlap:         in.chunkOverlap,
		separators:           separators,
		chunkAnchors: map[string]bool{
			"heading_path": in.anchorHeadingPath,
			"clause_id":    in.anchorClauseID,
			"row_number":   in.anchorRowNumber,
			"speaker":      in.anchorSpeaker,
			"sentence_idx": in.anchorSentenceIndex,
		},
	}

	docUnits := in.docUnits
	if len(docUnits) == 0 {
		// Keep behavior consistent with runPipeline: empty content yields degraded path.
		docUnits = []DocumentUnit{{
			Content: "content unavailable",
			Provenance: map[string]any{
				"format":     format,
				"source_uri": sourceURI,
				"reason":     "empty_units",
			},
			Confidence: 0.2,
		}}
		out.degraded = true
		out.errorCode = "degraded"
		out.reason = "empty_content"
		out.coveragePct = 0
	}
	s.emitProgress(ctx, in.job, "extract", 5, 0, out.embeddingPct, out.maskingPct, in.space.TenantUUID)

	lastChunkProgress := -1
	emitChunkProgress := func(done, total float64) {
		if total <= 0 || in.job == nil {
			return
		}
		if done > total {
			done = total
		}
		const chunkProgressStart = 5
		const chunkProgressEnd = 15
		const chunkProgressStep = 2
		progress := chunkProgressStart + int(float64(chunkProgressEnd-chunkProgressStart)*done/total+0.5)
		if progress < chunkProgressStart {
			progress = chunkProgressStart
		}
		if progress > chunkProgressEnd {
			progress = chunkProgressEnd
		}
		if lastChunkProgress >= 0 && progress-lastChunkProgress < chunkProgressStep && done < total {
			return
		}
		lastChunkProgress = progress
		s.emitProgress(ctx, in.job, "chunk", progress, 0, out.embeddingPct, out.maskingPct, in.space.TenantUUID)
	}
	chunks := ChunkDocument(in.space.UUID, format, sourceURI, docUnits, ChunkingOptions{
		Mode:         in.segmentMode,
		SizePolicy:   sizePolicy,
		PagePriority: in.pagePriority,
		DocUUID:      in.docUUID,
		ChunkSize:    in.chunkSize,
		ChunkOverlap: in.chunkOverlap,
		SegmentOrder: in.segmentOrder,
		Separators:   separators,
		Anchors: ChunkAnchors{
			HeadingPath:   in.anchorHeadingPath,
			ClauseID:      in.anchorClauseID,
			RowNumber:     in.anchorRowNumber,
			Speaker:       in.anchorSpeaker,
			SentenceIndex: in.anchorSentenceIndex,
		},
	}, emitChunkProgress)
	if len(chunks) == 0 || !hasContentChunks(chunks) {
		chunks = append(chunks, IngestionChunk{
			ID:      uuid.NewSHA1(in.space.UUID, []byte("section_summary|placeholder|"+format+"|"+sourceURI)),
			Kind:    "section_summary",
			Content: "Section 1 summary (placeholder)",
			Metadata: map[string]any{
				"format":     format,
				"source_uri": sourceURI,
				"provenance": map[string]any{},
				"section":    1,
			},
		})
		chunks = append(chunks, IngestionChunk{
			ID:      uuid.NewSHA1(in.space.UUID, []byte("chunk|placeholder|"+format+"|"+sourceURI)),
			Kind:    "chunk",
			Content: "content unavailable",
			Metadata: map[string]any{
				"format":     format,
				"source_uri": sourceURI,
				"provenance": map[string]any{},
			},
		})
		out.degraded = true
		if out.errorCode == "" {
			out.errorCode = "degraded"
		}
		if out.reason == "" {
			out.reason = "empty_content"
		}
		out.coveragePct = 0
	}

	// Masking.
	masker := NewMasker(in.maskingProfile)
	maskedChunks, maskingPct, maskBlock := masker.Apply(chunks)
	out.maskingPct = maskingPct
	if maskBlock {
		out.status = knowledge.IngestionStatusBlocked
		out.errorCode = "masking_required"
		out.reason = "masking_blocked"
		out.coveragePct = 0
		out.embeddingPct = 0
		s.emitProgress(ctx, in.job, "chunk", 15, len(maskedChunks), out.embeddingPct, out.maskingPct, in.space.TenantUUID)
		return out, maskedChunks, nil
	}

	out.language = detectLanguage(maskedChunks)
	summaryCount, contentCount := countChunkKinds(maskedChunks)
	out.summaryCount = summaryCount
	out.chunkCount = contentCount
	out.totalChunks = len(maskedChunks)
	s.emitProgress(ctx, in.job, "chunk", 15, out.totalChunks, out.embeddingPct, out.maskingPct, in.space.TenantUUID)

	for i := range maskedChunks {
		if maskedChunks[i].Metadata == nil {
			maskedChunks[i].Metadata = map[string]any{}
		}
		maskedChunks[i].Metadata["job_uuid"] = in.job.UUID.String()
	}

	lastEmbedProgress := -1
	emitEmbedProgress := func(done, total int) {
		if total <= 0 || in.job == nil {
			return
		}
		if done > total {
			done = total
		}
		const embedProgressStart = 15
		const embedProgressEnd = 85
		const embedProgressStep = 2
		progress := embedProgressStart + int(float64(embedProgressEnd-embedProgressStart)*float64(done)/float64(total)+0.5)
		if progress < embedProgressStart {
			progress = embedProgressStart
		}
		if progress > embedProgressEnd {
			progress = embedProgressEnd
		}
		if lastEmbedProgress >= 0 && progress-lastEmbedProgress < embedProgressStep && done < total {
			return
		}
		lastEmbedProgress = progress
		embeddingPct := 100.0 * float64(done) / float64(total)
		s.emitProgress(ctx, in.job, "embed", progress, out.totalChunks, embeddingPct, out.maskingPct, in.space.TenantUUID)
	}
	records, embeddingPct, embedDegraded, embedErrCode, embedReason, embedMaxInput, embedProvider, embedModel := s.buildVectorRecords(
		ctx,
		in.space,
		maskedChunks,
		emitEmbedProgress,
	)
	out.embeddingPct = embeddingPct
	out.embeddingMaxInputTokens = embedMaxInput
	out.embeddingProvider = strings.TrimSpace(embedProvider)
	out.embeddingModel = strings.TrimSpace(embedModel)
	if embedDegraded {
		out.degraded = true
		if out.errorCode == "" {
			out.errorCode = embedErrCode
		}
		if out.reason == "" {
			out.reason = embedReason
		}
		if embedErrCode == "vector_index_not_activated" {
			out.status = knowledge.IngestionStatusBlocked
		}
	}
	s.emitProgress(ctx, in.job, "embed", 85, out.totalChunks, out.embeddingPct, out.maskingPct, in.space.TenantUUID)

	return out, maskedChunks, records
}

func stableSourceID(sourceURI string) string {
	normalized := strings.ToLower(strings.TrimSpace(sourceURI))
	if normalized == "" {
		return fmt.Sprintf("src-%s", uuid.NewString())
	}
	return "src-" + ContentHash(normalized)
}

func detectLanguage(chunks []IngestionChunk) string {
	if len(chunks) == 0 {
		return "unknown"
	}
	sample := ""
	for _, c := range chunks {
		if strings.TrimSpace(c.Content) != "" {
			sample += " " + c.Content
		}
		if len(sample) > 4096 {
			break
		}
	}
	sample = strings.TrimSpace(sample)
	if sample == "" {
		return "unknown"
	}

	var han, latin, other int
	for _, r := range sample {
		switch {
		case r >= 0x4E00 && r <= 0x9FFF:
			han++
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z'):
			latin++
		case r <= 0x7F:
			// ignore ASCII punctuation/space
		default:
			other++
		}
	}

	total := han + latin + other
	if total == 0 {
		return "unknown"
	}
	if han*100/total >= 20 {
		if latin*100/total >= 20 {
			return "mixed"
		}
		return "zh"
	}
	if latin*100/total >= 20 {
		return "en"
	}
	return "unknown"
}

func countChunkKinds(chunks []IngestionChunk) (summaryCount int, contentCount int) {
	for _, c := range chunks {
		switch c.Kind {
		case "doc_summary", "section_summary":
			summaryCount++
		default:
			contentCount++
		}
	}
	return summaryCount, contentCount
}

func hasContentChunks(chunks []IngestionChunk) bool {
	for _, c := range chunks {
		if c.Kind != "doc_summary" && c.Kind != "section_summary" {
			return true
		}
	}
	return false
}
