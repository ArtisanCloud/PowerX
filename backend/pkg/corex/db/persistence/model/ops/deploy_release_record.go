package ops

import (
	"strings"
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
)

type DeployAction string

type DeployStatus string

const (
	DeployActionRelease  DeployAction = "release"
	DeployActionRollback DeployAction = "rollback"

	DeployStatusPending DeployStatus = "pending"
	DeployStatusRunning DeployStatus = "running"
	DeployStatusSuccess DeployStatus = "success"
	DeployStatusFailed  DeployStatus = "failed"
)

// DeployReleaseRecord 记录部署发布/回滚动作。
type DeployReleaseRecord struct {
	coremodel.PowerUUIDModel

	Environment     string       `gorm:"column:environment;type:varchar(64);not null;index:idx_ops_deploy_env_status" json:"environment"`
	BackendVersion  string       `gorm:"column:backend_version;type:varchar(128);not null" json:"backend_version"`
	WebAdminVersion string       `gorm:"column:web_admin_version;type:varchar(128);not null" json:"web_admin_version"`
	Action          DeployAction `gorm:"column:action;type:varchar(32);not null;index:idx_ops_deploy_action" json:"action"`
	Status          DeployStatus `gorm:"column:status;type:varchar(32);not null;index:idx_ops_deploy_env_status" json:"status"`
	Operator        string       `gorm:"column:operator;type:varchar(128);not null" json:"operator"`
	TraceID         string       `gorm:"column:trace_id;type:varchar(128);index:idx_ops_deploy_trace" json:"trace_id,omitempty"`
	StartedAt       *time.Time   `gorm:"column:started_at" json:"started_at,omitempty"`
	EndedAt         *time.Time   `gorm:"column:ended_at" json:"ended_at,omitempty"`
	ErrorMessage    string       `gorm:"column:error_message;type:text" json:"error_message,omitempty"`
}

func (DeployReleaseRecord) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableOpsDeployReleaseRecords
}

func (m *DeployReleaseRecord) Normalize() {
	m.Environment = strings.TrimSpace(strings.ToLower(m.Environment))
	m.BackendVersion = strings.TrimSpace(m.BackendVersion)
	m.WebAdminVersion = strings.TrimSpace(m.WebAdminVersion)
	m.Action = DeployAction(strings.TrimSpace(strings.ToLower(string(m.Action))))
	m.Status = DeployStatus(strings.TrimSpace(strings.ToLower(string(m.Status))))
	m.Operator = strings.TrimSpace(m.Operator)
	m.TraceID = strings.TrimSpace(m.TraceID)
}
