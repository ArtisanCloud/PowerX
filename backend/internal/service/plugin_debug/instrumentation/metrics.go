package instrumentation

import (
	"context"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

// Instruments exposes OpenTelemetry handles for plugin debug metrics.
type Instruments struct {
	meter               metric.Meter
	HotReloadDuration   metric.Float64Histogram
	HotReloadFailures   metric.Int64Counter
	HostVersionMismatch metric.Int64Counter
}

// NewInstruments initializes the debug instrumentation set.
func NewInstruments(component string) *Instruments {
	meter := otel.Meter(component)

	latency, err := meter.Float64Histogram("debug.hot_reload.duration_ms")
	if err != nil {
		logger.ErrorF(context.Background(), "create debug hot reload duration histogram failed: %v", err)
	}

	failures, err := meter.Int64Counter("debug.hot_reload.failure_total")
	if err != nil {
		logger.ErrorF(context.Background(), "create debug hot reload failure counter failed: %v", err)
	}

	mismatch, err := meter.Int64Counter("debug.host.version_mismatch_total")
	if err != nil {
		logger.ErrorF(context.Background(), "create debug host version mismatch counter failed: %v", err)
	}

	return &Instruments{
		meter:               meter,
		HotReloadDuration:   latency,
		HotReloadFailures:   failures,
		HostVersionMismatch: mismatch,
	}
}

// RecordLatency observes the latency histogram (milliseconds).
func (i *Instruments) RecordLatency(ctx context.Context, duration time.Duration) {
	if i == nil || i.HotReloadDuration == nil {
		return
	}
	i.HotReloadDuration.Record(ctx, float64(duration.Milliseconds()))
}

// RecordFailure increments the failure counter.
func (i *Instruments) RecordFailure(ctx context.Context) {
	if i == nil || i.HotReloadFailures == nil {
		return
	}
	i.HotReloadFailures.Add(ctx, 1)
}

// RecordVersionMismatch increments host mismatch counter.
func (i *Instruments) RecordVersionMismatch(ctx context.Context) {
	if i == nil || i.HostVersionMismatch == nil {
		return
	}
	i.HostVersionMismatch.Add(ctx, 1)
}
