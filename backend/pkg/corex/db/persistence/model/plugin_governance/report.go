package plugin_governance

import (
	"strings"
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"gorm.io/datatypes"
)

// VersionGovernanceReport captures scan results per tenant/plugin.
type VersionGovernanceReport struct {
	coremodel.PowerUUIDModel

	TenantID           string         `gorm:"column:tenant_id;type:varchar(128);index" json:"tenant_id"`
	PluginID           string         `gorm:"column:plugin_id;type:varchar(128);index" json:"plugin_id"`
	CurrentVersion     string         `gorm:"column:current_version;type:varchar(64)" json:"current_version"`
	RecommendedVersion string         `gorm:"column:recommended_version;type:varchar(64)" json:"recommended_version"`
	RiskLevel          string         `gorm:"column:risk_level;type:varchar(32);index" json:"risk_level"`
	Status             string         `gorm:"column:status;type:varchar(32);index" json:"status"`
	Summary            datatypes.JSON `gorm:"column:summary;type:jsonb" json:"summary"`
	GeneratedAt        time.Time      `gorm:"column:generated_at" json:"generated_at"`
}

func (VersionGovernanceReport) TableName() string {
	schema := strings.TrimSpace(coremodel.PowerXSchema)
	if schema == "" {
		return coremodel.TableVersionGovernanceReports
	}
	return schema + "." + coremodel.TableVersionGovernanceReports
}
