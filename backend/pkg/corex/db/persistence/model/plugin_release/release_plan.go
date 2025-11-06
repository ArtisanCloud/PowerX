package plugin_release

import (
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"gorm.io/datatypes"
)

// ReleasePlan encodes production rollout strategy produced after approvals.
type ReleasePlan struct {
	coremodel.PowerModel

	ReleaseCandidateID  uint64         `gorm:"column:release_candidate_id;not null;index" json:"release_candidate_id"`
	WindowStart         time.Time      `gorm:"column:window_start;type:timestamptz;not null" json:"window_start"`
	WindowEnd           time.Time      `gorm:"column:window_end;type:timestamptz;not null" json:"window_end"`
	CanaryBatches       datatypes.JSON `gorm:"column:canary_batches;type:jsonb;default:'[]'" json:"canary_batches,omitempty"`
	RollbackScripts     datatypes.JSON `gorm:"column:rollback_scripts;type:jsonb;default:'[]'" json:"rollback_scripts,omitempty"`
	DependencyList      datatypes.JSON `gorm:"column:dependency_list;type:jsonb;default:'[]'" json:"dependency_list,omitempty"`
	NotificationTargets datatypes.JSON `gorm:"column:notification_targets;type:jsonb;default:'[]'" json:"notification_targets,omitempty"`
	ApprovalTrail       datatypes.JSON `gorm:"column:approval_trail;type:jsonb;default:'[]'" json:"approval_trail,omitempty"`
	Status              string         `gorm:"column:status;type:varchar(32);not null;default:'draft';index" json:"status"`
	CreatedBy           string         `gorm:"column:created_by;type:varchar(128)" json:"created_by,omitempty"`
	UpdatedBy           string         `gorm:"column:updated_by;type:varchar(128)" json:"updated_by,omitempty"`

	CanaryRecords []CanaryDeploymentRecord `gorm:"foreignKey:ReleasePlanID;references:ID" json:"canary_records,omitempty"`
}

func (ReleasePlan) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TablePluginReleasePlans
}

const (
	ReleasePlanStatusDraft      = "draft"
	ReleasePlanStatusScheduled  = "scheduled"
	ReleasePlanStatusExecuting  = "executing"
	ReleasePlanStatusCompleted  = "completed"
	ReleasePlanStatusRolledBack = "rolled_back"
)
