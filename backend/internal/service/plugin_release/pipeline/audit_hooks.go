package pipeline

import (
	"context"
	"encoding/json"

	audit "github.com/ArtisanCloud/PowerX/pkg/corex/audit"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/plugin_release"
)

// AuditHooks captures important lifecycle transitions for compliance.
type AuditHooks interface {
	CandidateSubmitted(ctx context.Context, candidate *models.PluginReleaseCandidate)
	GatesEvaluated(ctx context.Context, candidate *models.PluginReleaseCandidate, result GateResult)
	PlanGenerated(ctx context.Context, plan *models.ReleasePlan)
}

// NoopAuditHooks prevents nil pointer invocations.
type NoopAuditHooks struct{}

func (NoopAuditHooks) CandidateSubmitted(context.Context, *models.PluginReleaseCandidate) {}
func (NoopAuditHooks) GatesEvaluated(context.Context, *models.PluginReleaseCandidate, GateResult) {
}
func (NoopAuditHooks) PlanGenerated(context.Context, *models.ReleasePlan) {}

type auditHook struct {
	auditor audit.Auditor
}

// NewAuditHooks builds hooks backed by the shared auditor.
func NewAuditHooks(auditor audit.Auditor) AuditHooks {
	if auditor == nil {
		return NoopAuditHooks{}
	}
	return &auditHook{auditor: auditor}
}

func (h *auditHook) CandidateSubmitted(ctx context.Context, candidate *models.PluginReleaseCandidate) {
	if candidate == nil {
		return
	}
	h.logEvent(ctx, "plugin_release.candidate.submitted", map[string]any{
		"candidate_id": candidate.UUID.String(),
		"tenant_id":    candidate.TenantID,
		"plugin_id":    candidate.PluginID,
		"version":      candidate.Version,
	})
}

func (h *auditHook) GatesEvaluated(ctx context.Context, candidate *models.PluginReleaseCandidate, result GateResult) {
	if candidate == nil {
		return
	}
	payload := map[string]any{
		"candidate_id": candidate.UUID.String(),
		"status":       result.Status,
	}
	if len(result.Violations) > 0 {
		payload["violations"] = result.Violations
	}
	h.logEvent(ctx, "plugin_release.candidate.gates", payload)
}

func (h *auditHook) PlanGenerated(ctx context.Context, plan *models.ReleasePlan) {
	if plan == nil {
		return
	}
	h.logEvent(ctx, "plugin_release.plan.generated", map[string]any{
		"plan_id":              plan.ID,
		"release_candidate_id": plan.ReleaseCandidateID,
		"window_start":         plan.WindowStart,
		"window_end":           plan.WindowEnd,
	})
}

func (h *auditHook) logEvent(ctx context.Context, action string, payload map[string]any) {
	if h.auditor == nil {
		return
	}
	h.auditor.LogAPI(ctx, action, 200, 0)
	// payload reserved for future structured sinks; serialize to ensure deterministic behaviour.
	if payload != nil {
		_, _ = json.Marshal(payload)
	}
}
