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
}

// NewInstruments constructs the metric instruments using OTel meter provider.
func NewInstruments(component string) *Instruments {
	meter := otel.Meter(component)
	hotload, err := meter.Float64Histogram("plugin_release.hotload.latency_ms")
	if err != nil {
		logger.Errorf(context.Background(), "create hotload histogram failed: %v", err)
	}
	pipeline, err := meter.Float64Histogram("plugin_release.pipeline.duration_seconds")
	if err != nil {
		logger.Errorf(context.Background(), "create pipeline histogram failed: %v", err)
	}
	rollback, err := meter.Float64Histogram("plugin_release.canary.rollback_seconds")
	if err != nil {
		logger.Errorf(context.Background(), "create rollback histogram failed: %v", err)
	}
	distribution, err := meter.Float64Histogram("plugin_release.distribution.sla_seconds")
	if err != nil {
		logger.Errorf(context.Background(), "create distribution histogram failed: %v", err)
	}
	return &Instruments{
		meter:                  meter,
		LocalHotloadLatency:    hotload,
		PipelineDuration:       pipeline,
		CanaryRollbackLatency:  rollback,
		DistributionSLASeconds: distribution,
	}
}
