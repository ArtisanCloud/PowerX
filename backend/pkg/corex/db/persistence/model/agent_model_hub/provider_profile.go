package agent_model_hub

import (
	"strings"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"gorm.io/datatypes"
)

// ProviderProfile captures onboarded provider metadata, including rollout state and Vault references.
type ProviderProfile struct {
	coremodel.PowerUUIDModel

	Env      string  `gorm:"column:env;type:varchar(32);not null;index:idx_provider_profiles_scope_name,unique,priority:1" json:"env"`
	TenantID *uint64 `gorm:"column:tenant_id;index:idx_provider_profiles_scope_name,unique,priority:2" json:"tenant_id,omitempty"`

	Name            string                      `gorm:"column:name;type:varchar(128);not null;index:idx_provider_profiles_scope_name,unique,priority:3" json:"name"`
	Capabilities    datatypes.JSONSlice[string] `gorm:"column:capabilities;type:jsonb;default:'[]'::jsonb" json:"capabilities"`
	PrimaryEndpoint string                      `gorm:"column:primary_endpoint;type:text;not null" json:"primary_endpoint"`
	Regions         datatypes.JSONSlice[string] `gorm:"column:regions;type:jsonb;default:'[]'::jsonb" json:"regions"`
	TenantWhitelist datatypes.JSON              `gorm:"column:tenant_whitelist;type:jsonb;default:'[]'::jsonb" json:"tenant_whitelist"`
	SecretRefs      datatypes.JSONMap           `gorm:"column:secret_refs;type:jsonb;default:'{}'::jsonb" json:"secret_refs"`
	SealedSecrets   datatypes.JSONMap           `gorm:"column:sealed_secrets;type:jsonb;default:'{}'::jsonb" json:"-"`
	HealthScore     float64                     `gorm:"column:health_score;type:numeric(5,4);default:0" json:"health_score"`
	RolloutStatus   string                      `gorm:"column:rollout_status;type:varchar(32);not null;default:'draft';index" json:"rollout_status"`
	AuditTrailID    string                      `gorm:"column:audit_trail_id;type:varchar(128)" json:"audit_trail_id"`
}

func (ProviderProfile) TableName() string {
	schema := strings.TrimSpace(coremodel.PowerXSchema)
	if schema == "" {
		return coremodel.TableAgentProviderProfiles
	}
	return schema + "." + coremodel.TableAgentProviderProfiles
}
