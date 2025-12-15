package plugin_import

import (
	"time"

	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/plugin_release"
)

// ImportRequest represents the payload describing a third-party package import.
type ImportRequest struct {
	TenantUUID         string            `json:"tenant_uuid"`
	PackageName        string            `json:"packageName"`
	Vendor             string            `json:"vendor"`
	SourceURI          string            `json:"sourceUri"`
	Checksum           string            `json:"checksum"`
	SubmittedBy        string            `json:"submittedBy"`
	HasSPDX            bool              `json:"hasSpdx"`
	HighRiskLicenses   []string          `json:"highRiskLicenses"`
	LicenseInventory   map[string]int    `json:"licenseInventory"`
	VulnerabilityCount int               `json:"vulnerabilityCount"`
	Notes              string            `json:"notes"`
	Metadata           map[string]string `json:"metadata"`
}

// ImportResult summarises repository result.
type ImportResult struct {
	Run         *model.PluginImportRun `json:"run"`
	RiskLevel   string                 `json:"riskLevel"`
	Findings    map[string]any         `json:"findings"`
	NextActions []string               `json:"nextActions"`
}

// ImportRecord is a lightweight DTO used by HTTP handlers.
type ImportRecord struct {
	ID         string                 `json:"id"`
	Status     string                 `json:"status"`
	RiskLevel  string                 `json:"riskLevel"`
	Package    string                 `json:"packageName"`
	Vendor     string                 `json:"vendor"`
	TenantUUID string                 `json:"tenant_uuid"`
	Submitted  time.Time              `json:"submittedAt"`
	Completed  *time.Time             `json:"completedAt,omitempty"`
	Findings   map[string]any         `json:"findings"`
	Notes      string                 `json:"notes,omitempty"`
	ReportRef  string                 `json:"reportRef,omitempty"`
	Submission map[string]interface{} `json:"submission,omitempty"`
}
