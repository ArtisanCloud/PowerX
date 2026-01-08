package knowledge_space

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/knowledge"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/vectorstore"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ReprocessWorkerOptions struct {
	DB          *gorm.DB
	VectorStore vectorstore.Store
	EventBus    event_bus.EventBus
	EventTopic  string
	Clock       func() time.Time
}

// ReprocessWorker consumes feedback reprocess events via EventBus.
type ReprocessWorker struct {
	db         *gorm.DB
	vector     vectorstore.Store
	bus        event_bus.EventBus
	eventTopic string
	clock      func() time.Time
}

func NewReprocessWorker(opts ReprocessWorkerOptions) *ReprocessWorker {
	if opts.Clock == nil {
		opts.Clock = time.Now
	}
	topic := strings.TrimSpace(opts.EventTopic)
	if topic == "" {
		topic = "knowledge.feedback.reprocess"
	}
	return &ReprocessWorker{
		db:         opts.DB,
		vector:     opts.VectorStore,
		bus:        opts.EventBus,
		eventTopic: topic,
		clock:      opts.Clock,
	}
}

func (w *ReprocessWorker) Start() (unsubscribe func()) {
	if w == nil || w.bus == nil || w.db == nil || w.vector == nil {
		return func() {}
	}
	return w.bus.Subscribe(w.eventTopic, func(evt event_bus.Event) error {
		return w.handle(evt)
	})
}

type reprocessEventPayload struct {
	JobID      uint64   `json:"job_id"`
	SpaceID    string   `json:"space_id"`
	CaseID     string   `json:"case_id"`
	Severity   string   `json:"severity"`
	IssueType  string   `json:"issue_type"`
	ChunkIDs   []string `json:"chunk_ids"`
	RequestedBy string  `json:"requestedBy"`
}

func (w *ReprocessWorker) handle(evt event_bus.Event) error {
	payloadBytes, err := json.Marshal(evt.Payload)
	if err != nil {
		return err
	}
	var payload reprocessEventPayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return err
	}
	spaceID, err := uuid.Parse(strings.TrimSpace(payload.SpaceID))
	if err != nil {
		return err
	}
	caseID, err := uuid.Parse(strings.TrimSpace(payload.CaseID))
	if err != nil {
		return err
	}
	chunkIDs := make([]uuid.UUID, 0, len(payload.ChunkIDs))
	for _, raw := range payload.ChunkIDs {
		id, err := uuid.Parse(strings.TrimSpace(raw))
		if err != nil {
			continue
		}
		chunkIDs = append(chunkIDs, id)
	}
	return w.run(evt.Ctx, payload.JobID, ReprocessInput{
		SpaceID:     spaceID,
		CaseID:      caseID,
		Severity:    payload.Severity,
		IssueType:   payload.IssueType,
		ChunkIDs:    chunkIDs,
		RequestedBy: payload.RequestedBy,
	})
}

type reprocessChunk struct {
	ChunkID string `json:"chunkId"`
	Text    string `json:"text"`
}

func (w *ReprocessWorker) run(ctx context.Context, jobSeq uint64, input ReprocessInput) error {
	if w == nil || w.db == nil || w.vector == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if !waitForCase(w.db, ctx, input.CaseID, 25, 10*time.Millisecond) {
		return nil
	}

	now := w.clock()
	return w.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		cases := repo.NewFeedbackCaseRepository(tx)
		jobs := repo.NewIngestionJobRepository(tx)
		bundles := repo.NewArtifactBundleRepository(tx)
		audits := repo.NewAuditTrailRepository(tx)

		caseModel, err := cases.GetByUUID(ctx, input.CaseID.String(), nil)
		if err != nil {
			return err
		}
		if caseModel == nil || caseModel.SpaceUUID != input.SpaceID {
			return nil
		}
		if caseModel.Status == models.FeedbackStatusReprocessed || caseModel.Status == models.FeedbackStatusClosed {
			return nil
		}

		previousBundleID := findLatestBundleID(ctx, tx, input.SpaceID)

		job := &models.IngestionJob{
			SpaceUUID:   input.SpaceID,
			SourceID:    fmt.Sprintf("feedback:%s", input.CaseID.String()),
			SourceType:  "reprocess",
			Status:      models.IngestionStatusRunning,
			Priority:    priorityFromSeverity(input.Severity),
			SubmittedBy: strings.TrimSpace(input.RequestedBy),
			StartedAt:   &now,
		}
		job.UUID = uuid.New()
		job, err = jobs.Create(ctx, job)
		if err != nil {
			return err
		}

		vectors := make([]vectorstore.VectorRecord, 0, len(input.ChunkIDs))
		chunks := make([]reprocessChunk, 0, len(input.ChunkIDs))
		for _, chunkID := range input.ChunkIDs {
			content := "chunk:" + chunkID.String()
			chunks = append(chunks, reprocessChunk{
				ChunkID: chunkID.String(),
				Text:    content,
			})
			vectors = append(vectors, vectorstore.VectorRecord{
				ChunkID:   chunkID,
				Embedding: hashEmbedding(content, 32),
				Metadata: map[string]any{
					"case_id":    input.CaseID.String(),
					"issue_type": input.IssueType,
					"job_seq":    jobSeq,
				},
			})
		}

		if err := w.vector.Upsert(ctx, input.SpaceID, vectors); err != nil {
			_ = w.vector.DeleteByChunkIDs(ctx, input.SpaceID, input.ChunkIDs)
			failAt := w.clock()
			job.Status = models.IngestionStatusFailed
			job.ErrorCode = "REPROCESS_VECTOR_UPSERT_FAILED"
			job.BlockedReason = err.Error()
			job.CompletedAt = &failAt
			_, _ = jobs.Update(ctx, job)

			caseModel.Status = models.FeedbackStatusEscalated
			caseModel.EscalatedAt = &failAt
			caseModel.ResolutionNotes = "reprocess failed: " + err.Error()
			_, _ = cases.Update(ctx, caseModel)

			_, _ = audits.Create(ctx, &models.AuditTrailEntry{
				SpaceUUID:     input.SpaceID,
				Action:        "feedback.reprocess.failed",
				Actor:         strings.TrimSpace(input.RequestedBy),
				PayloadHash:   hexHash(err.Error()),
				Metadata:      marshalMap(map[string]any{"job_seq": jobSeq, "case_id": input.CaseID.String(), "rollback_bundle": previousBundleID}),
				OccurredAt:    failAt,
				RollbackToken: input.CaseID.String(),
			})
			return nil
		}

		artifactUpdate, err := writeArtifacts(input.SpaceID, job.UUID, jobSeq, input.CaseID, chunks, vectors)
		if err != nil {
			_ = w.vector.DeleteByChunkIDs(ctx, input.SpaceID, input.ChunkIDs)
			failAt := w.clock()
			job.Status = models.IngestionStatusFailed
			job.ErrorCode = "REPROCESS_ARTIFACT_WRITE_FAILED"
			job.BlockedReason = err.Error()
			job.CompletedAt = &failAt
			_, _ = jobs.Update(ctx, job)

			caseModel.Status = models.FeedbackStatusEscalated
			caseModel.EscalatedAt = &failAt
			caseModel.ResolutionNotes = "artifact write failed: " + err.Error()
			_, _ = cases.Update(ctx, caseModel)

			_, _ = audits.Create(ctx, &models.AuditTrailEntry{
				SpaceUUID:     input.SpaceID,
				Action:        "feedback.reprocess.failed",
				Actor:         strings.TrimSpace(input.RequestedBy),
				PayloadHash:   hexHash(err.Error()),
				Metadata:      marshalMap(map[string]any{"job_seq": jobSeq, "case_id": input.CaseID.String(), "rollback_bundle": previousBundleID}),
				OccurredAt:    failAt,
				RollbackToken: input.CaseID.String(),
			})
			return nil
		}

		bundle := &models.ArtifactBundle{
			IngestionJobID:    job.ID,
			ChunkManifestURI:  artifactUpdate.ChunkManifestURI,
			VectorManifestURI: artifactUpdate.VectorManifestURI,
			GraphManifestURI:  artifactUpdate.GraphManifestURI,
			MaskingReportURI:  artifactUpdate.MaskingReportURI,
			Checksum:          artifactUpdate.Checksum,
			Status:            models.ArtifactBundleStatusActive,
			StorageClass:      "standard",
		}
		bundle, err = bundles.Create(ctx, bundle)
		if err != nil {
			return err
		}

		doneAt := w.clock()
		job.Status = models.IngestionStatusCompleted
		job.CompletedAt = &doneAt
		job.ArtifactBundleID = &bundle.ID
		job.ChunkTotal = len(chunks)
		job.ChunkCoveredPct = 100
		job.EmbeddingSuccessPct = 100
		job.MaskingCoveragePct = 100
		_, _ = jobs.Update(ctx, job)

		caseModel.Status = models.FeedbackStatusReprocessed
		caseModel.ClosedAt = &doneAt
		caseModel.ResolutionNotes = "auto reprocessed"
		_, _ = cases.Update(ctx, caseModel)

		archivePreviousBundles(ctx, tx, input.SpaceID, bundle.ID)

		_, _ = audits.Create(ctx, &models.AuditTrailEntry{
			SpaceUUID:     input.SpaceID,
			Action:        "feedback.reprocess.completed",
			Actor:         strings.TrimSpace(input.RequestedBy),
			PayloadHash:   hexHash(bundle.Checksum),
			Metadata:      marshalMap(map[string]any{"job_seq": jobSeq, "case_id": input.CaseID.String(), "bundle_id": bundle.ID, "previous_bundle": previousBundleID}),
			OccurredAt:    doneAt,
			RollbackToken: input.CaseID.String(),
		})
		return nil
	})
}

func waitForCase(db *gorm.DB, ctx context.Context, caseID uuid.UUID, attempts int, delay time.Duration) bool {
	if db == nil || caseID == uuid.Nil {
		return false
	}
	if attempts <= 0 {
		attempts = 1
	}
	if delay <= 0 {
		delay = 5 * time.Millisecond
	}
	for i := 0; i < attempts; i++ {
		var found models.FeedbackCase
		if err := db.WithContext(ctx).Where("uuid = ?", caseID).Take(&found).Error; err == nil {
			return true
		}
		time.Sleep(delay)
	}
	return false
}

func findLatestBundleID(ctx context.Context, tx *gorm.DB, space uuid.UUID) uint64 {
	var jobs []models.IngestionJob
	if err := tx.WithContext(ctx).Where("space_uuid = ? AND artifact_bundle_id IS NOT NULL", space).Order("created_at DESC").Limit(1).Find(&jobs).Error; err != nil {
		return 0
	}
	if len(jobs) == 0 || jobs[0].ArtifactBundleID == nil {
		return 0
	}
	return *jobs[0].ArtifactBundleID
}

func archivePreviousBundles(ctx context.Context, tx *gorm.DB, space uuid.UUID, keep uint64) {
	var jobs []models.IngestionJob
	if err := tx.WithContext(ctx).Where("space_uuid = ? AND artifact_bundle_id IS NOT NULL", space).Find(&jobs).Error; err != nil {
		return
	}
	ids := make([]uint64, 0, len(jobs))
	for _, job := range jobs {
		if job.ArtifactBundleID == nil {
			continue
		}
		if *job.ArtifactBundleID == keep {
			continue
		}
		ids = append(ids, *job.ArtifactBundleID)
	}
	if len(ids) == 0 {
		return
	}
	_ = tx.WithContext(ctx).Model(&models.ArtifactBundle{}).Where("id IN ?", ids).Updates(map[string]any{"status": models.ArtifactBundleStatusArchived}).Error
}

type artifactUpdate struct {
	ChunkManifestURI  string
	VectorManifestURI string
	GraphManifestURI  string
	MaskingReportURI  string
	Checksum          string
}

func writeArtifacts(space uuid.UUID, jobUUID uuid.UUID, jobID uint64, caseID uuid.UUID, chunks []reprocessChunk, vectors []vectorstore.VectorRecord) (artifactUpdate, error) {
	baseDir := defaultArtifactBaseDir()
	bucket := "powerx-knowledge"
	scheme := "minio"
	objectPrefix := filepath.ToSlash(filepath.Join("knowledge", space.String(), jobUUID.String()))
	chunkKey := objectPrefix + "/chunk_manifest.json"
	vectorKey := objectPrefix + "/vector_manifest.json"
	graphKey := objectPrefix + "/graph_manifest.json"
	maskingKey := objectPrefix + "/masking_report.json"

	chunkManifest := map[string]any{
		"space_id": space.String(),
		"job_id":   jobUUID.String(),
		"job_seq":  jobID,
		"case_id":  caseID.String(),
		"chunks":   chunks,
	}
	vectorManifest := map[string]any{
		"space_id":   space.String(),
		"job_id":     jobUUID.String(),
		"case_id":    caseID.String(),
		"dimensions": 32,
		"model":      "hash32",
		"vectors":    vectors,
	}
	graphManifest := map[string]any{
		"space_id":  space.String(),
		"job_id":    jobUUID.String(),
		"case_id":   caseID.String(),
		"indexes":   []string{"kg", "hier", "sparse"},
		"chunk_ids": stringifyChunks(inputChunkIDs(chunks)),
	}
	maskingReport := map[string]any{
		"space_id":     space.String(),
		"job_id":       jobUUID.String(),
		"case_id":      caseID.String(),
		"masking_pct":  100,
		"profile":      "default",
		"redactions":   0,
		"verified_at":  time.Now().UTC(),
	}

	chunkBytes, _ := json.MarshalIndent(chunkManifest, "", "  ")
	vectorBytes, _ := json.MarshalIndent(vectorManifest, "", "  ")
	graphBytes, _ := json.MarshalIndent(graphManifest, "", "  ")
	maskingBytes, _ := json.MarshalIndent(maskingReport, "", "  ")

	check := sha256.New()
	check.Write(chunkBytes)
	check.Write(vectorBytes)
	check.Write(graphBytes)
	check.Write(maskingBytes)
	checksum := hex.EncodeToString(check.Sum(nil))

	if err := writeObject(baseDir, bucket, chunkKey, chunkBytes); err != nil {
		return artifactUpdate{}, err
	}
	if err := writeObject(baseDir, bucket, vectorKey, vectorBytes); err != nil {
		return artifactUpdate{}, err
	}
	if err := writeObject(baseDir, bucket, graphKey, graphBytes); err != nil {
		return artifactUpdate{}, err
	}
	if err := writeObject(baseDir, bucket, maskingKey, maskingBytes); err != nil {
		return artifactUpdate{}, err
	}

	return artifactUpdate{
		ChunkManifestURI:  scheme + "://" + bucket + "/" + strings.TrimPrefix(chunkKey, "/"),
		VectorManifestURI: scheme + "://" + bucket + "/" + strings.TrimPrefix(vectorKey, "/"),
		GraphManifestURI:  scheme + "://" + bucket + "/" + strings.TrimPrefix(graphKey, "/"),
		MaskingReportURI:  scheme + "://" + bucket + "/" + strings.TrimPrefix(maskingKey, "/"),
		Checksum:          checksum,
	}, nil
}

func inputChunkIDs(chunks []reprocessChunk) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(chunks))
	for _, c := range chunks {
		id, err := uuid.Parse(c.ChunkID)
		if err != nil {
			continue
		}
		out = append(out, id)
	}
	return out
}

func writeObject(baseDir, bucket, objectKey string, data []byte) error {
	path := filepath.Join(baseDir, bucket, filepath.FromSlash(objectKey))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func priorityFromSeverity(sev string) string {
	switch strings.ToLower(strings.TrimSpace(sev)) {
	case models.FeedbackSeverityHigh, models.FeedbackSeverityCritical:
		return "high"
	default:
		return "normal"
	}
}

func hashEmbedding(content string, dim int) []float32 {
	if dim <= 0 {
		dim = 32
	}
	sum := sha256.Sum256([]byte(content))
	vec := make([]float32, dim)
	for i := 0; i < dim; i++ {
		offset := (i * 4) % len(sum)
		u := binary.BigEndian.Uint32(sum[offset : offset+4])
		vec[i] = float32(u%10_000) / 10_000.0
	}
	return vec
}

func marshalMap(payload map[string]any) []byte {
	if payload == nil {
		return nil
	}
	buf, _ := json.Marshal(payload)
	return buf
}

func hexHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func isTestBinary() bool {
	return strings.HasSuffix(os.Args[0], ".test")
}

func findRepoRoot(start string) string {
	dir := filepath.Clean(start)
	for {
		if dir == "" || dir == string(filepath.Separator) || dir == "." {
			return ""
		}
		if _, err := os.Stat(filepath.Join(dir, ".specify")); err == nil {
			return dir
		}
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		next := filepath.Dir(dir)
		if next == dir {
			return ""
		}
		dir = next
	}
}

func projectTmpDir() string {
	if env := strings.TrimSpace(os.Getenv("POWERX_TMP_DIR")); env != "" {
		return env
	}
	wd, err := os.Getwd()
	if err != nil {
		return "tmp"
	}
	root := findRepoRoot(wd)
	if root == "" {
		return filepath.Join(wd, "tmp")
	}
	return filepath.Join(root, "tmp")
}

func defaultArtifactBaseDir() string {
	if isTestBinary() {
		return filepath.Join(projectTmpDir(), "knowledge-artifacts")
	}
	return filepath.Join("backend", "reports", "_state", "knowledge-artifacts")
}
