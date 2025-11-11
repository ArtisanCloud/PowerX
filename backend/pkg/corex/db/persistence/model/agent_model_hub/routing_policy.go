package agent_model_hub

import (
	"strings"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"gorm.io/datatypes"
)

// RoutingPolicy stores weighted routing rules, approvals and safe-mode thresholds per tenant scope.
type RoutingPolicy struct {
	coremodel.PowerUUIDModel

	Env         string `gorm:"column:env;type:varchar(32);not null;index:idx_routing_policies_scope_version,unique,priority:1" json:"env"`
	TenantScope string `gorm:"column:tenant_scope;type:varchar(128);not null;index:idx_routing_policies_scope_version,unique,priority:2" json:"tenant_scope"`
	Version     uint32 `gorm:"column:version;not null;default:1;index:idx_routing_policies_scope_version,unique,priority:3" json:"version"`

	Status string `gorm:"column:status;type:varchar(32);not null;default:'draft';index" json:"status"`

	Rules              datatypes.JSON    `gorm:"column:rules;type:jsonb;default:'[]'::jsonb" json:"rules"`
	FallbackChain      datatypes.JSON    `gorm:"column:fallback_chain;type:jsonb;default:'[]'::jsonb" json:"fallback_chain"`
	ApprovalRecord     datatypes.JSONMap `gorm:"column:approval_record;type:jsonb;default:'{}'::jsonb" json:"approval_record"`
	SafeModeThresholds datatypes.JSONMap `gorm:"column:safe_mode_thresholds;type:jsonb;default:'{}'::jsonb" json:"safe_mode_thresholds"`
}

func (RoutingPolicy) TableName() string {
	schema := strings.TrimSpace(coremodel.PowerXSchema)
	if schema == "" {
		return coremodel.TableAgentRoutingPolicies
	}
	return schema + "." + coremodel.TableAgentRoutingPolicies
}
