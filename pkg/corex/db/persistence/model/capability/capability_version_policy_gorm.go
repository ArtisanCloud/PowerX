package capability

import (
	"gorm.io/datatypes"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
)

// CapabilityVersionPolicy 管理能力的版本兼容策略。
type CapabilityVersionPolicy struct {
	coremodel.PowerUUIDModel

	TenantID            uint64         `gorm:"column:tenant_id;not null;index:idx_capability_version_policy_tenant_key,priority:1;uniqueIndex:uk_capability_version_policy_tenant_key,priority:1" json:"tenant_id"`
	CapabilityKey       string         `gorm:"column:capability_key;type:varchar(128);not null;index:idx_capability_version_policy_tenant_key,priority:2;uniqueIndex:uk_capability_version_policy_tenant_key,priority:2" json:"capability_key"`
	DefaultStrategy     string         `gorm:"column:default_strategy;type:varchar(32);not null" json:"default_strategy"`
	AllowedVersions     datatypes.JSON `gorm:"column:allowed_versions;type:jsonb;default:'{}'" json:"allowed_versions,omitempty"`
	CompatibilityMatrix datatypes.JSON `gorm:"column:compatibility_matrix;type:jsonb;default:'{}'" json:"compatibility_matrix,omitempty"`
	DeprecationPolicy   datatypes.JSON `gorm:"column:deprecation_policy;type:jsonb;default:'{}'" json:"deprecation_policy,omitempty"`
	AuditConfig         datatypes.JSON `gorm:"column:audit_config;type:jsonb;default:'{}'" json:"audit_config,omitempty"`
	Status              int16          `gorm:"column:status;default:1;index" json:"status"`
}

func (m *CapabilityVersionPolicy) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableCapabilityVersionPolicy
}

func (m *CapabilityVersionPolicy) GetTableName(full bool) string {
	if full {
		return m.TableName()
	}
	return coremodel.TableCapabilityVersionPolicy
}
