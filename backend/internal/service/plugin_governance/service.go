package plugin_governance

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	govmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/plugin_governance"
	govrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/plugin_governance"
	pluginrelease "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/plugin_release"
	"gorm.io/datatypes"
)

// Service orchestrates version governance reports.
type Service struct {
	reports    *govrepo.ReportRepository
	candidates *pluginrelease.ReleaseCandidateRepository
	now        func() time.Time
}

// ScanInput defines parameters for a governance scan.
type ScanInput struct {
	TenantUUID     string `json:"-"`
	PluginID       string `json:"pluginId"`
	CurrentVersion string `json:"currentVersion"`
	TargetVersion  string `json:"targetVersion"`
}

// BoardFilter filters board summary data.
type BoardFilter struct {
	TenantUUID string
	Limit      int
}

// BoardSummary aggregates latest reports for board view.
type BoardSummary struct {
	Total  int64                              `json:"total"`
	ByRisk map[string]int64                   `json:"riskCounts"`
	Items  []govmodel.VersionGovernanceReport `json:"items"`
}

// NewService constructs the governance service.
func NewService(reports *govrepo.ReportRepository, candidates *pluginrelease.ReleaseCandidateRepository, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{reports: reports, candidates: candidates, now: now}
}

// Scan generates a governance report based on latest release candidate and inputs.
func (s *Service) Scan(ctx context.Context, input ScanInput) (*govmodel.VersionGovernanceReport, error) {
	if s == nil || s.reports == nil {
		return nil, errors.New("governance service unavailable")
	}
	tenantUUID := strings.TrimSpace(input.TenantUUID)
	if tenantUUID == "" {
		return nil, errors.New("tenant_uuid is required")
	}
	pluginID := strings.TrimSpace(input.PluginID)
	if pluginID == "" {
		return nil, errors.New("pluginId is required")
	}

	var candidateVersion string
	var risk string = "info"
	summary := map[string]any{}
	if s.candidates != nil {
		candidate, err := s.candidates.FindLatestByTenantPlugin(ctx, tenantUUID, pluginID)
		if err != nil {
			return nil, err
		}
		if candidate != nil {
			candidateVersion = candidate.Version
			summary["gate_status"] = candidate.GateStatus
			summary["approval_status"] = candidate.ApprovalStatus
			summary["failure_reason"] = candidate.FailureReason
			summary["labels"] = candidate.Labels
			if strings.EqualFold(candidate.GateStatus, "failed") || candidate.FailureReason != "" {
				risk = "critical"
			} else if !strings.EqualFold(candidate.ApprovalStatus, "approved") {
				risk = "warning"
			} else {
				risk = "pass"
			}
		}
	}

	currentVersion := strings.TrimSpace(input.CurrentVersion)
	if currentVersion == "" {
		currentVersion = candidateVersion
	}
	targetVersion := strings.TrimSpace(input.TargetVersion)
	if targetVersion == "" {
		targetVersion = candidateVersion
	}

	report := &govmodel.VersionGovernanceReport{
		TenantUUID:         tenantUUID,
		PluginID:           pluginID,
		CurrentVersion:     currentVersion,
		RecommendedVersion: targetVersion,
		RiskLevel:          risk,
		Status:             "generated",
		Summary:            mustJSON(summary),
		GeneratedAt:        s.now().UTC(),
	}
	return s.reports.Create(ctx, report)
}

// Board summarizes latest reports per tenant.
func (s *Service) Board(ctx context.Context, filter BoardFilter) (*BoardSummary, error) {
	if s == nil || s.reports == nil {
		return nil, errors.New("governance service unavailable")
	}
	reports, err := s.reports.ListRecent(ctx, filter.TenantUUID, filter.Limit)
	if err != nil {
		return nil, err
	}
	summary := &BoardSummary{
		Total:  int64(len(reports)),
		ByRisk: map[string]int64{},
		Items:  reports,
	}
	for _, report := range reports {
		risk := report.RiskLevel
		if risk == "" {
			risk = "info"
		}
		summary.ByRisk[risk]++
	}
	return summary, nil
}

func mustJSON(v map[string]any) datatypes.JSON {
	if v == nil {
		return datatypes.JSON("{}")
	}
	data, err := json.Marshal(v)
	if err != nil {
		return datatypes.JSON("{}")
	}
	return datatypes.JSON(data)
}
