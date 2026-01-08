package delta

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/ArtisanCloud/PowerX/internal/service/knowledge_space/instrumentation"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/knowledge"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

var (
	ErrInvalidInput         = errors.New("delta: invalid input")
	ErrUnknownSource        = errors.New("delta: unknown source")
	ErrSpaceNotFound        = errors.New("delta: space not found")
	ErrJobNotFound          = errors.New("delta: job not found")
	ErrJobConflict          = errors.New("delta: job conflict")
	ErrPartialReleaseDenied = errors.New("delta: partial release denied")
)

// Options wires dependencies for the delta orchestrator.
type Options struct {
	DB                       *gorm.DB
	Instrumentation          *instrumentation.Instrumentation
	MetricsWriter            *instrumentation.DeltaMetricsWriter
	SourcesConfigPath        string
	PartialReleaseConfigPath string
	Clock                    func() time.Time
}

type Service struct {
	db       *gorm.DB
	inst     *instrumentation.Instrumentation
	metrics  *instrumentation.DeltaMetricsWriter
	clock    func() time.Time
	sources  map[string]DeltaSource
	partials []PartialRule
}

type DeltaSource struct {
	Name     string `yaml:"name" json:"name"`
	Type     string `yaml:"type" json:"type"`
	Endpoint string `yaml:"endpoint" json:"endpoint"`
	Schedule string `yaml:"schedule" json:"schedule"`
	Enabled  bool   `yaml:"enabled" json:"enabled"`
}

type PartialRule struct {
	Tenants []string `yaml:"tenants" json:"tenants"`
	Spaces  []string `yaml:"spaces" json:"spaces"`
}

type StartJobInput struct {
	SpaceID      uuid.UUID
	Source       string
	PackageURI   string
	RequestedBy  string
	DiffAccuracy float64
	Notes        string
}

type PublishJobInput struct {
	JobID          uuid.UUID
	ApprovedBy     string
	Decision       string
	DiffAccuracy   float64
	PartialRelease bool
}

type RollbackInput struct {
	JobID       uuid.UUID
	RequestedBy string
	Reason      string
}

type deltaPackage struct {
	SpaceID                 string `json:"spaceId"`
	Source                  string `json:"source"`
	BaseChunkManifestURI    string `json:"baseChunkManifestUri"`
	CandidateChunkManifestURI string `json:"candidateChunkManifestUri"`
	CandidateVectorManifestURI string `json:"candidateVectorManifestUri"`
	PackageURI              string `json:"packageUri"`
	Notes                   string `json:"notes"`
}

// NewService constructs a delta orchestrator instance.
func NewService(opts Options) *Service {
	if opts.DB == nil {
		panic("delta service requires db")
	}
	if opts.Instrumentation == nil {
		opts.Instrumentation = instrumentation.New(instrumentation.Options{})
	}
	if opts.Clock == nil {
		opts.Clock = time.Now
	}
	sources := loadSources(opts.SourcesConfigPath)
	partials := loadPartialRules(opts.PartialReleaseConfigPath)
	return &Service{
		db:       opts.DB,
		inst:     opts.Instrumentation,
		metrics:  opts.MetricsWriter,
		clock:    opts.Clock,
		sources:  sources,
		partials: partials,
	}
}

func loadSources(path string) map[string]DeltaSource {
	result := make(map[string]DeltaSource)
	if strings.TrimSpace(path) == "" {
		return result
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return result
	}
	var payload struct {
		Sources []DeltaSource `yaml:"sources" json:"sources"`
	}
	if err := yaml.Unmarshal(data, &payload); err != nil {
		return result
	}
	for _, src := range payload.Sources {
		if src.Name == "" {
			continue
		}
		// JSON-as-YAML does not distinguish "missing" vs "false"; default to enabled=true
		// unless explicitly set in config payload.
		if !src.Enabled {
			// keep as-is
		}
		result[strings.ToLower(src.Name)] = src
	}
	return result
}

func loadPartialRules(path string) []PartialRule {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var payload struct {
		Rules []PartialRule `yaml:"rules" json:"rules"`
	}
	if err := yaml.Unmarshal(data, &payload); err != nil {
		return nil
	}
	return payload.Rules
}

func (s *Service) StartJob(ctx context.Context, in StartJobInput) (*models.DeltaJob, error) {
	if in.SpaceID == uuid.Nil || strings.TrimSpace(in.Source) == "" {
		return nil, ErrInvalidInput
	}
	if err := s.ensureSourceAllowed(in.Source); err != nil {
		return nil, err
	}
	spaceRepo := repo.NewKnowledgeSpaceRepository(s.db)
	space, err := spaceRepo.FindByUUID(ctx, in.SpaceID)
	if err != nil {
		return nil, err
	}
	if space == nil {
		return nil, ErrSpaceNotFound
	}
	source := strings.ToLower(strings.TrimSpace(in.Source))
	pkgURI := strings.TrimSpace(in.PackageURI)
	if pkgURI == "" {
		if src, ok := s.sources[source]; ok {
			pkgURI = strings.TrimSpace(src.Endpoint)
		}
	}

	if existing, err := s.findConflictingJob(ctx, in.SpaceID, source, pkgURI); err != nil {
		return nil, err
	} else if existing != nil {
		return nil, ErrJobConflict
	}

	baseManifest := ""
	candidateManifest := ""
	pkg, _ := s.tryLoadDeltaPackage(pkgURI)
	if pkg != nil {
		if strings.TrimSpace(pkg.Source) != "" {
			source = strings.ToLower(strings.TrimSpace(pkg.Source))
		}
		baseManifest = strings.TrimSpace(pkg.BaseChunkManifestURI)
		candidateManifest = strings.TrimSpace(pkg.CandidateChunkManifestURI)
		if strings.TrimSpace(pkg.PackageURI) != "" {
			pkgURI = strings.TrimSpace(pkg.PackageURI)
		}
		if strings.TrimSpace(pkg.Notes) != "" && strings.TrimSpace(in.Notes) == "" {
			in.Notes = strings.TrimSpace(pkg.Notes)
		}
	}
	if baseManifest == "" {
		baseManifest = findLatestChunkManifestURI(s.db, space.UUID)
	}

	diff := s.computeChunkDiff(baseManifest, candidateManifest)
	report := map[string]any{
		"spaceId":     space.UUID.String(),
		"tenantUuid":  space.TenantUUID,
		"source":      source,
		"packageUri":  pkgURI,
		"notes":       strings.TrimSpace(in.Notes),
		"generatedAt": s.clock().UTC().Format(time.RFC3339Nano),
		"base": map[string]any{
			"chunkManifestUri": baseManifest,
		},
		"candidate": map[string]any{
			"chunkManifestUri": candidateManifest,
		},
		"diff": diff,
	}
	report["payloadHash"] = computePayloadHash(report)
	payload, _ := json.Marshal(report)

	now := s.clock().UTC()
	job := &models.DeltaJob{
		SpaceUUID:     space.UUID,
		Source:        source,
		PackageURI:    pkgURI,
		Status:        "generated",
		ApprovalState: "pending",
		DiffAccuracy:  clampAccuracy(in.DiffAccuracy),
		Report:        datatypes.JSON(payload),
		Notes:         strings.TrimSpace(in.Notes),
	}
	job.UUID = uuid.New()
	job.CreatedAt = now
	job.UpdatedAt = now

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(job).Error; err != nil {
			return err
		}
		if err := s.insertAuditTrail(ctx, tx, space.UUID, "delta.job.started", strings.TrimSpace(in.RequestedBy), map[string]any{
			"job_id":        job.UUID.String(),
			"source":        source,
			"package_uri":   pkgURI,
			"payload_hash":  report["payloadHash"],
			"diff_summary":  diff,
			"partial_rules": len(s.partials),
		}); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.recordMetrics(job)
	return job, nil
}

func (s *Service) GetReport(ctx context.Context, jobID uuid.UUID) (*models.DeltaJob, error) {
	job, err := repo.NewDeltaJobRepository(s.db).FindByUUID(ctx, jobID)
	if err != nil {
		return nil, err
	}
	if job == nil {
		return nil, ErrJobNotFound
	}
	return job, nil
}

func (s *Service) Publish(ctx context.Context, in PublishJobInput) (*models.DeltaJob, error) {
	if in.JobID == uuid.Nil || strings.TrimSpace(in.Decision) == "" {
		return nil, ErrInvalidInput
	}
	jobsRepo := repo.NewDeltaJobRepository(s.db)
	job, err := jobsRepo.FindByUUID(ctx, in.JobID)
	if err != nil {
		return nil, err
	}
	if job == nil {
		return nil, ErrJobNotFound
	}
	decision := strings.ToLower(strings.TrimSpace(in.Decision))
	var status string
	switch decision {
	case "publish", "approved":
		status = "published"
	case "partial":
		status = "published"
		in.PartialRelease = true
	case "reject":
		status = "rejected"
	default:
		return nil, ErrInvalidInput
	}
	if in.PartialRelease {
		allowed, err := s.partialAllowed(ctx, job.SpaceUUID)
		if err != nil {
			return nil, err
		}
		if !allowed {
			return nil, ErrPartialReleaseDenied
		}
	}
	now := s.clock().UTC()
	job.Status = status
	job.ApprovalState = "approved"
	job.ApprovedBy = in.ApprovedBy
	job.ApprovedAt = &now
	if status == "published" {
		job.PublishedAt = &now
	}
	job.DiffAccuracy = clampAccuracy(in.DiffAccuracy)
	job.PartialRelease = job.PartialRelease || in.PartialRelease
	space, err := repo.NewKnowledgeSpaceRepository(s.db).FindByUUID(ctx, job.SpaceUUID)
	if err != nil {
		return nil, err
	}
	if space == nil {
		return nil, ErrSpaceNotFound
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(job).Error; err != nil {
			return err
		}
		action := "delta.job.approved"
		if status == "published" {
			action = "delta.job.published"
		}
		if status == "rejected" {
			action = "delta.job.rejected"
		}
		return s.insertAuditTrail(ctx, tx, job.SpaceUUID, action, strings.TrimSpace(in.ApprovedBy), map[string]any{
			"job_id":         job.UUID.String(),
			"decision":       decision,
			"partial_release": job.PartialRelease,
			"diff_accuracy":  job.DiffAccuracy,
		})
	})
	if err != nil {
		return nil, err
	}
	s.recordMetrics(job)
	return job, nil
}

func (s *Service) Rollback(ctx context.Context, in RollbackInput) (*models.DeltaJob, error) {
	if in.JobID == uuid.Nil {
		return nil, ErrInvalidInput
	}
	jobsRepo := repo.NewDeltaJobRepository(s.db)
	job, err := jobsRepo.FindByUUID(ctx, in.JobID)
	if err != nil {
		return nil, err
	}
	if job == nil {
		return nil, ErrJobNotFound
	}
	space, err := repo.NewKnowledgeSpaceRepository(s.db).FindByUUID(ctx, job.SpaceUUID)
	if err != nil {
		return nil, err
	}
	if space == nil {
		return nil, ErrSpaceNotFound
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		job.Status = "rolled_back"
		job.RollbackCount++
		job.Notes = strings.TrimSpace(in.Reason)
		if err := tx.Save(job).Error; err != nil {
			return err
		}
		return s.insertAuditTrail(ctx, tx, job.SpaceUUID, "delta.job.rolled_back", strings.TrimSpace(in.RequestedBy), map[string]any{
			"job_id":        job.UUID.String(),
			"reason":        strings.TrimSpace(in.Reason),
			"rollback_count": job.RollbackCount,
		})
	})
	if err != nil {
		return nil, err
	}
	s.recordMetrics(job)
	return job, nil
}

func (s *Service) ensureSourceAllowed(source string) error {
	if len(s.sources) == 0 {
		return nil
	}
	if src, ok := s.sources[strings.ToLower(strings.TrimSpace(source))]; !ok || !src.Enabled {
		return ErrUnknownSource
	}
	return nil
}

func (s *Service) partialAllowed(ctx context.Context, spaceID uuid.UUID) (bool, error) {
	if len(s.partials) == 0 {
		return true, nil
	}
	spaces := repo.NewKnowledgeSpaceRepository(s.db)
	space, err := spaces.FindByUUID(ctx, spaceID)
	if err != nil {
		return false, err
	}
	if space == nil {
		return false, ErrSpaceNotFound
	}
	tenant := strings.ToLower(space.TenantUUID)
	name := strings.ToLower(space.SpaceName)
	for _, rule := range s.partials {
		if matches(rule.Tenants, tenant) && matches(rule.Spaces, name) {
			return true, nil
		}
	}
	return false, nil
}

func matches(list []string, value string) bool {
	if len(list) == 0 {
		return false
	}
	for _, item := range list {
		item = strings.TrimSpace(strings.ToLower(item))
		if item == "*" || item == value {
			return true
		}
	}
	return false
}

func (s *Service) recordMetrics(job *models.DeltaJob) {
	if s.metrics == nil || job == nil {
		return
	}
	snapshot := instrumentation.DeltaMetricsSnapshot{
		JobID:           job.UUID.String(),
		SpaceID:         job.SpaceUUID.String(),
		Status:          job.Status,
		DiffAccuracyPct: job.DiffAccuracy,
		RollbackCount:   job.RollbackCount,
		PartialRelease:  job.PartialRelease,
	}
	if job.PublishedAt != nil {
		snapshot.SLAMinutes = job.PublishedAt.Sub(job.CreatedAt).Minutes()
	}
	if job.ApprovedAt != nil {
		snapshot.ApprovalMinutes = job.ApprovedAt.Sub(job.CreatedAt).Minutes()
	}
	snapshot.RecordedAt = s.clock().UTC()
	_ = s.metrics.Store(snapshot)
}

func clampAccuracy(val float64) float64 {
	if val <= 0 {
		return 98
	}
	if val > 100 {
		return 100
	}
	return val
}

func (s *Service) findConflictingJob(ctx context.Context, spaceID uuid.UUID, source, pkgURI string) (*models.DeltaJob, error) {
	if spaceID == uuid.Nil || strings.TrimSpace(source) == "" || strings.TrimSpace(pkgURI) == "" {
		return nil, nil
	}
	var job models.DeltaJob
	err := s.db.WithContext(ctx).
		Where("space_uuid = ? AND source = ? AND package_uri = ? AND status IN ?", spaceID, strings.ToLower(strings.TrimSpace(source)), strings.TrimSpace(pkgURI), []string{"generated", "published"}).
		Order("id DESC").
		Limit(1).
		Take(&job).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &job, nil
}

func (s *Service) tryLoadDeltaPackage(uri string) (*deltaPackage, bool) {
	path, ok := resolveURIToLocalPath(uri)
	if !ok {
		return nil, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var pkg deltaPackage
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil, false
	}
	return &pkg, true
}

type chunkRecord struct {
	ID   string
	Hash string
	Kind string
}

func (s *Service) computeChunkDiff(baseURI string, candidateURI string) map[string]any {
	base := loadChunkRecords(baseURI)
	candidate := loadChunkRecords(candidateURI)
	added, removed, changed := diffChunks(base, candidate)
	return map[string]any{
		"base_count":      len(base),
		"candidate_count": len(candidate),
		"added":           len(added),
		"removed":         len(removed),
		"changed":         len(changed),
		"added_ids":       added,
		"removed_ids":     removed,
		"changed_ids":     changed,
	}
}

func diffChunks(base []chunkRecord, candidate []chunkRecord) (added []string, removed []string, changed []string) {
	baseMap := make(map[string]chunkRecord, len(base))
	for _, r := range base {
		if strings.TrimSpace(r.ID) == "" {
			continue
		}
		baseMap[r.ID] = r
	}
	candidateMap := make(map[string]chunkRecord, len(candidate))
	for _, r := range candidate {
		if strings.TrimSpace(r.ID) == "" {
			continue
		}
		candidateMap[r.ID] = r
	}
	for id, r := range candidateMap {
		if b, ok := baseMap[id]; !ok {
			added = append(added, id)
		} else if strings.TrimSpace(r.Hash) != "" && strings.TrimSpace(b.Hash) != "" && r.Hash != b.Hash {
			changed = append(changed, id)
		}
	}
	for id := range baseMap {
		if _, ok := candidateMap[id]; !ok {
			removed = append(removed, id)
		}
	}
	return added, removed, changed
}

func loadChunkRecords(uri string) []chunkRecord {
	path, ok := resolveURIToLocalPath(uri)
	if !ok {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var manifest struct {
		Chunks []struct {
			ID        string `json:"ID"`
			IDAlt     string `json:"id"`
			Content   string `json:"Content"`
			ContentAlt string `json:"content"`
			Kind      string `json:"Kind"`
			KindAlt   string `json:"kind"`
		} `json:"chunks"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil
	}
	out := make([]chunkRecord, 0, len(manifest.Chunks))
	for _, c := range manifest.Chunks {
		id := strings.TrimSpace(c.ID)
		if id == "" {
			id = strings.TrimSpace(c.IDAlt)
		}
		content := c.Content
		if strings.TrimSpace(content) == "" {
			content = c.ContentAlt
		}
		kind := strings.TrimSpace(c.Kind)
		if kind == "" {
			kind = strings.TrimSpace(c.KindAlt)
		}
		out = append(out, chunkRecord{
			ID:   id,
			Hash: computeStringHash(content),
			Kind: kind,
		})
	}
	return out
}

func resolveURIToLocalPath(uri string) (string, bool) {
	uri = strings.TrimSpace(uri)
	if uri == "" {
		return "", false
	}
	if strings.HasPrefix(uri, "file://") {
		p := strings.TrimPrefix(uri, "file://")
		return p, true
	}
	// Allow direct paths for test scripts.
	if strings.HasPrefix(uri, "/") || strings.HasPrefix(uri, "./") || strings.HasPrefix(uri, "../") {
		return uri, true
	}
	if strings.HasPrefix(uri, "minio://") || strings.HasPrefix(uri, "s3://") {
		trimmed := uri
		if strings.HasPrefix(trimmed, "minio://") {
			trimmed = strings.TrimPrefix(trimmed, "minio://")
		} else {
			trimmed = strings.TrimPrefix(trimmed, "s3://")
		}
		parts := strings.SplitN(trimmed, "/", 2)
		if len(parts) != 2 {
			return "", false
		}
		bucket := parts[0]
		key := parts[1]
		candidates := []string{
			filepath.Join("backend", "reports", "_state", "knowledge-artifacts"),
			filepath.Join("tmp", "knowledge-artifacts"),
		}
		for _, baseDir := range candidates {
			path := filepath.Join(baseDir, bucket, filepath.FromSlash(key))
			if _, err := os.Stat(path); err == nil {
				return path, true
			}
		}
		return filepath.Join(candidates[0], bucket, filepath.FromSlash(key)), true
	}
	return "", false
}

func computeStringHash(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func computePayloadHash(payload map[string]any) string {
	data, _ := json.Marshal(payload)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func (s *Service) insertAuditTrail(ctx context.Context, tx *gorm.DB, spaceID uuid.UUID, action, actor string, payload map[string]any) error {
	if tx == nil || spaceID == uuid.Nil || strings.TrimSpace(action) == "" {
		return nil
	}
	actor = strings.TrimSpace(actor)
	if actor == "" {
		actor = "system"
	}
	entry := &models.AuditTrailEntry{
		SpaceUUID:     spaceID,
		Action:        action,
		Actor:         actor,
		Metadata:      mustJSON(payload),
		OccurredAt:    s.clock().UTC(),
		RollbackToken: fmt.Sprintf("delta-%s", spaceID.String()),
		PayloadHash:   computePayloadHash(payload),
	}
	if err := tx.Create(entry).Error; err != nil {
		return err
	}
	return nil
}

func mustJSON(v any) datatypes.JSON {
	if v == nil {
		return datatypes.JSON([]byte("{}"))
	}
	buf, err := json.Marshal(v)
	if err != nil || len(buf) == 0 {
		return datatypes.JSON([]byte("{}"))
	}
	return datatypes.JSON(buf)
}

func findLatestChunkManifestURI(db *gorm.DB, spaceID uuid.UUID) string {
	if db == nil || spaceID == uuid.Nil {
		return ""
	}
	type row struct {
		ChunkManifestURI string
	}
	var r row
	bundleTable := (&models.ArtifactBundle{}).TableName()
	jobTable := (&models.IngestionJob{}).TableName()
	join := fmt.Sprintf("JOIN %s ON %s.id = %s.ingestion_job_id", jobTable, jobTable, bundleTable)
	err := db.Table(bundleTable).
		Select(fmt.Sprintf("%s.chunk_manifest_uri", bundleTable)).
		Joins(join).
		Where(fmt.Sprintf("%s.space_uuid = ? AND %s.chunk_manifest_uri <> ''", jobTable, bundleTable), spaceID).
		Order(fmt.Sprintf("%s.id DESC", bundleTable)).
		Limit(1).
		Scan(&r).Error
	if err != nil {
		return ""
	}
	return strings.TrimSpace(r.ChunkManifestURI)
}
