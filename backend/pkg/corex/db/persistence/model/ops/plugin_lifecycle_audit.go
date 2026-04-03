package ops

import (
	"strings"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
)

type PluginLifecycleAction string

type PluginLifecycleResult string

const (
	PluginLifecycleActionInstall   PluginLifecycleAction = "install"
	PluginLifecycleActionSwitch    PluginLifecycleAction = "switch"
	PluginLifecycleActionRollback  PluginLifecycleAction = "rollback"
	PluginLifecycleActionUninstall PluginLifecycleAction = "uninstall"

	PluginLifecycleResultSuccess PluginLifecycleResult = "success"
	PluginLifecycleResultFailed  PluginLifecycleResult = "failed"
)

// PluginLifecycleAudit 记录插件生命周期关键动作审计。
type PluginLifecycleAudit struct {
	coremodel.PowerUUIDModel

	PluginID    string                `gorm:"column:plugin_id;type:varchar(128);not null;index:idx_ops_plugin_audit_plugin" json:"plugin_id"`
	FromVersion string                `gorm:"column:from_version;type:varchar(128)" json:"from_version,omitempty"`
	ToVersion   string                `gorm:"column:to_version;type:varchar(128)" json:"to_version,omitempty"`
	Action      PluginLifecycleAction `gorm:"column:action;type:varchar(32);not null;index:idx_ops_plugin_audit_action" json:"action"`
	Result      PluginLifecycleResult `gorm:"column:result;type:varchar(32);not null;index:idx_ops_plugin_audit_result" json:"result"`
	GateResult  string                `gorm:"column:gate_result;type:varchar(64)" json:"gate_result,omitempty"`
	GateReason  string                `gorm:"column:gate_reason;type:text" json:"gate_reason,omitempty"`
	Operator    string                `gorm:"column:operator;type:varchar(128);not null" json:"operator"`
	TraceID     string                `gorm:"column:trace_id;type:varchar(128);index:idx_ops_plugin_audit_trace" json:"trace_id,omitempty"`
	Detail      string                `gorm:"column:detail;type:text" json:"detail,omitempty"`
}

func (PluginLifecycleAudit) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableOpsPluginLifecycleAudits
}

func (m *PluginLifecycleAudit) Normalize() {
	m.PluginID = strings.TrimSpace(strings.ToLower(m.PluginID))
	m.FromVersion = strings.TrimSpace(m.FromVersion)
	m.ToVersion = strings.TrimSpace(m.ToVersion)
	m.Action = PluginLifecycleAction(strings.TrimSpace(strings.ToLower(string(m.Action))))
	m.Result = PluginLifecycleResult(strings.TrimSpace(strings.ToLower(string(m.Result))))
	m.GateResult = strings.TrimSpace(strings.ToLower(m.GateResult))
	m.GateReason = strings.TrimSpace(m.GateReason)
	m.Operator = strings.TrimSpace(m.Operator)
	m.TraceID = strings.TrimSpace(m.TraceID)
}
