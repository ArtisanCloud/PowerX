package distribution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/plugin_release/instrumentation"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/plugin_release"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/plugin_release"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

var (
	// ErrFeatureDisabled indicates the offline distribution feature flag is disabled.
	ErrFeatureDisabled = errors.New("plugin_release.distribution: feature disabled")
	// ErrInvalidInput indicates missing or malformed parameters.
	ErrInvalidInput = errors.New("plugin_release.distribution: invalid input")
	// ErrCandidateNotFound indicates the referenced release candidate cannot be found.
	ErrCandidateNotFound = errors.New("plugin_release.distribution: candidate not found")
	// ErrPackageNotFound indicates the offline package cannot be located.
	ErrPackageNotFound = errors.New("plugin_release.distribution: offline package not found")
	// ErrListingNotFound indicates the marketplace listing cannot be located.
	ErrListingNotFound = errors.New("plugin_release.distribution: marketplace listing not found")
)

const (
	defaultEscalationThreshold = 2
	defaultArtifactRetention   = 30 * 24 * time.Hour
	defaultReviewSLA           = 48 * time.Hour
)

// Options configures distribution behaviour.
type Options struct {
	OfflineBucket       string
	OfflinePrefix       string
	EscalationThreshold int
	ArtifactRetention   time.Duration
	ReviewSLA           time.Duration
	FeatureEnabled      bool
}

// Dependencies enumerates runtime collaborators.
type Dependencies struct {
	Candidates  *repo.ReleaseCandidateRepository
	Repository  *repo.DistributionRepository
	ImportRuns  *repo.ImportRepository
	Instruments *instrumentation.Instruments
	Validator   *Validator
	Hooks       AuditHooks
	Clock       func() time.Time
}

// Service orchestrates offline package storage, marketplace review and tenant imports.
type Service struct {
	candidates  *repo.ReleaseCandidateRepository
	repo        *repo.DistributionRepository
	importRuns  *repo.ImportRepository
	instruments *instrumentation.Instruments
	validator   *Validator
	hooks       AuditHooks
	clock       func() time.Time
	opts        Options
}

// StoreOfflinePackageInput captures data required to persist an offline package.
type StoreOfflinePackageInput struct {
	CandidateID          uuid.UUID
	PackageURI           string
	Content              []byte
	Checksum             string
	SignatureFingerprint string
	Dependencies         []string
	LicenseReport        map[string]any
	Actor                string
}

// SubmitListingInput captures payload for listing submissions.
type SubmitListingInput struct {
	OfflinePackageID uint64
	Channel          string
	Pricing          map[string]any
	SupportPolicy    map[string]any
	SubmissionForm   map[string]any
	Actor            string
}

// ReviewListingInput captures review decisions.
type ReviewListingInput struct {
	ListingID uint64
	Decision  string
	Comment   string
	Actor     string
}

// OfflineImportInput captures tenant import requests.
type OfflineImportInput struct {
	TenantUUID      string
	PackageURI      string
	Checksum        string
	DryRun          bool
	LicenseAccepted bool
	Actor           string
}

// ImportJob tracks offline import progress for tenant APIs.
type ImportJob struct {
	ID          string     `json:"jobId"`
	TenantUUID  string     `json:"tenant_uuid"`
	PackageURI  string     `json:"packageUri"`
	Checksum    string     `json:"checksum"`
	Status      string     `json:"status"`
	DryRun      bool       `json:"dryRun"`
	StartedAt   time.Time  `json:"startedAt"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
}

// ListMarketplaceListingsInput controls listing filters and pagination.
type ListMarketplaceListingsInput struct {
	Page    int
	Size    int
	Status  string
	Channel string
}

// MarketplaceListingPage wraps paginated responses.
type MarketplaceListingPage struct {
	Items []models.MarketplaceListing `json:"items"`
	Total int64                       `json:"total"`
	Page  int                         `json:"page"`
	Size  int                         `json:"size"`
}

// NewService constructs the distribution service with validated dependencies.
func NewService(deps Dependencies, opts Options) *Service {
	if deps.Candidates == nil || deps.Repository == nil {
		panic("distribution service requires candidate and repository dependencies")
	}
	if deps.Validator == nil {
		deps.Validator = NewValidator()
	}
	if deps.Hooks == nil {
		deps.Hooks = NoopAuditHooks{}
	}
	if deps.Clock == nil {
		deps.Clock = time.Now
	}
	if opts.EscalationThreshold <= 0 {
		opts.EscalationThreshold = defaultEscalationThreshold
	}
	if opts.ArtifactRetention <= 0 {
		opts.ArtifactRetention = defaultArtifactRetention
	}
	if opts.ReviewSLA <= 0 {
		opts.ReviewSLA = defaultReviewSLA
	}
	return &Service{
		candidates:  deps.Candidates,
		repo:        deps.Repository,
		instruments: deps.Instruments,
		validator:   deps.Validator,
		hooks:       deps.Hooks,
		clock:       deps.Clock,
		opts:        opts,
		importRuns:  deps.ImportRuns,
	}
}

// StoreOfflinePackage records an offline package entry backing future distribution.
func (s *Service) StoreOfflinePackage(ctx context.Context, input StoreOfflinePackageInput) (*models.OfflineDistributionPackage, error) {
	if !s.opts.FeatureEnabled {
		return nil, ErrFeatureDisabled
	}
	if input.CandidateID == uuid.Nil {
		return nil, ErrInvalidInput
	}
	normalizedChecksum := normalizeChecksum(input.Checksum)
	if normalizedChecksum == "" {
		return nil, ErrInvalidInput
	}
	if err := s.validator.VerifyChecksum(input.Content, normalizedChecksum); err != nil {
		return nil, err
	}
	signature := strings.TrimSpace(input.SignatureFingerprint)
	if signature == "" {
		signature = normalizedChecksum
	}
	if err := s.validator.RequireSignature(signature); err != nil {
		return nil, err
	}
	if input.LicenseReport == nil {
		input.LicenseReport = map[string]any{"status": "pending"}
	}
	if err := s.validator.VerifyLicense(input.LicenseReport); err != nil {
		return nil, err
	}
	candidate, err := s.candidates.GetByUUID(ctx, input.CandidateID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCandidateNotFound
		}
		return nil, err
	}
	if candidate == nil {
		return nil, ErrCandidateNotFound
	}

	packageURI := strings.TrimSpace(input.PackageURI)
	if packageURI == "" {
		packageURI = s.buildPackageURI(candidate)
	}

	dependenciesJSON, err := encodeStringSlice(input.Dependencies)
	if err != nil {
		return nil, err
	}
	licenseJSON, err := encodeMap(input.LicenseReport)
	if err != nil {
		return nil, err
	}

	now := s.clock()
	slaDeadline := now.Add(s.opts.ReviewSLA)
	pkg := &models.OfflineDistributionPackage{
		ReleaseCandidateID:   candidate.ID,
		PackageURI:           packageURI,
		Checksum:             normalizedChecksum,
		SignatureFingerprint: signature,
		Dependencies:         dependenciesJSON,
		LicenseReport:        licenseJSON,
		Status:               models.OfflinePackageStatusSubmitted,
		SLADeadline:          &slaDeadline,
		CreatedBy:            input.Actor,
		UpdatedBy:            input.Actor,
	}
	pkg, err = s.repo.CreatePackage(ctx, pkg)
	if err != nil {
		return nil, err
	}
	if s.instruments != nil && s.opts.ReviewSLA > 0 {
		s.instruments.DistributionSLASeconds.Record(ctx, s.opts.ReviewSLA.Seconds())
	}
	s.hooks.OnOfflinePackageStored(ctx, pkg)
	return pkg, nil
}

// SubmitListing creates a marketplace listing for the supplied offline package.
func (s *Service) SubmitListing(ctx context.Context, input SubmitListingInput) (*models.MarketplaceListing, error) {
	if !s.opts.FeatureEnabled {
		return nil, ErrFeatureDisabled
	}
	if input.OfflinePackageID == 0 || strings.TrimSpace(input.Channel) == "" {
		return nil, ErrInvalidInput
	}
	pkg, err := s.repo.GetPackageByID(ctx, input.OfflinePackageID)
	if err != nil {
		return nil, err
	}
	if pkg == nil {
		return nil, ErrPackageNotFound
	}

	pricingJSON, err := encodeMap(input.Pricing)
	if err != nil {
		return nil, err
	}
	supportJSON, err := encodeMap(input.SupportPolicy)
	if err != nil {
		return nil, err
	}
	formJSON, err := encodeMap(input.SubmissionForm)
	if err != nil {
		return nil, err
	}

	listing := &models.MarketplaceListing{
		OfflinePackageID: pkg.ID,
		Channel:          strings.ToLower(strings.TrimSpace(input.Channel)),
		Pricing:          pricingJSON,
		SupportPolicy:    supportJSON,
		SubmissionForm:   formJSON,
		ReviewStatus:     models.MarketplaceListingStatusPending,
		CreatedBy:        input.Actor,
		UpdatedBy:        input.Actor,
	}
	listing, err = s.repo.CreateListing(ctx, listing)
	if err != nil {
		return nil, err
	}
	s.hooks.OnListingSubmitted(ctx, listing)
	return listing, nil
}

// ReviewListing applies the review decision and escalates as needed.
func (s *Service) ReviewListing(ctx context.Context, input ReviewListingInput) (*models.MarketplaceListing, error) {
	if !s.opts.FeatureEnabled {
		return nil, ErrFeatureDisabled
	}
	if input.ListingID == 0 {
		return nil, ErrInvalidInput
	}
	listing, err := s.repo.GetListingByID(ctx, input.ListingID)
	if err != nil {
		return nil, err
	}
	if listing == nil {
		return nil, ErrListingNotFound
	}

	decision := strings.ToLower(strings.TrimSpace(input.Decision))
	var escalatedAt *time.Time
	switch decision {
	case "need_fix":
		listing.ReviewStatus = models.MarketplaceListingStatusNeedFix
		listing.ReviewCount++
		if listing.ReviewCount >= s.opts.EscalationThreshold {
			now := s.clock()
			escalatedAt = &now
		}
	case "approved":
		listing.ReviewStatus = models.MarketplaceListingStatusApproved
		listing.ReviewCount++
	case "rejected":
		listing.ReviewStatus = models.MarketplaceListingStatusRejected
		listing.ReviewCount++
	default:
		return nil, ErrInvalidInput
	}

	if err := s.repo.UpdateListingReview(ctx, listing.ID, listing.ReviewStatus, listing.ReviewCount, escalatedAt); err != nil {
		return nil, err
	}

	if decision == "approved" {
		now := s.clock()
		if err := s.repo.UpdateListingNotification(ctx, listing.ID, nil, &now); err != nil {
			return nil, err
		}
		s.hooks.OnListingApproved(ctx, listing)
	} else if escalatedAt != nil {
		s.hooks.OnListingEscalated(ctx, listing)
	}

	return s.repo.GetListingByID(ctx, listing.ID)
}

// StartOfflineImport persists a tenant-scoped import job.
func (s *Service) StartOfflineImport(ctx context.Context, input OfflineImportInput) (*ImportJob, error) {
	if !s.opts.FeatureEnabled {
		return nil, ErrFeatureDisabled
	}
	if strings.TrimSpace(input.TenantUUID) == "" || strings.TrimSpace(input.PackageURI) == "" || strings.TrimSpace(input.Checksum) == "" {
		return nil, ErrInvalidInput
	}
	if !input.LicenseAccepted {
		return nil, fmt.Errorf("%w: license must be accepted before importing", ErrInvalidInput)
	}
	if err := s.validator.VerifyChecksum(nil, input.Checksum); err != nil {
		return nil, err
	}

	now := s.clock()
	completedAt := now
	run := &models.PluginImportRun{
		PowerUUIDModel: coremodel.PowerUUIDModel{UUID: uuid.New()},
		TenantUUID:     strings.TrimSpace(input.TenantUUID),
		PackageName:    strings.TrimSpace(input.PackageURI),
		SourceURI:      strings.TrimSpace(input.PackageURI),
		Checksum:       normalizeChecksum(input.Checksum),
		SubmittedBy:    strings.TrimSpace(input.Actor),
		Status:         models.PluginImportStatusApproved,
		RiskLevel:      models.PluginImportRiskLow,
		CompletedAt:    &completedAt,
	}
	if s.importRuns == nil {
		return nil, errors.New("plugin release import repository unavailable")
	}
	if err := s.importRuns.Create(ctx, run); err != nil {
		return nil, err
	}
	job := ImportJob{
		ID:          run.UUID.String(),
		TenantUUID:  strings.TrimSpace(input.TenantUUID),
		PackageURI:  strings.TrimSpace(input.PackageURI),
		Checksum:    normalizeChecksum(input.Checksum),
		Status:      "completed",
		DryRun:      input.DryRun,
		StartedAt:   now,
		CompletedAt: &completedAt,
	}

	s.hooks.OnOfflineImportStarted(ctx, job)
	return &job, nil
}

// GetImportJob returns the job by identifier.
func (s *Service) GetImportJob(ctx context.Context, tenantUUID, jobID string) (*ImportJob, error) {
	id := strings.TrimSpace(jobID)
	if id == "" {
		return nil, ErrInvalidInput
	}
	jobUUID, err := uuid.Parse(id)
	if err != nil {
		return nil, ErrInvalidInput
	}
	if s.importRuns == nil {
		return nil, errors.New("plugin release import repository unavailable")
	}
	run, err := s.importRuns.FindByTenantUUID(ctx, strings.TrimSpace(tenantUUID), jobUUID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &ImportJob{ID: run.UUID.String(), TenantUUID: run.TenantUUID, PackageURI: run.SourceURI, Checksum: run.Checksum, Status: "completed", StartedAt: run.CreatedAt, CompletedAt: run.CompletedAt}, nil
}

// ListMarketplaceListings returns listings with pagination data.
func (s *Service) ListMarketplaceListings(ctx context.Context, input ListMarketplaceListingsInput) (*MarketplaceListingPage, error) {
	if !s.opts.FeatureEnabled {
		return nil, ErrFeatureDisabled
	}
	filter := repo.MarketplaceListingFilter{
		Page:    input.Page,
		Size:    input.Size,
		Status:  input.Status,
		Channel: input.Channel,
	}
	items, total, err := s.repo.ListMarketplaceListings(ctx, filter)
	if err != nil {
		return nil, err
	}
	return &MarketplaceListingPage{
		Items: items,
		Total: total,
		Page:  filter.Page,
		Size:  filter.Size,
	}, nil
}

func (s *Service) buildPackageURI(candidate *models.PluginReleaseCandidate) string {
	bucket := strings.TrimSpace(s.opts.OfflineBucket)
	if bucket == "" {
		bucket = "plugin-release-artifacts"
	}
	prefix := strings.Trim(strings.TrimSpace(s.opts.OfflinePrefix), "/")
	filename := fmt.Sprintf("%s-%s-%d.pxp",
		sanitizeSegment(candidate.PluginID),
		sanitizeSegment(candidate.Version),
		s.clock().UnixNano(),
	)
	objectPath := filename
	if prefix != "" {
		objectPath = path.Join(prefix, filename)
	}
	return fmt.Sprintf("s3://%s/%s", bucket, objectPath)
}

func encodeStringSlice(values []string) (datatypes.JSON, error) {
	if len(values) == 0 {
		return datatypes.JSON([]byte("[]")), nil
	}
	payload, err := json.Marshal(values)
	if err != nil {
		return nil, err
	}
	return datatypes.JSON(payload), nil
}

func encodeMap(data map[string]any) (datatypes.JSON, error) {
	if data == nil {
		data = map[string]any{}
	}
	payload, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	return datatypes.JSON(payload), nil
}

func sanitizeSegment(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "artifact"
	}
	replacer := strings.NewReplacer(" ", "-", "/", "-", "\\", "-", ":", "-")
	return replacer.Replace(strings.ToLower(trimmed))
}
