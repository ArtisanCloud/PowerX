package flow

// pkg/corex/db/persistence/model/flow/run_log_gorm.go

import (
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"time"

	"gorm.io/datatypes"
)

/*
 * 计划级运行表：agent_plan_runs
 * 一条记录 = 一次计划（plan）的整体运行
 */
type AgentPlanRun struct {
	PlanID    string `gorm:"primaryKey;column:plan_id;type:varchar(128)"              json:"plan_id"`
	RequestID string `gorm:"column:request_id;type:varchar(128);index"                 json:"request_id,omitempty"`
	TraceID   string `gorm:"column:trace_id;type:varchar(128);index"                   json:"trace_id,omitempty"`
	// 多租户 + 参与者（后台用户 & 终端客户）
	TenantID   string `gorm:"column:tenant_id;type:varchar(128);index"                  json:"tenant_id,omitempty"`
	UserID     string `gorm:"column:user_id;type:varchar(128);index"                    json:"user_id,omitempty"`     // 触发者（后台）
	CustomerID string `gorm:"column:customer_id;type:varchar(128);index"                json:"customer_id,omitempty"` // 触发者（终端）

	Status string `gorm:"column:status;type:varchar(24);not null;default:'running'" json:"status"` // running/completed/failed/cancelled

	StartedAt time.Time  `gorm:"column:started_at;index"                                    json:"started_at"`
	EndedAt   *time.Time `gorm:"column:ended_at;index"                                      json:"ended_at,omitempty"`

	// 轻量元数据（尽量只放必要键）
	Meta datatypes.JSON `gorm:"column:meta;type:jsonb"                                                json:"meta,omitempty"`

	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime"                           json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime"                           json:"updated_at"`
}

func (mdl *AgentPlanRun) TableName() string {
	return model.PowerXSchema + "." + model.TableAgentPlanRun
}

func (mdl *AgentPlanRun) GetTableName(needFull bool) string {
	tableName := model.TableAgentPlanRun
	if needFull {
		tableName = mdl.TableName()
	}
	return tableName
}

/*
 * 任务事件表：agent_task_events
 * 多条记录 = 一个 plan 内每个 task 的 start/ok/err 事件
 */
type AgentTaskEvent struct {
	ID uint64 `gorm:"primaryKey;autoIncrement;column:id"                                            json:"id"`

	PlanID  string `gorm:"column:plan_id;type:varchar(128);index:idx_plan_ts,priority:1;index:idx_plan_task_ts,priority:1" json:"plan_id"`
	TaskID  string `gorm:"column:task_id;type:varchar(128);index:idx_plan_task_ts,priority:2"                              json:"task_id"`
	FlowID  string `gorm:"column:flow_id;type:varchar(256);index"                                                          json:"flow_id"`
	Stage   int    `gorm:"column:stage;index"                                                                              json:"stage"`
	AgentID string `gorm:"column:agent_id;type:varchar(128);index"                                                         json:"agent_id"`

	// 参与者维度（用于按人/客户过滤）
	TenantID   string `gorm:"column:tenant_id;type:varchar(128);index:idx_tenant_customer_ts,priority:1"  json:"tenant_id,omitempty"`
	UserID     string `gorm:"column:user_id;type:varchar(128);index"                                       json:"user_id,omitempty"`
	CustomerID string `gorm:"column:customer_id;type:varchar(128);index:idx_tenant_customer_ts,priority:2" json:"customer_id,omitempty"`

	Kind       string    `gorm:"column:kind;type:varchar(24);index"                                           json:"kind"` // task.start | task.ok | task.err
	Ts         time.Time `gorm:"column:ts;index:idx_plan_ts,priority:2;index:idx_plan_task_ts,priority:3;index:idx_tenant_customer_ts,priority:3" json:"ts"`
	DurationMS int64     `gorm:"column:duration_ms"                                                            json:"duration_ms,omitempty"`

	// 建议写入“瘦身后”的入参/出参/元信息
	Input  datatypes.JSON `gorm:"column:input"                                                                  json:"input,omitempty"`
	Output datatypes.JSON `gorm:"column:output"                                                                 json:"output,omitempty"`
	Error  string         `gorm:"column:error;type:text"                                                        json:"error,omitempty"`
	Meta   datatypes.JSON `gorm:"column:meta;type:jsonb"                                                                   json:"meta,omitempty"`

	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime"                                              json:"created_at"`
}

func (mdl *AgentTaskEvent) TableName() string {
	return model.PowerXSchema + "." + model.TableAgentEvent
}

func (mdl *AgentTaskEvent) GetTableName(needFull bool) string {
	tableName := model.TableAgentEvent
	if needFull {
		tableName = mdl.TableName()
	}
	return tableName
}
