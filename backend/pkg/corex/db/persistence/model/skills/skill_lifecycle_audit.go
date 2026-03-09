package skills

import (
	"strings"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
)

// SkillLifecycleAudit stores audit trails for skill governance operations.
type SkillLifecycleAudit struct {
	coremodel.PowerUUIDModel

	AuditID      string `gorm:"column:audit_id;type:varchar(128);not null;uniqueIndex:uk_skill_lifecycle_audit" json:"audit_id"`
	Action       string `gorm:"column:action;type:varchar(64);not null;index:idx_skill_lifecycle_action" json:"action"`
	SkillID      string `gorm:"column:skill_id;type:varchar(128);not null;index:idx_skill_lifecycle_skill" json:"skill_id"`
	Version      string `gorm:"column:version;type:varchar(64);not null" json:"version"`
	Operator     string `gorm:"column:operator;type:varchar(128);not null" json:"operator"`
	TenantScope  string `gorm:"column:tenant_scope;type:varchar(64);not null;default:'global'" json:"tenant_scope"`
	Reason       string `gorm:"column:reason;type:text" json:"reason,omitempty"`
	Result       string `gorm:"column:result;type:varchar(32);not null" json:"result"`
	TraceID      string `gorm:"column:trace_id;type:varchar(128);index:idx_skill_lifecycle_trace" json:"trace_id,omitempty"`
	Source       string `gorm:"column:source;type:varchar(32);not null" json:"source"`
	ErrorSummary string `gorm:"column:error_summary;type:text" json:"error_summary,omitempty"`
}

func (SkillLifecycleAudit) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableSkillsLifecycleAudits
}

func (a *SkillLifecycleAudit) Normalize() {
	a.AuditID = strings.TrimSpace(a.AuditID)
	a.Action = strings.TrimSpace(strings.ToLower(a.Action))
	a.SkillID = strings.TrimSpace(strings.ToLower(a.SkillID))
	a.Version = strings.TrimSpace(a.Version)
	a.Operator = strings.TrimSpace(a.Operator)
	a.TenantScope = strings.TrimSpace(strings.ToLower(a.TenantScope))
	a.Result = strings.TrimSpace(strings.ToLower(a.Result))
	a.TraceID = strings.TrimSpace(a.TraceID)
	a.Source = strings.TrimSpace(strings.ToLower(a.Source))
}
