package plugin_compat

import (
	"strings"
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
)

// CompatException stores approval workflow for compatibility overrides.
type CompatException struct {
	coremodel.PowerUUIDModel

	TenantID       string     `gorm:"column:tenant_id;type:varchar(128);index" json:"tenant_id"`
	PluginID       string     `gorm:"column:plugin_id;type:varchar(128);index" json:"plugin_id"`
	CurrentVersion string     `gorm:"column:current_version;type:varchar(64)" json:"current_version"`
	TargetVersion  string     `gorm:"column:target_version;type:varchar(64)" json:"target_version"`
	Status         string     `gorm:"column:status;type:varchar(32);index" json:"status"`
	Reason         string     `gorm:"column:reason;type:text" json:"reason"`
	Reviewer       string     `gorm:"column:reviewer;type:varchar(128)" json:"reviewer"`
	DecisionNotes  string     `gorm:"column:decision_notes;type:text" json:"decision_notes"`
	ResolvedAt     *time.Time `gorm:"column:resolved_at" json:"resolved_at"`
}

func (CompatException) TableName() string {
	schema := strings.TrimSpace(coremodel.PowerXSchema)
	if schema == "" {
		return coremodel.TablePluginCompatExceptions
	}
	return schema + "." + coremodel.TablePluginCompatExceptions
}
