package plugin_compat

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	compatmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/plugin_compat"
	compatrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/plugin_compat"
	"github.com/google/uuid"
)

// Service encapsulates compatibility checks and exception workflow.
type Service struct {
	repo *compatrepo.ExceptionRepository
	now  func() time.Time
}

// CheckRequest contains version comparison data.
type CheckRequest struct {
	HostVersion   string `json:"hostVersion"`
	PluginVersion string `json:"pluginVersion"`
	TenantUUID    string `json:"tenant_uuid"`
	PluginID      string `json:"pluginId"`
}

// CheckResponse describes compatibility result.
type CheckResponse struct {
	Compatible bool   `json:"compatible"`
	Reason     string `json:"reason,omitempty"`
	Suggested  string `json:"suggestedVersion,omitempty"`
}

// ExceptionRequest captures exception creation payload.
type ExceptionRequest struct {
	TenantUUID     string `json:"tenant_uuid"`
	PluginID       string `json:"pluginId"`
	CurrentVersion string `json:"currentVersion"`
	TargetVersion  string `json:"targetVersion"`
	Reason         string `json:"reason"`
}

// ApproveRequest handles updating exception status.
type ApproveRequest struct {
	ID            uuid.UUID `json:"id"`
	Status        string    `json:"status"`
	Reviewer      string    `json:"reviewer"`
	DecisionNotes string    `json:"decisionNotes"`
}

// NewService constructs the compat service.
func NewService(repo *compatrepo.ExceptionRepository, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{repo: repo, now: now}
}

// Check determines whether plugin version is compatible with host manifest.
func (s *Service) Check(ctx context.Context, req CheckRequest) (*CheckResponse, error) {
	if strings.TrimSpace(req.PluginVersion) == "" {
		return nil, errors.New("pluginVersion is required")
	}
	if strings.TrimSpace(req.HostVersion) == "" {
		return nil, errors.New("hostVersion is required")
	}
	pluginVer := parseVersion(req.PluginVersion)
	hostVer := parseVersion(req.HostVersion)
	result := &CheckResponse{Compatible: true}
	if pluginVer.Major > hostVer.Major {
		result.Compatible = false
		result.Reason = fmt.Sprintf("plugin major version %d exceeds host %d", pluginVer.Major, hostVer.Major)
		result.Suggested = fmt.Sprintf("%d.%d.%d", hostVer.Major, pluginVer.Minor, pluginVer.Patch)
	} else if pluginVer.Major == hostVer.Major && pluginVer.Minor > hostVer.Minor+1 {
		result.Compatible = false
		result.Reason = "plugin minor version too far ahead"
	}
	return result, nil
}

// CreateException records a compatibility exception request.
func (s *Service) CreateException(ctx context.Context, req ExceptionRequest) (*compatmodel.CompatException, error) {
	if s == nil || s.repo == nil {
		return nil, errors.New("compat service unavailable")
	}
	if strings.TrimSpace(req.PluginID) == "" {
		return nil, errors.New("pluginId is required")
	}
	entity := &compatmodel.CompatException{
		TenantUUID:     strings.TrimSpace(req.TenantUUID),
		PluginID:       strings.TrimSpace(req.PluginID),
		CurrentVersion: strings.TrimSpace(req.CurrentVersion),
		TargetVersion:  strings.TrimSpace(req.TargetVersion),
		Reason:         strings.TrimSpace(req.Reason),
		Status:         "pending",
	}
	return s.repo.Create(ctx, entity)
}

// Approve updates exception status.
func (s *Service) Approve(ctx context.Context, req ApproveRequest) (*compatmodel.CompatException, error) {
	if s == nil || s.repo == nil {
		return nil, errors.New("compat service unavailable")
	}
	if req.ID == uuid.Nil {
		return nil, errors.New("id is required")
	}
	status := strings.ToLower(strings.TrimSpace(req.Status))
	if status == "" {
		status = "approved"
	}
	fields := map[string]any{
		"status":         status,
		"reviewer":       strings.TrimSpace(req.Reviewer),
		"decision_notes": strings.TrimSpace(req.DecisionNotes),
		"resolved_at":    s.now().UTC(),
	}
	if err := s.repo.UpdateStatus(ctx, req.ID, fields); err != nil {
		return nil, err
	}
	return s.repo.Get(ctx, req.ID)
}

type versionInfo struct {
	Major int
	Minor int
	Patch int
}

func parseVersion(raw string) versionInfo {
	parts := strings.Split(strings.TrimSpace(raw), ".")
	info := versionInfo{}
	if len(parts) > 0 {
		info.Major = mustAtoi(parts[0])
	}
	if len(parts) > 1 {
		info.Minor = mustAtoi(parts[1])
	}
	if len(parts) > 2 {
		info.Patch = mustAtoi(parts[2])
	}
	return info
}

func mustAtoi(s string) int {
	val, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	return val
}
