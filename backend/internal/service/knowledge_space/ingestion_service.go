package knowledge_space

import (
	"context"
	"encoding/json"
	"errors"
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
	// L1/L2/L3 snapshot (best-effort).
	RagSceneKey  string
	RagBundleKey string
	RagPrimary   string
	SegmentMode      string
	ChunkSize        int
	ChunkOverlap     int
	Separators       []string
	AnchorHeadingPath  bool
	AnchorClauseID     bool
	AnchorRowNumber    bool
	AnchorSpeaker      bool
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

	outcome, chunks, vectorRecords := s.runPipeline(ctx, pipelineInput{
		space:            space,
		job:              job,
		bundle:           bundle,
		format:           format,
		sourceURI:        in.SourceURI,
		ingestionProfile: in.IngestionProfile,
		processorProfile: in.ProcessorProfile,
		ocrRequired:      in.OCRRequired,
		maskingProfile:   in.MaskingProfile,
		ragSceneKey:      strings.TrimSpace(in.RagSceneKey),
		ragBundleKey:     strings.TrimSpace(in.RagBundleKey),
		ragPrimary:       strings.TrimSpace(in.RagPrimary),
		segmentMode:      in.SegmentMode,
		chunkSize:        in.ChunkSize,
		chunkOverlap:     in.ChunkOverlap,
		separators:       in.Separators,
		anchorHeadingPath:  in.AnchorHeadingPath,
		anchorClauseID:     in.AnchorClauseID,
		anchorRowNumber:    in.AnchorRowNumber,
		anchorSpeaker:      in.AnchorSpeaker,
		anchorSentenceIndex: in.AnchorSentenceIndex,
	})
	return s.finalizeIngestion(ctx, space, job, bundle, format, in, outcome, chunks, vectorRecords)
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
		space:          space,
		job:            job,
		bundle:         bundle,
		format:         format,
		sourceURI:      in.SourceURI,
		docUnits:       docUnits,
		maskingProfile: in.MaskingProfile,
		ocrRequired:    in.OCRRequired,
		ragSceneKey:    strings.TrimSpace(in.RagSceneKey),
		ragBundleKey:   strings.TrimSpace(in.RagBundleKey),
		ragPrimary:     strings.TrimSpace(in.RagPrimary),
		segmentMode:    in.SegmentMode,
		chunkSize:      in.ChunkSize,
		chunkOverlap:   in.ChunkOverlap,
		separators:     in.Separators,
		anchorHeadingPath:  in.AnchorHeadingPath,
		anchorClauseID:     in.AnchorClauseID,
		anchorRowNumber:    in.AnchorRowNumber,
		anchorSpeaker:      in.AnchorSpeaker,
		anchorSentenceIndex: in.AnchorSentenceIndex,
	})
	return s.finalizeIngestion(ctx, space, job, bundle, format, in, outcome, chunks, vectorRecords)
}

// TriggerAsync creates an ingestion job and runs the pipeline in background.
// It is intended for HTTP/API usage so UI can poll job status asynchronously.
func (s *IngestionService) TriggerAsync(ctx context.Context, in TriggerIngestionInput) (*knowledge.IngestionJob, error) {
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
		// Run the same pipeline and update job in DB.
		outcome, chunks, vectors := s.runPipeline(bg, pipelineInput{
			space:            space,
			job:              job,
			bundle:           bundle,
			format:           format,
			sourceURI:        in.SourceURI,
			ingestionProfile: in.IngestionProfile,
			processorProfile: in.ProcessorProfile,
			ocrRequired:      in.OCRRequired,
			maskingProfile:   in.MaskingProfile,
			ragSceneKey:      strings.TrimSpace(in.RagSceneKey),
			ragBundleKey:     strings.TrimSpace(in.RagBundleKey),
			ragPrimary:       strings.TrimSpace(in.RagPrimary),
			segmentMode:      in.SegmentMode,
			chunkSize:        in.ChunkSize,
			chunkOverlap:     in.ChunkOverlap,
			separators:       in.Separators,
			anchorHeadingPath:  in.AnchorHeadingPath,
			anchorClauseID:     in.AnchorClauseID,
			anchorRowNumber:    in.AnchorRowNumber,
			anchorSpeaker:      in.AnchorSpeaker,
			anchorSentenceIndex: in.AnchorSentenceIndex,
		})
		if _, err := s.finalizeIngestion(bg, space, job, bundle, format, in, outcome, chunks, vectors); err != nil {
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
		}); err == nil {
			bundle.ChunkManifestURI = artifactUpdate.ChunkManifestURI
			bundle.VectorManifestURI = artifactUpdate.VectorManifestURI
			bundle.MaskingReportURI = artifactUpdate.MaskingReportURI
			bundle.Checksum = artifactUpdate.Checksum
			bundle.SummaryChunkCount = outcome.summaryCount
			bundle.ParagraphChunkCount = outcome.chunkCount
			_, _ = repo.NewArtifactBundleRepository(s.db).Update(ctx, bundle)
		}
	}

	vectorErr := s.persistWithRetry(ctx, in.SpaceID, vectorRecords, job, outcome)
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
		s.emitMetrics(job, outcome)
		return job, nil
	}

	if vectorErr != nil && !errors.Is(vectorErr, ErrIngestionDegraded) {
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
	s.emitMetrics(job, outcome)

	if vectorErr != nil && !errors.Is(vectorErr, ErrIngestionDegraded) {
		return job, vectorErr
	}
	return job, nil
}

func (s *IngestionService) persistWithRetry(ctx context.Context, space uuid.UUID, records []vectorstore.VectorRecord, job *knowledge.IngestionJob, outcome pipelineOutcome) error {
	if outcome.status == knowledge.IngestionStatusBlocked || s.vectorStore == nil || len(records) == 0 {
		return nil
	}
	var lastErr error
	for attempt := 0; attempt <= s.maxRetries; attempt++ {
		if attempt > 0 && job != nil {
			job.RetryCount = attempt
			job.Status = knowledge.IngestionStatusRetrying
			_, _ = repo.NewIngestionJobRepository(s.db).Update(ctx, job)
		}
		if err := s.vectorStore.Upsert(ctx, space, records); err != nil {
			lastErr = err
			if attempt < s.maxRetries {
				time.Sleep(10 * time.Millisecond)
				continue
			}
			// Best-effort compensation.
			var chunkIDs []uuid.UUID
			for _, rec := range records {
				chunkIDs = append(chunkIDs, rec.ChunkID)
			}
			_ = s.vectorStore.DeleteByChunkIDs(ctx, space, chunkIDs)
			return err
		}
		lastErr = nil
		break
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

type pipelineInput struct {
	space            *knowledge.KnowledgeSpace
	job              *knowledge.IngestionJob
	bundle           *knowledge.ArtifactBundle
	format           string
	sourceURI        string
	ingestionProfile string
	processorProfile string
	ocrRequired      bool
	maskingProfile   string
	ragSceneKey      string
	ragBundleKey     string
	ragPrimary       string
	segmentMode      string
	chunkSize        int
	chunkOverlap     int
	separators       []string
	anchorHeadingPath  bool
	anchorClauseID     bool
	anchorRowNumber    bool
	anchorSpeaker      bool
	anchorSentenceIndex bool
}

type pipelineUnitsInput struct {
	space          *knowledge.KnowledgeSpace
	job            *knowledge.IngestionJob
	bundle         *knowledge.ArtifactBundle
	format         string
	sourceURI      string
	docUnits       []DocumentUnit
	maskingProfile string
	ocrRequired    bool
	ragSceneKey    string
	ragBundleKey   string
	ragPrimary     string
	segmentMode    string
	chunkSize      int
	chunkOverlap   int
	separators     []string
	anchorHeadingPath  bool
	anchorClauseID     bool
	anchorRowNumber    bool
	anchorSpeaker      bool
	anchorSentenceIndex bool
}

type pipelineOutcome struct {
	status               string
	degraded             bool
	errorCode            string
	reason               string
	totalChunks          int
	summaryCount         int
	chunkCount           int
	coveragePct          float64
	embeddingPct         float64
	maskingPct           float64
	language             string
	ocrRequired          bool
	ocrNeeded            bool
	ocrUsed              bool
	ocrCoveragePct       float64
	ocrConfidenceBuckets map[string]int
	// config snapshot (best-effort, for audit/debug)
	ragSceneKey   string
	ragBundleKey  string
	ragPrimary    string
	segmentMode   string
	chunkSize     int
	chunkOverlap  int
	separators    []string
	chunkAnchors  map[string]bool
}

func (o pipelineOutcome) snapshot(completed time.Time) map[string]any {
	return map[string]any{
		"status":           o.status,
		"degraded":         o.degraded,
		"error_code":       o.errorCode,
		"reason":           o.reason,
		"chunk_total":      o.totalChunks,
		"summary_chunks":   o.summaryCount,
		"content_chunks":   o.chunkCount,
		"coverage_pct":     o.coveragePct,
		"embedding_pct":    o.embeddingPct,
		"masking_pct":      o.maskingPct,
		"language":         o.language,
		"ocr_required":     o.ocrRequired,
		"ocr_needed":       o.ocrNeeded,
		"ocr_used":         o.ocrUsed,
		"ocr_coverage_pct": o.ocrCoveragePct,
		"ocr_confidence":   o.ocrConfidenceBuckets,
		"rag_scene_key":    o.ragSceneKey,
		"rag_bundle_key":   o.ragBundleKey,
		"rag_primary":      o.ragPrimary,
		"segment_mode":     o.segmentMode,
		"chunk_size":       o.chunkSize,
		"chunk_overlap":    o.chunkOverlap,
		"separators":       o.separators,
		"chunk_anchors":    o.chunkAnchors,
		"completed":        completed,
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

func (s *IngestionService) runPipeline(ctx context.Context, in pipelineInput) (pipelineOutcome, []IngestionChunk, []vectorstore.VectorRecord) {
	format := strings.ToLower(strings.TrimSpace(in.format))
	sourceURI := strings.TrimSpace(in.sourceURI)
	separators := sanitizeSeparators(in.separators)
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
		segmentMode:          strings.TrimSpace(in.segmentMode),
		chunkSize:            in.chunkSize,
		chunkOverlap:         in.chunkOverlap,
		separators:           separators,
		chunkAnchors: map[string]bool{
			"heading_path":  in.anchorHeadingPath,
			"clause_id":     in.anchorClauseID,
			"row_number":    in.anchorRowNumber,
			"speaker":       in.anchorSpeaker,
			"sentence_idx":  in.anchorSentenceIndex,
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
		return out, nil, nil
	}
	if resolution.Decision == ProcessorDecisionDegraded {
		out.degraded = true
		out.errorCode = resolution.ErrorCode
		out.reason = resolution.Reason
		out.coveragePct = 40
	}

	docUnits, ocrStats := processor.Process(ctx, DocumentProcessInput{
		Format:       format,
		SourceURI:    sourceURI,
		NeedOCR:      needsOCR,
		OCRAvailable: resolution.OCRAvailable,
	})
	out.ocrCoveragePct = ocrStats.CoveragePct
	out.ocrConfidenceBuckets = ocrStats.ConfidenceBuckets

	chunks := ChunkDocument(in.space.UUID, format, sourceURI, docUnits, ChunkingOptions{
		Mode:         in.segmentMode,
		ChunkSize:    in.chunkSize,
		ChunkOverlap: in.chunkOverlap,
		Separators:   separators,
		Anchors: ChunkAnchors{
			HeadingPath:  in.anchorHeadingPath,
			ClauseID:     in.anchorClauseID,
			RowNumber:    in.anchorRowNumber,
			Speaker:      in.anchorSpeaker,
			SentenceIndex: in.anchorSentenceIndex,
		},
	})
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
		return out, maskedChunks, nil
	}

	out.language = detectLanguage(maskedChunks)
	summaryCount, contentCount := countChunkKinds(maskedChunks)
	out.summaryCount = summaryCount
	out.chunkCount = contentCount
	out.totalChunks = len(maskedChunks)

	records := make([]vectorstore.VectorRecord, 0, len(maskedChunks))
	for _, chunk := range maskedChunks {
		embedding := HashEmbedding(chunk.Content, 32)
		meta := make(map[string]any, len(chunk.Metadata)+2)
		for k, v := range chunk.Metadata {
			meta[k] = v
		}
		meta["chunk_kind"] = chunk.Kind
		meta["content_hash"] = ContentHash(chunk.Content)
		records = append(records, vectorstore.VectorRecord{
			ChunkID:   chunk.ID,
			Embedding: embedding,
			Metadata:  meta,
		})
	}

	return out, maskedChunks, records
}

func (s *IngestionService) runPipelineFromUnits(ctx context.Context, in pipelineUnitsInput) (pipelineOutcome, []IngestionChunk, []vectorstore.VectorRecord) {
	format := strings.ToLower(strings.TrimSpace(in.format))
	sourceURI := strings.TrimSpace(in.sourceURI)
	separators := sanitizeSeparators(in.separators)
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
		segmentMode:          strings.TrimSpace(in.segmentMode),
		chunkSize:            in.chunkSize,
		chunkOverlap:         in.chunkOverlap,
		separators:           separators,
		chunkAnchors: map[string]bool{
			"heading_path":  in.anchorHeadingPath,
			"clause_id":     in.anchorClauseID,
			"row_number":    in.anchorRowNumber,
			"speaker":       in.anchorSpeaker,
			"sentence_idx":  in.anchorSentenceIndex,
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

	chunks := ChunkDocument(in.space.UUID, format, sourceURI, docUnits, ChunkingOptions{
		Mode:         in.segmentMode,
		ChunkSize:    in.chunkSize,
		ChunkOverlap: in.chunkOverlap,
		Separators:   separators,
		Anchors: ChunkAnchors{
			HeadingPath:  in.anchorHeadingPath,
			ClauseID:     in.anchorClauseID,
			RowNumber:    in.anchorRowNumber,
			Speaker:      in.anchorSpeaker,
			SentenceIndex: in.anchorSentenceIndex,
		},
	})
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
		return out, maskedChunks, nil
	}

	out.language = detectLanguage(maskedChunks)
	summaryCount, contentCount := countChunkKinds(maskedChunks)
	out.summaryCount = summaryCount
	out.chunkCount = contentCount
	out.totalChunks = len(maskedChunks)

	records := make([]vectorstore.VectorRecord, 0, len(maskedChunks))
	for _, chunk := range maskedChunks {
		embedding := HashEmbedding(chunk.Content, 32)
		meta := make(map[string]any, len(chunk.Metadata)+2)
		for k, v := range chunk.Metadata {
			meta[k] = v
		}
		meta["chunk_kind"] = chunk.Kind
		meta["content_hash"] = ContentHash(chunk.Content)
		records = append(records, vectorstore.VectorRecord{
			ChunkID:   chunk.ID,
			Embedding: embedding,
			Metadata:  meta,
		})
	}

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
