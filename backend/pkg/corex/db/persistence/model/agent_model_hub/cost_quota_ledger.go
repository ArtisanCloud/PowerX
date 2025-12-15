package agent_model_hub

import (
	"strings"
	"time"

	"github.com/google/uuid"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"gorm.io/datatypes"
)

// CostQuotaLedger stores tenant/provider budget windows, usage snapshots, and enforcement metadata.
type CostQuotaLedger struct {
	coremodel.PowerUUIDModel

	Env          string `gorm:"column:env;type:varchar(32);not null;index:idx_cost_quota_scope,priority:1" json:"env"`
	TenantUUID   string `gorm:"column:tenant_uuid;type:varchar(128);not null;index:idx_cost_quota_scope,priority:2" json:"tenant_uuid"`
	BudgetPeriod string `gorm:"column:budget_period;type:varchar(32);not null" json:"budget_period"`

	ProviderProfileID *uuid.UUID `gorm:"column:provider_profile_id;type:uuid" json:"provider_profile_id,omitempty"`

	QuotaLimit  float64 `gorm:"column:quota_limit;type:numeric(24,4);not null;default:0" json:"quota_limit"`
	UsageActual float64 `gorm:"column:usage_actual;type:numeric(24,4);not null;default:0" json:"usage_actual"`

	AnomalyState     datatypes.JSONMap `gorm:"column:anomaly_state;type:jsonb;default:'{}'::jsonb" json:"anomaly_state"`
	EnforcementState datatypes.JSONMap `gorm:"column:enforcement_state;type:jsonb;default:'{}'::jsonb" json:"enforcement_state"`
	SealedMetadata   datatypes.JSONMap `gorm:"column:sealed_metadata;type:jsonb;default:'{}'::jsonb" json:"-"`
	DashboardScope   string            `gorm:"column:dashboard_scope;type:varchar(128);not null" json:"dashboard_scope"`

	LastAnomalyAt *time.Time `gorm:"column:last_anomaly_at" json:"last_anomaly_at"`
}

func (CostQuotaLedger) TableName() string {
	schema := strings.TrimSpace(coremodel.PowerXSchema)
	if schema == "" {
		return coremodel.TableAgentCostQuotaLedgers
	}
	return schema + "." + coremodel.TableAgentCostQuotaLedgers
}
