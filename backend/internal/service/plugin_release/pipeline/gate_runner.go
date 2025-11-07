package pipeline

import (
	"context"
	"strings"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/plugin_release"
)

// GateRunnerOptions tune guardrail heuristics.
type GateRunnerOptions struct {
	MinReleaseNotesLength int
	RequireCommitHash     bool
}

// GateRunner evaluates static heuristics before approval.
type GateRunner struct {
	opts GateRunnerOptions
}

// GateViolation describes a failed gate.
type GateViolation struct {
	Code    string
	Message string
	Owner   string
}

// GateReport summarises execution output.
type GateReport struct {
	Passed     bool
	Violations []GateViolation
	Score      map[string]float64
}

// NewGateRunner builds a gate runner with default thresholds.
func NewGateRunner(opts GateRunnerOptions) *GateRunner {
	if opts.MinReleaseNotesLength <= 0 {
		opts.MinReleaseNotesLength = 20
	}
	return &GateRunner{opts: opts}
}

// Run executes synchronous quality checks and returns a report.
func (r *GateRunner) Run(_ context.Context, candidate *models.PluginReleaseCandidate) GateReport {
	report := GateReport{
		Passed:     true,
		Violations: make([]GateViolation, 0, 2),
		Score: map[string]float64{
			"docs_completeness": 1.0,
			"security_score":    1.0,
		},
	}
	if len(strings.TrimSpace(candidate.ReleaseNotes)) < r.opts.MinReleaseNotesLength {
		report.Passed = false
		report.Violations = append(report.Violations, GateViolation{
			Code:    "DOCS_INCOMPLETE",
			Message: "release notes must describe change impact (>= 20 chars)",
			Owner:   "dev_team",
		})
		report.Score["docs_completeness"] = 0.2
	}
	if r.opts.RequireCommitHash && len(strings.TrimSpace(candidate.CommitHash)) < 7 {
		report.Passed = false
		report.Violations = append(report.Violations, GateViolation{
			Code:    "BUILD_METADATA_INVALID",
			Message: "commit hash must contain at least 7 characters",
			Owner:   "ci_pipeline",
		})
		report.Score["security_score"] = 0.4
	}
	if strings.Contains(strings.ToLower(candidate.ReleaseNotes), "todo") {
		report.Passed = false
		report.Violations = append(report.Violations, GateViolation{
			Code:    "QA_PENDING_TASKS",
			Message: "release notes indicate unresolved TODO items",
			Owner:   "qa_team",
		})
		report.Score["docs_completeness"] = 0.4
	}
	return report
}
