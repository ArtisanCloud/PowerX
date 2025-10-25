package agent_lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	imnotify "github.com/ArtisanCloud/PowerX/internal/notifications/im"
	agentmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	agentrepo "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/repository"
	agentinstr "github.com/ArtisanCloud/PowerX/internal/service/agent_lifecycle/instrumentation"
	"github.com/google/uuid"
)

var (
	allowedSubscriptionStatuses = map[string]struct{}{
		"healthy":     {},
		"degraded":    {},
		"unavailable": {},
		"unknown":     {},
	}
	allowedMetricsKeys = map[string]struct{}{
		"throughput_per_min": {},
		"success_rate":       {},
		"p95_latency_ms":     {},
		"resource_util_pct":  {},
		"error_rate":         {},
	}
	defaultSubscriptionStatuses = []string{"degraded", "unavailable"}
	defaultSubscriptionMetrics  = []string{"error_rate", "p95_latency_ms", "success_rate"}
	statusSeverity              = map[string]int{
		"healthy":     0,
		"unknown":     1,
		"degraded":    2,
		"unavailable": 3,
	}
)

// RecordHealthSnapshot 写入健康快照并触发观测。
func (s *Service) RecordHealthSnapshot(ctx context.Context, input HealthInput) error {
	if s.health == nil {
		return errors.New("health repository not configured")
	}
	if input.AgentID == uuid.Nil {
		return errors.New("agent_id is required")
	}
	if input.WindowDuration <= 0 {
		return errors.New("window duration must be positive")
	}

	start := s.clock()
	ctx, traceID := agentinstr.EnsureTraceContext(ctx)
	if strings.TrimSpace(input.TraceID) != "" {
		traceID = input.TraceID
	}

	profile, err := s.profiles.GetByUUID(ctx, input.AgentID)
	if err != nil {
		if errors.Is(err, agentrepo.ErrAgentProfileNotFound) {
			return ErrAgentNotFound
		}
		return fmt.Errorf("load agent: %w", err)
	}

	ctx = agentinstr.WithTenant(ctx, profile.TenantID)

	window := input.WindowStartedAt
	if window.IsZero() {
		window = s.clock().UTC().Truncate(time.Minute)
	}

	metrics := normalizeMetrics(input.Metrics)
	score, derivedStatus := s.evaluateHealth(metrics)
	explicitStatus := normalizeStatus(input.Status)
	status := moreSevereStatus(derivedStatus, explicitStatus)

	recommendations := buildRecommendations(status, metrics, input.Recommendations)
	recsBytes, _ := json.Marshal(recommendations)
	traceBytes, _ := json.Marshal(metrics.AnomalyTraceIDs)

	record := &agentmodel.AgentHealthSnapshotRecord{
		AgentUUID:         profile.UUID,
		TenantID:          profile.TenantID,
		WindowStartedAt:   window,
		WindowDurationSec: int32(input.WindowDuration.Seconds()),
		ThroughputPerMin:  metrics.ThroughputPerMin,
		SuccessRate:       metrics.SuccessRate,
		P95LatencyMs:      metrics.P95LatencyMs,
		ResourceUtilPct:   metrics.ResourceUtilPct,
		ErrorRate:         metrics.ErrorRate,
		HealthScore:       score,
		Status:            status,
		AnomalyTraceIDs:   traceBytes,
		Recommendations:   recsBytes,
		TraceID:           traceID,
	}

	if _, err := s.health.Upsert(ctx, record); err != nil {
		return fmt.Errorf("upsert health snapshot: %w", err)
	}

	if s.instr != nil {
		s.instr.RecordHealthSnapshot(ctx, status, float64(score))
		s.instr.RecordHealthLatency(ctx, status, s.clock().Sub(start))
	}

	payload := map[string]any{
		"agent_id":            profile.UUID.String(),
		"tenant_id":           profile.TenantID,
		"status":              status,
		"health_score":        score,
		"recommendations":     recommendations,
		"window_started_at":   window.UTC().Format(time.RFC3339),
		"window_duration_sec": int32(input.WindowDuration.Seconds()),
		"metrics": map[string]any{
			"throughput_per_min": metrics.ThroughputPerMin,
			"success_rate":       metrics.SuccessRate,
			"p95_latency_ms":     metrics.P95LatencyMs,
			"resource_util_pct":  metrics.ResourceUtilPct,
			"error_rate":         metrics.ErrorRate,
			"anomaly_trace_ids":  metrics.AnomalyTraceIDs,
		},
		"trace_id": traceID,
	}

	cfg := s.subscriptionForProfile(profile)
	s.publishHealth(ctx, status, payload, traceID)

	if s.shouldSendAlert(status, cfg) && s.notifier != nil && s.instr.AllowHealthAlert(profile.UUID.String(), status, s.clock()) {
		msg := imnotify.Message{
			Title:    fmt.Sprintf("代理 %s 健康告警", profile.Alias),
			Content:  buildAlertContent(profile.Alias, status, score, recommendations),
			Severity: "critical",
			TraceID:  traceID,
			Metadata: map[string]any{
				"agent_id":     profile.UUID.String(),
				"tenant_id":    profile.TenantID,
				"status":       status,
				"health_score": score,
			},
		}
		if err := s.notifier.Send(ctx, msg); err != nil && s.instr != nil {
			s.instr.Logger(ctx).WarnF(ctx, "send IM notification failed: %v", err)
		}
	}

	return nil
}

func (s *Service) evaluateHealth(metrics HealthMetricsInput) (int32, string) {
	score := int32(100)

	score -= int32(metrics.ErrorRate * 100 * 0.4)
	if metrics.SuccessRate < 0.95 {
		score -= int32((0.95 - metrics.SuccessRate) * 100 * 0.3)
	}
	if metrics.P95LatencyMs > 1500 {
		score -= int32(float64(metrics.P95LatencyMs-1500) / 30)
	}
	if metrics.ResourceUtilPct > 0.9 {
		score -= int32((metrics.ResourceUtilPct - 0.9) * 100 * 0.5)
	}

	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}

	status := s.statusFromScore(score)
	if isZeroMetrics(metrics) {
		status = "unknown"
	}

	return score, status
}

func (s *Service) statusFromScore(score int32) string {
	if score >= s.config.HealthDegradedThreshold {
		return "healthy"
	}
	if score >= s.config.HealthUnavailableThreshold {
		return "degraded"
	}
	return "unavailable"
}

func (s *Service) shouldSendAlert(status string, cfg SubscriptionConfig) bool {
	status = normalizeStatus(status)
	if status == "" || status == "healthy" || status == "unknown" {
		return false
	}
	statuses := cfg.HealthStatuses
	if len(statuses) == 0 {
		statuses = defaultSubscriptionStatuses
	}
	for _, allowed := range statuses {
		if normalizeStatus(allowed) == status {
			return true
		}
	}
	return false
}

func buildAlertContent(alias, status string, score int32, recs []string) string {
	content := fmt.Sprintf("代理 %s 当前状态 %s，健康评分 %d。", alias, strings.ToUpper(status), score)
	if len(recs) > 0 {
		content += "建议：" + strings.Join(recs, "；")
	}
	return content
}

func normalizeMetrics(metrics HealthMetricsInput) HealthMetricsInput {
	if metrics.ThroughputPerMin < 0 {
		metrics.ThroughputPerMin = 0
	}
	if metrics.SuccessRate < 0 {
		metrics.SuccessRate = 0
	}
	if metrics.SuccessRate > 1 {
		metrics.SuccessRate = 1
	}
	if metrics.ResourceUtilPct < 0 {
		metrics.ResourceUtilPct = 0
	}
	if metrics.ResourceUtilPct > 1 {
		metrics.ResourceUtilPct = 1
	}
	if metrics.ErrorRate < 0 {
		metrics.ErrorRate = 0
	}
	if metrics.ErrorRate > 1 {
		metrics.ErrorRate = 1
	}
	if metrics.P95LatencyMs < 0 {
		metrics.P95LatencyMs = 0
	}
	return metrics
}

func isZeroMetrics(metrics HealthMetricsInput) bool {
	return metrics.ThroughputPerMin == 0 &&
		metrics.SuccessRate == 0 &&
		metrics.P95LatencyMs == 0 &&
		metrics.ResourceUtilPct == 0 &&
		metrics.ErrorRate == 0
}

func moreSevereStatus(a, b string) string {
	a = normalizeStatus(a)
	b = normalizeStatus(b)
	if b == "" {
		return a
	}
	if a == "" {
		return b
	}
	if statusSeverity[a] >= statusSeverity[b] {
		return a
	}
	return b
}

func normalizeStatus(val string) string {
	return strings.TrimSpace(strings.ToLower(val))
}

func buildRecommendations(status string, metrics HealthMetricsInput, manual []string) []string {
	result := make([]string, 0, len(manual)+4)
	seen := map[string]struct{}{}
	for _, item := range manual {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		result = append(result, item)
		seen[item] = struct{}{}
	}

	if metrics.ErrorRate > 0.3 {
		result = appendUnique(result, seen, "检查错误日志，定位主要失败原因")
	}
	if metrics.SuccessRate < 0.7 {
		result = appendUnique(result, seen, "评估最近变更，必要时执行回滚或降级")
	}
	if metrics.P95LatencyMs > 2000 {
		result = appendUnique(result, seen, "排查下游依赖与基础设施延迟")
	}
	if metrics.ResourceUtilPct > 0.9 {
		result = appendUnique(result, seen, "考虑扩容实例或调整负载策略")
	}

	if len(result) == 0 {
		switch status {
		case "degraded":
			result = appendUnique(result, seen, "持续观测指标变化，同时准备扩容或故障转移预案")
		case "unavailable":
			result = appendUnique(result, seen, "立即通知值班团队采取应急处置，评估是否执行回滚或隔离")
		}
	}

	return result
}

func appendUnique(list []string, seen map[string]struct{}, item string) []string {
	if _, ok := seen[item]; ok {
		return list
	}
	list = append(list, item)
	seen[item] = struct{}{}
	return list
}

// GetHealthSummary 返回最新健康摘要。
func (s *Service) GetHealthSummary(ctx context.Context, agentID uuid.UUID) (*HealthSummary, error) {
	if s.health == nil {
		return nil, errors.New("health repository not configured")
	}
	if agentID == uuid.Nil {
		return nil, errors.New("agent_id is required")
	}

	snapshots, err := s.health.ListByAgent(ctx, agentID, 0, 1)
	if err != nil {
		return nil, err
	}
	if len(snapshots) == 0 {
		return nil, ErrAgentNotFound
	}
	snap := snapshots[0]

	return convertSnapshot(&snap), nil
}

// ListHealthSnapshots 返回历史快照。
func (s *Service) ListHealthSnapshots(ctx context.Context, agentID uuid.UUID, rangeHours, limit int) ([]*HealthSummary, error) {
	if s.health == nil {
		return nil, errors.New("health repository not configured")
	}
	if limit <= 0 {
		limit = 50
	}

	snaps, err := s.health.ListByAgent(ctx, agentID, rangeHours, limit)
	if err != nil {
		return nil, err
	}

	result := make([]*HealthSummary, 0, len(snaps))
	for idx := range snaps {
		result = append(result, convertSnapshot(&snaps[idx]))
	}
	return result, nil
}

func convertSnapshot(snap *agentmodel.AgentHealthSnapshotRecord) *HealthSummary {
	if snap == nil {
		return nil
	}

	var anomalies []string
	if len(snap.AnomalyTraceIDs) > 0 {
		_ = json.Unmarshal(snap.AnomalyTraceIDs, &anomalies)
	}

	var recs []string
	if len(snap.Recommendations) > 0 {
		_ = json.Unmarshal(snap.Recommendations, &recs)
	}

	return &HealthSummary{
		AgentID:           snap.AgentUUID,
		Status:            snap.Status,
		HealthScore:       snap.HealthScore,
		UpdatedAt:         snap.WindowStartedAt,
		WindowDurationSec: snap.WindowDurationSec,
		Metrics: HealthMetricsInput{
			ThroughputPerMin: snap.ThroughputPerMin,
			SuccessRate:      snap.SuccessRate,
			P95LatencyMs:     snap.P95LatencyMs,
			ResourceUtilPct:  snap.ResourceUtilPct,
			ErrorRate:        snap.ErrorRate,
			AnomalyTraceIDs:  anomalies,
		},
		Recommendations: recs,
	}
}
