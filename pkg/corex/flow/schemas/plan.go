package schemas

import "time"

// 可放到 services/agent/schemas
type WireRule struct {
	// 谁连到谁（可留空：表示“上一个任务”→当前任务）
	FromFlow string `json:"from_flow,omitempty"` // 支持通配符（可后续扩展）
	ToFlow   string `json:"to_flow,omitempty"`
	// 过滤器（可选，比如只针对某 domain/entity）
	Domain string `json:"domain,omitempty"`
	Entity string `json:"entity,omitempty"`

	// 字段映射：输出字段 -> 输入参数
	Map map[string]string `json:"map,omitempty"` // e.g. {"id":"lead_id"}

	// 当输出字段名与输入名一致时是否自动连线
	AutoByName bool `json:"auto_by_name,omitempty"`

	// 优先级（多条命中时按大到小）
	Priority int `json:"priority,omitempty"`
}

// ResolvedPlan 生成的执行计划（Plan）
type ResolvedPlan struct {
	PlanID        string                 `json:"plan_id"`
	FlowID        string                 `json:"flow_id"`
	ResolvedSteps []ResolvedStep         `json:"resolved_steps"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

// ResolvedStep 计划中的单步
type ResolvedStep struct {
	StepID     string                 `json:"step_id"`
	Use        string                 `json:"use"`
	Parameters map[string]interface{} `json:"parameters,omitempty"`
}

// Or, 若你更偏好完整执行任务的指标：
type PlanStatus struct {
	PlanID      string     `json:"plan_id"`
	Status      string     `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	// …
}
