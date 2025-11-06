package plugin_release

import (
	"context"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/plugin_release/instrumentation"
	"github.com/ArtisanCloud/PowerX/internal/service/plugin_release/local"
	audit "github.com/ArtisanCloud/PowerX/pkg/corex/audit"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/plugin_release"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/plugin_release"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
)

// FeatureFlagOptions controls runtime feature toggles for plugin release flows.
type FeatureFlagOptions struct {
	EnableLocalInstall bool
}

// LocalInstallOptions encapsulates configuration for local hotload sessions.
type LocalInstallOptions struct {
	SessionTTL        time.Duration
	MaxArtifactSizeMB int
}

// Options aggregates module-level dependencies shared across sub-services.
type Options struct {
	FeatureFlags FeatureFlagOptions
	LocalInstall LocalInstallOptions
	Auditor      audit.Auditor
}

// Service aggregates repositories and instrumentation for plugin release orchestration.
type Service struct {
	candidates     *repo.ReleaseCandidateRepository
	plans          *repo.ReleasePlanRepository
	distribution   *repo.DistributionRepository
	localSessions  *repo.LocalInstallSessionRepository
	instruments    *instrumentation.Instruments
	tracerProvider *instrumentation.TracerProvider
	localInstall   *local.InstallService
	options        Options
}

// NewService wires plugin release repositories with shared instrumentation.
func NewService(
	candidates *repo.ReleaseCandidateRepository,
	plans *repo.ReleasePlanRepository,
	distribution *repo.DistributionRepository,
	localSessions *repo.LocalInstallSessionRepository,
	componentName string,
	opts Options,
) *Service {
	if opts.Auditor == nil {
		opts.Auditor = audit.Noop{}
	}

	svc := &Service{
		candidates:     candidates,
		plans:          plans,
		distribution:   distribution,
		localSessions:  localSessions,
		instruments:    instrumentation.NewInstruments(componentName),
		tracerProvider: instrumentation.NewTracerProvider(componentName),
		options:        opts,
	}
	localHooks := local.NewAuditHooks(opts.Auditor)
	svc.localInstall = local.NewInstallService(local.InstallServiceDeps{
		Repository:    localSessions,
		Auditor:       opts.Auditor,
		Clock:         time.Now,
		AuditObserver: localHooks,
	}, local.Options{
		SessionTTL:        opts.LocalInstall.SessionTTL,
		MaxArtifactSizeMB: opts.LocalInstall.MaxArtifactSizeMB,
		FeatureEnabled:    opts.FeatureFlags.EnableLocalInstall,
	})
	return svc
}

// CreateCandidate wraps repository create with instrumentation.
func (s *Service) CreateCandidate(ctx context.Context, candidate *models.PluginReleaseCandidate) (*models.PluginReleaseCandidate, error) {
	ctx, span := s.tracerProvider.StartSpan(ctx, "Service.CreateCandidate")
	defer span.End()

	result, err := s.candidates.CreateCandidate(ctx, candidate)
	if err != nil {
		logger.ErrorF(ctx, "create candidate failed: %v", err)
		return nil, err
	}
	return result, nil
}

// RecordPipelineDuration observes pipeline latency metrics.
func (s *Service) RecordPipelineDuration(ctx context.Context, duration time.Duration) {
	if s.instruments != nil {
		s.instruments.PipelineDuration.Record(ctx, duration.Seconds())
	}
}

// LocalInstall exposes the hotload session service.
func (s *Service) LocalInstall() *local.InstallService {
	return s.localInstall
}
