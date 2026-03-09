package skills

import (
	"strings"

	"gorm.io/datatypes"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
)

// SkillCapabilityBinding maps a published skill version to one capability.
type SkillCapabilityBinding struct {
	coremodel.PowerUUIDModel

	SkillID          string         `gorm:"column:skill_id;type:varchar(128);not null;uniqueIndex:uk_skill_binding_identity" json:"skill_id"`
	Version          string         `gorm:"column:version;type:varchar(64);not null;uniqueIndex:uk_skill_binding_identity" json:"version"`
	CapabilityID     string         `gorm:"column:capability_id;type:varchar(128);not null;uniqueIndex:uk_skill_binding_identity;index:idx_skill_binding_capability" json:"capability_id"`
	ToolGrants       datatypes.JSON `gorm:"column:tool_grants;type:jsonb;not null;default:'[]'" json:"tool_grants,omitempty"`
	VisibilityScope  string         `gorm:"column:visibility_scope;type:varchar(32);not null;default:'tenant'" json:"visibility_scope"`
	BindingStatus    string         `gorm:"column:binding_status;type:varchar(32);not null;default:'active'" json:"binding_status"`
	SourceConstraint string         `gorm:"column:source_constraint;type:varchar(32)" json:"source_constraint,omitempty"`
	CreatedBy        string         `gorm:"column:created_by;type:varchar(128)" json:"created_by,omitempty"`
	UpdatedBy        string         `gorm:"column:updated_by;type:varchar(128)" json:"updated_by,omitempty"`
}

func (SkillCapabilityBinding) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableSkillsCapabilityBindings
}

func (b *SkillCapabilityBinding) Normalize() {
	b.SkillID = strings.TrimSpace(strings.ToLower(b.SkillID))
	b.Version = strings.TrimSpace(b.Version)
	b.CapabilityID = strings.TrimSpace(b.CapabilityID)
	b.VisibilityScope = strings.TrimSpace(strings.ToLower(b.VisibilityScope))
	b.BindingStatus = strings.TrimSpace(strings.ToLower(b.BindingStatus))
}
