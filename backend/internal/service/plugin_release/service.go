package plugin_release

import (
	"context"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/plugin_release/instrumentation"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/plugin_release"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/plugin_release"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
)

// Service aggregates repositories and instrumentation for plugin release orchestration.
type Service struct {
	candidates     *repo.ReleaseCandidateRepository
	plans          *repo.ReleasePlanRepository
	distribution   *repo.DistributionRepository
	localSessions  *repo.LocalInstallSessionRepository
	instruments    *instrumentation.Instruments
	tracerProvider *instrumentation.TracerProvider
}

// NewService wires plugin release repositories with shared instrumentation.
func NewService(
	candidates *repo.ReleaseCandidateRepository,
	plans *repo.ReleasePlanRepository,
	distribution *repo.DistributionRepository,
	localSessions *repo.LocalInstallSessionRepository,
	componentName string,
) *Service {
	return &Service{
		candidates:     candidates,
		plans:          plans,
		distribution:   distribution,
		localSessions:  localSessions,
		instruments:    instrumentation.NewInstruments(componentName),
		tracerProvider: instrumentation.NewTracerProvider(componentName),
	}
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
