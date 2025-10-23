package agent

import (
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/agent_lifecycle"
)

type healthSummaryResponse struct {
	Status            string                `json:"status"`
	HealthScore       int32                 `json:"health_score"`
	UpdatedAt         string                `json:"updated_at"`
	WindowDurationSec int32                 `json:"window_duration_sec"`
	Metrics           healthMetricsResponse `json:"metrics"`
	Recommendations   []string              `json:"recommendations,omitempty"`
	AnomalyTraceIDs   []string              `json:"anomaly_trace_ids,omitempty"`
}

type healthMetricsResponse struct {
	ThroughputPerMin float64 `json:"throughput_per_min"`
	SuccessRate      float64 `json:"success_rate"`
	P95LatencyMs     int32   `json:"p95_latency_ms"`
	ResourceUtilPct  float64 `json:"resource_util_pct"`
	ErrorRate        float64 `json:"error_rate"`
}

type healthHistoryResponse struct {
	Snapshots []healthSummaryResponse `json:"snapshots"`
}

func fromHealthSummary(summary *agent_lifecycle.HealthSummary) healthSummaryResponse {
	if summary == nil {
		return healthSummaryResponse{}
	}
	resp := healthSummaryResponse{
		Status:            summary.Status,
		HealthScore:       summary.HealthScore,
		UpdatedAt:         summary.UpdatedAt.UTC().Format(time.RFC3339),
		WindowDurationSec: summary.WindowDurationSec,
		Metrics: healthMetricsResponse{
			ThroughputPerMin: summary.Metrics.ThroughputPerMin,
			SuccessRate:      summary.Metrics.SuccessRate,
			P95LatencyMs:     summary.Metrics.P95LatencyMs,
			ResourceUtilPct:  summary.Metrics.ResourceUtilPct,
			ErrorRate:        summary.Metrics.ErrorRate,
		},
		Recommendations: append([]string(nil), summary.Recommendations...),
		AnomalyTraceIDs: append([]string(nil), summary.Metrics.AnomalyTraceIDs...),
	}
	return resp
}
