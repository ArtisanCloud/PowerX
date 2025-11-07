package plugin_release

import (
	"strings"
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"gorm.io/datatypes"
)

// CanaryDeploymentRecord stores per-batch telemetry captured during rollout.
type CanaryDeploymentRecord struct {
	coremodel.PowerModel

	ReleasePlanID     uint64         `gorm:"column:release_plan_id;not null;index" json:"release_plan_id"`
	BatchName         string         `gorm:"column:batch_name;type:varchar(64);not null;index:idx_plugin_release_canary_plan_batch,priority:2" json:"batch_name"`
	TenantScope       datatypes.JSON `gorm:"column:tenant_scope;type:jsonb;default:'[]'" json:"tenant_scope,omitempty"`
	MetricSnapshot    datatypes.JSON `gorm:"column:metric_snapshot;type:jsonb;default:'{}'" json:"metric_snapshot,omitempty"`
	ThresholdBreached bool           `gorm:"column:threshold_breached;not null;default:false" json:"threshold_breached"`
	ActionTaken       string         `gorm:"column:action_taken;type:varchar(32);not null;default:'continue'" json:"action_taken"`
	CompletedAt       *time.Time     `gorm:"column:completed_at" json:"completed_at,omitempty"`
}

func (CanaryDeploymentRecord) TableName() string {
	schema := strings.TrimSpace(coremodel.PowerXSchema)
	if schema == "" {
		return coremodel.TablePluginReleaseCanaryRecords
	}
	return schema + "." + coremodel.TablePluginReleaseCanaryRecords
}

// EnsureUniqueConstraint returns fields that must be unique per plan.
func (CanaryDeploymentRecord) EnsureUniqueConstraint() []string {
	return []string{"release_plan_id", "batch_name"}
}

const (
	CanaryActionContinue = "continue"
	CanaryActionHold     = "hold"
	CanaryActionRollback = "rollback"
)
