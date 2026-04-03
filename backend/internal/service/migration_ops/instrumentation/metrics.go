package instrumentation

import (
	"context"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

type Recorder struct {
	total   metric.Int64Counter
	errors  metric.Int64Counter
	latency metric.Float64Histogram
}

func NewRecorder(component string) *Recorder {
	meter := otel.Meter(component)
	total, err := meter.Int64Counter("powerx_ops_migration_total")
	if err != nil {
		logger.ErrorF(context.Background(), "create migration total counter failed: %v", err)
	}
	errorsCounter, err := meter.Int64Counter("powerx_ops_migration_error_total")
	if err != nil {
		logger.ErrorF(context.Background(), "create migration error counter failed: %v", err)
	}
	latency, err := meter.Float64Histogram("powerx_ops_migration_latency_ms")
	if err != nil {
		logger.ErrorF(context.Background(), "create migration latency histogram failed: %v", err)
	}
	return &Recorder{total: total, errors: errorsCounter, latency: latency}
}

func (r *Recorder) Observe(ctx context.Context, operation string, startedAt time.Time, err error) {
	if r == nil {
		return
	}
	attrs := metric.WithAttributes(attribute.String("operation", operation))
	if r.total != nil {
		r.total.Add(ctx, 1, attrs)
	}
	if r.latency != nil {
		r.latency.Record(ctx, float64(time.Since(startedAt).Milliseconds()), attrs)
	}
	if err != nil && r.errors != nil {
		r.errors.Add(ctx, 1, attrs)
	}
}
