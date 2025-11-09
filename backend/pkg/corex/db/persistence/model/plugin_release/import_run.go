package plugin_release

import (
	"strings"
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"github.com/google/uuid"
	"gorm.io/datatypes"
)

// PluginImportRun captures metadata and risk evaluation for each third-party import submission.
type PluginImportRun struct {
	coremodel.PowerUUIDModel

	TenantID        string         `gorm:"column:tenant_id;type:varchar(128);index" json:"tenant_id"`
	PackageName     string         `gorm:"column:package_name;type:varchar(255);not null" json:"package_name"`
	Vendor          string         `gorm:"column:vendor;type:varchar(255)" json:"vendor"`
	SourceURI       string         `gorm:"column:source_uri;type:text" json:"source_uri"`
	Checksum        string         `gorm:"column:checksum;type:varchar(256)" json:"checksum"`
	SubmittedBy     string         `gorm:"column:submitted_by;type:varchar(128)" json:"submitted_by"`
	Status          string         `gorm:"column:status;type:varchar(32);index" json:"status"`
	RiskLevel       string         `gorm:"column:risk_level;type:varchar(16)" json:"risk_level"`
	Findings        datatypes.JSON `gorm:"column:findings;type:jsonb;default:'{}'" json:"findings"`
	LicenseSummary  datatypes.JSON `gorm:"column:license_summary;type:jsonb;default:'{}'" json:"license_summary"`
	VulnSummary     datatypes.JSON `gorm:"column:vuln_summary;type:jsonb;default:'{}'" json:"vuln_summary"`
	ApprovedBy      string         `gorm:"column:approved_by;type:varchar(128)" json:"approved_by"`
	ApprovalNote    string         `gorm:"column:approval_note;type:text" json:"approval_note"`
	ReportReference uuid.UUID      `gorm:"column:report_reference;type:uuid" json:"report_reference"`
	CompletedAt     *time.Time     `gorm:"column:completed_at" json:"completed_at,omitempty"`
}

func (PluginImportRun) TableName() string {
	schema := strings.TrimSpace(coremodel.PowerXSchema)
	if schema == "" {
		return coremodel.TablePluginImportRuns
	}
	return schema + "." + coremodel.TablePluginImportRuns
}

const (
	PluginImportStatusPending  = "pending"
	PluginImportStatusReview   = "review"
	PluginImportStatusApproved = "approved"
	PluginImportStatusRejected = "rejected"

	PluginImportRiskLow    = "low"
	PluginImportRiskMedium = "medium"
	PluginImportRiskHigh   = "high"
)
