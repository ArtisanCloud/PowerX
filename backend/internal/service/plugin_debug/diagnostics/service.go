package diagnostics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	ticketbridge "github.com/ArtisanCloud/PowerX/internal/service/integration/ticketbridge"
	auditpkg "github.com/ArtisanCloud/PowerX/pkg/corex/audit"
	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/plugin_debug"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/plugin_debug"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/google/uuid"
	"gorm.io/datatypes"
)

// Service aggregates report persistence, masking, and ticket workflows.
type Service struct {
	repo            *repo.ReportRepository
	auditSvc        auditpkg.Service
	now             func() time.Time
	template        *ReportTemplate
	masker          *Masker
	ticket          ticketbridge.Service
	fallbackLogBase string
}

// CreateRequest contains inputs for generating a report.
type CreateRequest struct {
	TenantID    uint64            `json:"tenantId"`
	PluginID    string            `json:"pluginId"`
	TraceID     string            `json:"traceId"`
	Notes       string            `json:"notes"`
	LogPointers []string          `json:"logPointers"`
	Summary     map[string]any    `json:"summary"`
	Metadata    map[string]string `json:"metadata"`
	Severity    string            `json:"severity"`
}

// ExportResponse returns a location for log bundles.
type ExportResponse struct {
	ReportID uuid.UUID `json:"reportId"`
	URL      string    `json:"url"`
}

// NewService constructs the diagnostics service.
func NewService(repository *repo.ReportRepository, auditSvc auditpkg.Service, clock func() time.Time, opts Options) *Service {
	if clock == nil {
		clock = time.Now
	}
	return &Service{
		repo:            repository,
		auditSvc:        auditSvc,
		now:             clock,
		template:        opts.Template,
		masker:          opts.Masker,
		ticket:          opts.TicketBridge,
		fallbackLogBase: opts.FallbackLogBase,
	}
}

// CreateReport persists a diagnostic request.
func (s *Service) CreateReport(ctx context.Context, req CreateRequest) (*model.DiagnosticReport, error) {
	if s == nil || s.repo == nil {
		return nil, errors.New("diagnostics repository unavailable")
	}
	if req.TenantID == 0 {
		return nil, errors.New("tenantId is required")
	}
	if strings.TrimSpace(req.PluginID) == "" {
		return nil, errors.New("pluginId is required")
	}

	masked := false
	summary := cloneMap(req.Summary)
	if maskedSummary, ok := s.masker.MaskMap(summary); ok {
		summary = maskedSummary
		masked = true
	}
	metadata := cloneStringMap(req.Metadata)
	if maskedMeta, ok := s.masker.MaskStringMap(metadata); ok {
		metadata = maskedMeta
		masked = true
	}
	logPointers := append([]string(nil), req.LogPointers...)
	if maskedLogs, ok := s.masker.MaskStrings(logPointers); ok {
		logPointers = maskedLogs
		masked = true
	}

	bundle := strings.Join(logPointers, ";")
	report := &model.DiagnosticReport{
		TenantID:      req.TenantID,
		PluginID:      strings.TrimSpace(req.PluginID),
		Status:        "processing",
		Summary:       marshalJSON(summary),
		Metadata:      marshalStringMap(metadata),
		LogBundleURI:  bundle,
		TraceID:       strings.TrimSpace(req.TraceID),
		Notes:         req.Notes,
		Masked:        masked,
		CorrelationID: uuid.New(),
	}
	created, err := s.repo.Create(ctx, report)
	if err != nil {
		return nil, err
	}

	if created.LogBundleURI == "" {
		fallback := s.fallbackLogURL(created.UUID)
		if fallback != "" {
			if err := s.repo.UpdateFields(ctx, created.UUID, map[string]any{"log_bundle_uri": fallback}); err == nil {
				created.LogBundleURI = fallback
			}
		}
	}
	return created, nil
}

// CompleteReport marks the report as finished with summary payload.
func (s *Service) CompleteReport(ctx context.Context, id uuid.UUID, summary map[string]any) error {
	if s == nil || s.repo == nil {
		return errors.New("diagnostics repository unavailable")
	}
	report, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	normalized := cloneMap(summary)
	masked := report.Masked
	if maskedSummary, ok := s.masker.MaskMap(normalized); ok {
		normalized = maskedSummary
		masked = true
	}
	rendered := normalized
	if s.template != nil {
		rendered = s.template.Render(normalized)
	}
	extras := map[string]any{
		"completed_at": s.now(),
	}
	if masked && !report.Masked {
		extras["masked"] = true
	}
	if s.ticket != nil {
		severity := extractSeverity(summary)
		ticket, ticketErr := s.ticket.CreateDiagnosticTicket(ctx, ticketbridge.DiagnosticTicketInput{
			TenantID:  report.TenantID,
			PluginID:  report.PluginID,
			ReportID:  report.UUID,
			Severity:  severity,
			Title:     fmt.Sprintf("Plugin %s debug report", report.PluginID),
			Summary:   normalized,
			LogBundle: report.LogBundleURI,
		})
		if ticketErr != nil {
			logger.WarnF(ctx, "[plugin_debug] create ticket failed report=%s err=%v", report.UUID, ticketErr)
		} else if ticket != nil {
			extras["ticket_ref"] = ticket.ID
			extras["ticket_url"] = ticket.URL
		}
	}
	if err := s.repo.UpdateStatus(ctx, id, "completed", rendered, extras); err != nil {
		return err
	}
	if s.auditSvc != nil {
		_ = s.auditSvc.Emit(ctx, &dbm.AuditEvent{
			OccurredAt:   s.now().UTC(),
			Source:       "plugin_debug",
			Operation:    "DIAGNOSTIC_REPORT_COMPLETED",
			ResourceType: "plugin_debug_report",
			ResourceID:   id.String(),
			Outcome:      "SUCCESS",
			Severity:     "INFO",
			Meta:         marshalJSON(rendered),
		})
	}
	return nil
}

// ExportLogs returns the bundle pointer for a report.
func (s *Service) ExportLogs(ctx context.Context, id uuid.UUID) (*ExportResponse, error) {
	if s == nil || s.repo == nil {
		return nil, errors.New("diagnostics repository unavailable")
	}
	report, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	url := report.LogBundleURI
	if url == "" {
		url = s.fallbackLogURL(id)
	}
	return &ExportResponse{
		ReportID: report.UUID,
		URL:      url,
	}, nil
}

// Get fetches a report by identifier.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (*model.DiagnosticReport, error) {
	if s == nil || s.repo == nil {
		return nil, errors.New("diagnostics repository unavailable")
	}
	return s.repo.Get(ctx, id)
}

func (s *Service) fallbackLogURL(id uuid.UUID) string {
	if s == nil || s.fallbackLogBase == "" || id == uuid.Nil {
		return ""
	}
	base := trimTrailingSlash(s.fallbackLogBase)
	return base + "/" + id.String()
}

func marshalJSON(v map[string]any) datatypes.JSON {
	if len(v) == 0 {
		return datatypes.JSON("{}")
	}
	data, err := json.Marshal(v)
	if err != nil {
		return datatypes.JSON("{}")
	}
	return datatypes.JSON(data)
}

func marshalStringMap(v map[string]string) datatypes.JSON {
	if len(v) == 0 {
		return datatypes.JSON("{}")
	}
	data, err := json.Marshal(v)
	if err != nil {
		return datatypes.JSON("{}")
	}
	return datatypes.JSON(data)
}

func cloneMap(src map[string]any) map[string]any {
	if src == nil {
		return map[string]any{}
	}
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func cloneStringMap(src map[string]string) map[string]string {
	if src == nil {
		return map[string]string{}
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func extractSeverity(summary map[string]any) string {
	if val, ok := summary["severity"].(string); ok && strings.TrimSpace(val) != "" {
		return strings.TrimSpace(val)
	}
	return "P3"
}
