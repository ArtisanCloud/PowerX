package instrumentation

import (
	"context"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

// Instruments exposes sandbox metrics.
type Instruments struct {
	deployLatency metric.Float64Histogram
	testPassRate  metric.Float64Histogram
}

// NewInstruments registers OTel instruments.
func NewInstruments(component string) *Instruments {
	if component == "" {
		component = "plugin_sandbox"
	}
	meter := otel.Meter(component)
	deploy, err := meter.Float64Histogram("sandbox.deploy.duration_ms")
	if err != nil {
		logger.Warn(context.Background(), "create sandbox deploy histogram failed: "+err.Error())
	}
	passRate, err := meter.Float64Histogram("sandbox.test.pass_rate")
	if err != nil {
		logger.Warn(context.Background(), "create sandbox pass rate histogram failed: "+err.Error())
	}
	return &Instruments{
		deployLatency: deploy,
		testPassRate:  passRate,
	}
}

// RecordDeploy captures deploy latency.
func (i *Instruments) RecordDeploy(ctx context.Context, duration time.Duration) {
	if i == nil || i.deployLatency == nil {
		return
	}
	i.deployLatency.Record(ctx, float64(duration.Milliseconds()))
}

// RecordTest captures pass/fail outcome.
func (i *Instruments) RecordTest(ctx context.Context, outcome string) {
	if i == nil || i.testPassRate == nil {
		return
	}
	value := 0.0
	if strings.EqualFold(outcome, "passed") || strings.EqualFold(outcome, "success") {
		value = 1.0
	}
	i.testPassRate.Record(ctx, value)
}
