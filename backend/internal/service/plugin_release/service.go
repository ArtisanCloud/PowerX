package plugin_release

import (
	"context"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/plugin_release/distribution"
	"github.com/ArtisanCloud/PowerX/internal/service/plugin_release/instrumentation"
	"github.com/ArtisanCloud/PowerX/internal/service/plugin_release/local"
	"github.com/ArtisanCloud/PowerX/internal/service/plugin_release/pipeline"
	"github.com/ArtisanCloud/PowerX/internal/service/plugin_release/runtime"
	audit "github.com/ArtisanCloud/PowerX/pkg/corex/audit"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/plugin_release"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/plugin_release"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
)

// FeatureFlagOptions controls runtime feature toggles for plugin release flows.
type FeatureFlagOptions struct {
	EnableLocalInstall        bool
	EnableOfflineDistribution bool
}

// LocalInstallOptions encapsulates configuration for local hotload sessions.
type LocalInstallOptions struct {
	SessionTTL        time.Duration
	MaxArtifactSizeMB int
}

// RuntimeOptions controls rollout orchestration.
type RuntimeOptions struct {
	RollbackTimeout time.Duration
}

// Options aggregates module-level dependencies shared across sub-services.
type Options struct {
	FeatureFlags FeatureFlagOptions
	LocalInstall LocalInstallOptions
	Auditor      audit.Auditor
	Clock        func() time.Time
	EventBus     event_bus.EventBus
	Runtime      RuntimeOptions
	Distribution DistributionOptions
}

// DistributionOptions configures offline package storage.
type DistributionOptions struct {
	OfflineBucket       string
	OfflinePrefix       string
	EscalationThreshold int
	ArtifactRetention   time.Duration
	ReviewSLA           time.Duration
}

// Service aggregates repositories and instrumentation for plugin release orchestration.
type Service struct {
	candidates       *repo.ReleaseCandidateRepository
	plans            *repo.ReleasePlanRepository
	distributionRepo *repo.DistributionRepository
	localSessions    *repo.LocalInstallSessionRepository
	distributionSvc  *distribution.Service
	instruments      *instrumentation.Instruments
	tracerProvider   *instrumentation.TracerProvider
	localInstall     *local.InstallService
	runtime          *runtime.Service
	pipeline         *pipeline.Service
	options          Options
}

// NewService wires plugin release repositories with shared instrumentation.
func NewService(
	candidates *repo.ReleaseCandidateRepository,
	plans *repo.ReleasePlanRepository,
	distributionRepo *repo.DistributionRepository,
	localSessions *repo.LocalInstallSessionRepository,
	componentName string,
	opts Options,
) *Service {
	if opts.Auditor == nil {
		opts.Auditor = audit.Noop{}
	}
	if opts.Clock == nil {
		opts.Clock = time.Now
	}

	svc := &Service{
		candidates:       candidates,
		plans:            plans,
		distributionRepo: distributionRepo,
		localSessions:    localSessions,
		instruments:      instrumentation.NewInstruments(componentName),
		tracerProvider:   instrumentation.NewTracerProvider(componentName),
		options:          opts,
	}
	if opts.Runtime.RollbackTimeout <= 0 {
		opts.Runtime.RollbackTimeout = 5 * time.Minute
	}

	localHooks := local.NewAuditHooks(opts.Auditor)
	svc.localInstall = local.NewInstallService(local.InstallServiceDeps{
		Repository:    localSessions,
		Auditor:       opts.Auditor,
		Clock:         opts.Clock,
		AuditObserver: localHooks,
	}, local.Options{
		SessionTTL:        opts.LocalInstall.SessionTTL,
		MaxArtifactSizeMB: opts.LocalInstall.MaxArtifactSizeMB,
		FeatureEnabled:    opts.FeatureFlags.EnableLocalInstall,
	})
	pipelineHooks := pipeline.NewAuditHooks(opts.Auditor)
	svc.pipeline = pipeline.NewService(
		candidates,
		plans,
		pipeline.NewGateRunner(pipeline.GateRunnerOptions{
			MinReleaseNotesLength: 32,
			RequireCommitHash:     true,
		}),
		pipeline.Options{
			Hooks:       pipelineHooks,
			Instruments: svc.instruments,
			Clock:       opts.Clock,
		},
	)

	runtimeHooks := runtime.NewEventHooks(opts.EventBus)
	svc.runtime = runtime.NewService(runtime.Dependencies{
		Plans:       plans,
		Candidates:  candidates,
		Instruments: svc.instruments,
		Hooks:       runtimeHooks,
		Clock:       opts.Clock,
	}, runtime.Options{
		RollbackTimeout: opts.Runtime.RollbackTimeout,
	})

	distHooks := distribution.NewAuditHooks(opts.Auditor)
	svc.distributionSvc = distribution.NewService(distribution.Dependencies{
		Candidates:  candidates,
		Repository:  distributionRepo,
		Instruments: svc.instruments,
		Validator:   distribution.NewValidator(),
		Hooks:       distHooks,
		Clock:       opts.Clock,
	}, distribution.Options{
		OfflineBucket:       opts.Distribution.OfflineBucket,
		OfflinePrefix:       opts.Distribution.OfflinePrefix,
		EscalationThreshold: opts.Distribution.EscalationThreshold,
		ArtifactRetention:   opts.Distribution.ArtifactRetention,
		ReviewSLA:           opts.Distribution.ReviewSLA,
		FeatureEnabled:      opts.FeatureFlags.EnableOfflineDistribution,
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

// Pipeline exposes end-to-end release orchestration.
func (s *Service) Pipeline() *pipeline.Service {
	return s.pipeline
}

// Runtime exposes deployment runtime orchestration.
func (s *Service) Runtime() *runtime.Service {
	return s.runtime
}

// Distribution exposes offline package and marketplace orchestration.
func (s *Service) Distribution() *distribution.Service {
	if s == nil {
		return nil
	}
	return s.distributionSvc
}
