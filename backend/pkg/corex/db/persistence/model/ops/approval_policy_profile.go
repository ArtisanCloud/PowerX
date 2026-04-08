package ops

import (
	"strings"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
)

type ApprovalMode string

const (
	ApprovalModeNone         ApprovalMode = "none"
	ApprovalModeDualApproval ApprovalMode = "dual_approval"
)

// ApprovalPolicyProfile 按环境定义审批策略。
type ApprovalPolicyProfile struct {
	coremodel.PowerUUIDModel

	Environment  string       `gorm:"column:environment;type:varchar(64);not null;uniqueIndex:uk_ops_approval_policy_env" json:"environment"`
	ApprovalMode ApprovalMode `gorm:"column:approval_mode;type:varchar(32);not null;default:'none'" json:"approval_mode"`
	UpdatedBy    string       `gorm:"column:updated_by;type:varchar(128);not null" json:"updated_by"`
}

func (ApprovalPolicyProfile) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableOpsApprovalPolicyProfiles
}

func (m *ApprovalPolicyProfile) Normalize() {
	m.Environment = strings.TrimSpace(strings.ToLower(m.Environment))
	m.ApprovalMode = ApprovalMode(strings.TrimSpace(strings.ToLower(string(m.ApprovalMode))))
	m.UpdatedBy = strings.TrimSpace(m.UpdatedBy)
}
