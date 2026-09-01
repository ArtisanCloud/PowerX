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

type bridgeStateResponse struct {
	Agent  bridgeAgentResponse   `json:"agent"`
	Health healthSummaryResponse `json:"health"`
	Events []bridgeEventResponse `json:"events"`
}

type bridgeAgentResponse struct {
	ID         string                 `json:"id"`
	TenantUUID string                 `json:"tenant_uuid"`
	Alias      string                 `json:"alias"`
	Status     string                 `json:"status"`
	Capacity   bridgeCapacityResponse `json:"capacity"`
	UpdatedAt  string                 `json:"updated_at"`
}

type bridgeCapacityResponse struct {
	Default int32  `json:"default"`
	Current int32  `json:"current"`
	Max     *int32 `json:"max,omitempty"`
}

type bridgeEventResponse struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	From        string `json:"from_status"`
	To          string `json:"to_status"`
	Reason      string `json:"reason"`
	TriggeredBy string `json:"triggered_by"`
	TraceID     string `json:"trace_id"`
	OccurredAt  string `json:"occurred_at"`
}

type bridgeLifecycleResponse struct {
	Agent bridgeAgentResponse `json:"agent"`
	Event bridgeEventResponse `json:"event"`
}

type bridgeControlRequest struct {
	Reason  string `json:"reason"`
	TraceID string `json:"trace_id"`
}

type bridgeRebalanceRequest struct {
	TargetCapacityInstances int32  `json:"target_capacity_instances" binding:"required"`
	Reason                  string `json:"reason"`
	TraceID                 string `json:"trace_id"`
}

func fromBridgeState(state *agent_lifecycle.AgentBridgeState) bridgeStateResponse {
	if state == nil {
		return bridgeStateResponse{}
	}
	resp := bridgeStateResponse{
		Agent: fromBridgeAgent(state.Agent),
	}
	if state.Health != nil {
		resp.Health = fromHealthSummary(state.Health)
	}
	for _, evt := range state.Events {
		resp.Events = append(resp.Events, fromBridgeEvent(evt))
	}
	return resp
}

func fromBridgeAgent(agent *agent_lifecycle.Agent) bridgeAgentResponse {
	if agent == nil {
		return bridgeAgentResponse{}
	}
	return bridgeAgentResponse{
		ID:         agent.ID.String(),
		TenantUUID: agent.TenantUUID,
		Alias:      agent.Alias,
		Status:     agent.Status,
		Capacity: bridgeCapacityResponse{
			Default: agent.DefaultCapacityInstances,
			Current: agent.CurrentCapacityInstances,
			Max:     agent.MaxCapacityInstances,
		},
		UpdatedAt: agent.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func fromBridgeEvent(evt agent_lifecycle.BridgeLifecycleEvent) bridgeEventResponse {
	return bridgeEventResponse{
		ID:          evt.ID,
		Type:        evt.Type,
		From:        evt.From,
		To:          evt.To,
		Reason:      evt.Reason,
		TriggeredBy: evt.TriggeredBy,
		TraceID:     evt.TraceID,
		OccurredAt:  evt.OccurredAt,
	}
}

func fromLifecycleResult(result *agent_lifecycle.LifecycleResult) bridgeLifecycleResponse {
	if result == nil {
		return bridgeLifecycleResponse{}
	}
	resp := bridgeLifecycleResponse{
		Agent: fromBridgeAgent(result.Agent),
	}
	if result.Event != nil {
		resp.Event = bridgeEventResponse{
			ID:          result.Event.UUID.String(),
			Type:        result.Event.EventType,
			From:        result.Event.FromStatus,
			To:          result.Event.ToStatus,
			Reason:      result.Event.Reason,
			TriggeredBy: result.Event.TriggeredBy,
			TraceID:     result.Event.TraceID,
			OccurredAt:  result.Event.OccurredAt.UTC().Format(time.RFC3339),
		}
	}
	return resp
}
