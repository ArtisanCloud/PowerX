package plugin_import

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	auditpkg "github.com/ArtisanCloud/PowerX/pkg/corex/audit"
	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/plugin_release"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/plugin_release"
	pxlog "github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/google/uuid"
	"gorm.io/datatypes"
)

// Options configure the import service.
type Options struct {
	Repo     *repo.ImportRepository
	Auditor  auditpkg.Auditor
	AuditSvc auditpkg.Service
	Now      func() time.Time
}

// Service orchestrates import submissions and risk scoring.
type Service struct {
	repo     *repo.ImportRepository
	auditor  auditpkg.Auditor
	auditSvc auditpkg.Service
	now      func() time.Time
}

// NewService returns a new Service if repo is provided, otherwise nil.
func NewService(opts Options) *Service {
	if opts.Repo == nil {
		return nil
	}
	if opts.Auditor == nil {
		opts.Auditor = auditpkg.Noop{}
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	return &Service{
		repo:     opts.Repo,
		auditor:  opts.Auditor,
		auditSvc: opts.AuditSvc,
		now:      now,
	}
}

// Submit registers a new import job and returns calculated risk summary.
func (s *Service) Submit(ctx context.Context, req ImportRequest) (*ImportResult, error) {
	if s == nil {
		return nil, errors.New("import service is not configured")
	}
	if strings.TrimSpace(req.PackageName) == "" {
		return nil, errors.New("packageName is required")
	}
	run := &model.PluginImportRun{
		TenantID:    strings.TrimSpace(req.TenantID),
		PackageName: strings.TrimSpace(req.PackageName),
		Vendor:      strings.TrimSpace(req.Vendor),
		SourceURI:   strings.TrimSpace(req.SourceURI),
		Checksum:    strings.TrimSpace(req.Checksum),
		SubmittedBy: strings.TrimSpace(req.SubmittedBy),
		Status:      model.PluginImportStatusPending,
		RiskLevel:   model.PluginImportRiskLow,
	}

	riskLevel, findings := evaluateRisk(req)
	run.RiskLevel = riskLevel
	run.Findings = marshalJSON(findings)
	run.LicenseSummary = marshalJSON(req.LicenseInventory)
	run.VulnSummary = marshalJSON(map[string]any{
		"total": req.VulnerabilityCount,
	})

	if err := s.repo.Create(ctx, run); err != nil {
		return nil, err
	}

	result := &ImportResult{
		Run:         run,
		RiskLevel:   riskLevel,
		Findings:    findings,
		NextActions: nextStepsForRisk(riskLevel),
	}

	s.emitAudit(ctx, run.TenantID, "PLUGIN_IMPORT_SUBMIT", strings.ToUpper(run.Status), run.UUID.String(), map[string]any{
		"package":  run.PackageName,
		"risk":     riskLevel,
		"tenantId": run.TenantID,
	})

	return result, nil
}

// Get retrieves the import run by UUID string.
func (s *Service) Get(ctx context.Context, id string) (*model.PluginImportRun, error) {
	if s == nil {
		return nil, errors.New("import service is not configured")
	}
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("invalid import id: %w", err)
	}
	return s.repo.FindByID(ctx, uid)
}

func evaluateRisk(req ImportRequest) (string, map[string]any) {
	score := 0
	if len(req.HighRiskLicenses) > 0 {
		score += 2
	}
	if req.VulnerabilityCount > 0 {
		score++
	}
	if !req.HasSPDX {
		score++
	}

	risk := model.PluginImportRiskLow
	switch {
	case score >= 3:
		risk = model.PluginImportRiskHigh
	case score == 2:
		risk = model.PluginImportRiskMedium
	}

	findings := map[string]any{
		"highRiskLicenses":   req.HighRiskLicenses,
		"vulnerabilityCount": req.VulnerabilityCount,
		"hasSpdx":            req.HasSPDX,
		"notes":              req.Notes,
		"metadata":           req.Metadata,
	}
	return risk, findings
}

func nextStepsForRisk(risk string) []string {
	switch risk {
	case model.PluginImportRiskHigh:
		return []string{
			"Escalate to security for manual approval.",
			"Request vendor SPDX + SBOM attachments.",
		}
	case model.PluginImportRiskMedium:
		return []string{
			"Capture remediation plan for flagged items.",
			"Schedule sandbox validation before approval.",
		}
	default:
		return []string{
			"Proceed with automated template adaptation.",
		}
	}
}

func marshalJSON(v any) datatypes.JSON {
	if v == nil {
		return datatypes.JSON("{}")
	}
	data, err := json.Marshal(v)
	if err != nil {
		return datatypes.JSON("{}")
	}
	return datatypes.JSON(data)
}

func (s *Service) emitAudit(ctx context.Context, tenantID, operation, outcome, resourceID string, meta map[string]any) {
	if s.auditSvc == nil {
		return
	}
	event := &dbm.AuditEvent{
		OccurredAt:    s.now().UTC(),
		TenantID:      tenantNumeric(tenantID),
		Source:        "plugin_import",
		Operation:     operation,
		ResourceType:  "plugin_import",
		ResourceID:    resourceID,
		CorrelationID: resourceID,
		Outcome:       strings.ToUpper(outcome),
		Severity:      severityFromOutcome(outcome),
		Meta:          marshalJSON(meta),
	}
	if err := s.auditSvc.Emit(ctx, event); err != nil {
		pxlog.WarnF(ctx, "[plugin_import] emit audit failed: %v", err)
	}
}

func tenantNumeric(tenantID string) uint64 {
	id := strings.TrimSpace(tenantID)
	if id == "" {
		return 0
	}
	value, err := strconv.ParseUint(id, 10, 64)
	if err != nil {
		return 0
	}
	return value
}

func severityFromOutcome(outcome string) string {
	switch strings.ToLower(outcome) {
	case "pending":
		return "INFO"
	case "approved":
		return "INFO"
	default:
		return "WARN"
	}
}
