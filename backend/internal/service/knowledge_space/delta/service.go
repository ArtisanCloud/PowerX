package delta

import (
	"context"
	"encoding/json"
	"errors"
	"os"
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
		if !src.Enabled && src.Enabled != false {
			src.Enabled = true
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
	report := map[string]any{
		"source":      in.Source,
		"packageUri":  in.PackageURI,
		"notes":       in.Notes,
		"generatedAt": s.clock().UTC().Format(time.RFC3339Nano),
	}
	payload, _ := json.Marshal(report)
	job := &models.DeltaJob{
		SpaceUUID:     space.UUID,
		Source:        strings.ToLower(strings.TrimSpace(in.Source)),
		PackageURI:    in.PackageURI,
		Status:        "generated",
		ApprovalState: "pending",
		DiffAccuracy:  clampAccuracy(in.DiffAccuracy),
		Report:        datatypes.JSON(payload),
		Notes:         in.Notes,
	}
	created, err := repo.NewDeltaJobRepository(s.db).Create(ctx, job)
	if err != nil {
		return nil, err
	}
	s.recordMetrics(created)
	return created, nil
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
	repo := repo.NewDeltaJobRepository(s.db)
	job, err := repo.FindByUUID(ctx, in.JobID)
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
	if _, err := repo.Update(ctx, job); err != nil {
		return nil, err
	}
	s.recordMetrics(job)
	return job, nil
}

func (s *Service) Rollback(ctx context.Context, in RollbackInput) (*models.DeltaJob, error) {
	if in.JobID == uuid.Nil {
		return nil, ErrInvalidInput
	}
	repo := repo.NewDeltaJobRepository(s.db)
	job, err := repo.FindByUUID(ctx, in.JobID)
	if err != nil {
		return nil, err
	}
	if job == nil {
		return nil, ErrJobNotFound
	}
	job.Status = "rolled_back"
	job.RollbackCount++
	job.Notes = strings.TrimSpace(in.Reason)
	if _, err := repo.Update(ctx, job); err != nil {
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

// Audit helpers can be added later (no-op for now).
