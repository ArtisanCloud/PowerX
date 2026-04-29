package instrumentation

import (
	"context"

	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

// Instruments provides handles for plugin release metrics.
type Instruments struct {
	meter                  metric.Meter
	LocalHotloadLatency    metric.Float64Histogram
	PipelineDuration       metric.Float64Histogram
	CanaryRollbackLatency  metric.Float64Histogram
	DistributionSLASeconds metric.Float64Histogram
	CanaryPhaseLatency     metric.Float64Histogram
	CanaryErrorRate        metric.Float64Histogram
	CanaryRollbackCounter  metric.Int64Counter
}

// NewInstruments constructs the metric instruments using OTel meter provider.
func NewInstruments(component string) *Instruments {
	logCtx := logger.WithLogFields(context.Background(), map[string]interface{}{"module": "plugin_release.instrumentation"})
	meter := otel.Meter(component)
	hotload, err := meter.Float64Histogram("plugin_release.hotload.latency_ms")
	if err != nil {
		logger.ErrorF(logCtx, "create hotload histogram failed: %v", err)
	}
	pipeline, err := meter.Float64Histogram("plugin_release.pipeline.duration_seconds")
	if err != nil {
		logger.ErrorF(logCtx, "create pipeline histogram failed: %v", err)
	}
	rollback, err := meter.Float64Histogram("plugin_release.canary.rollback_seconds")
	if err != nil {
		logger.ErrorF(logCtx, "create rollback histogram failed: %v", err)
	}
	distribution, err := meter.Float64Histogram("plugin_release.distribution.sla_seconds")
	if err != nil {
		logger.ErrorF(logCtx, "create distribution histogram failed: %v", err)
	}
	phaseLatency, err := meter.Float64Histogram("plugin_release.canary.phase_duration_seconds")
	if err != nil {
		logger.ErrorF(logCtx, "create canary phase histogram failed: %v", err)
	}
	errorRate, err := meter.Float64Histogram("plugin_release.canary.error_rate")
	if err != nil {
		logger.ErrorF(logCtx, "create canary error_rate histogram failed: %v", err)
	}
	rollbackCounter, err := meter.Int64Counter("plugin_release.canary.rollback_total")
	if err != nil {
		logger.ErrorF(logCtx, "create canary rollback counter failed: %v", err)
	}
	return &Instruments{
		meter:                  meter,
		LocalHotloadLatency:    hotload,
		PipelineDuration:       pipeline,
		CanaryRollbackLatency:  rollback,
		DistributionSLASeconds: distribution,
		CanaryPhaseLatency:     phaseLatency,
		CanaryErrorRate:        errorRate,
		CanaryRollbackCounter:  rollbackCounter,
	}
}
