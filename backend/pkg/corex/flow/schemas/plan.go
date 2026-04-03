package schemas

import "time"

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

// 执行计划（顺序/并行 + 数据引用）
type ExecutionPlan struct {
	PlanID string     `json:"plan_id"`
	Tasks  []PlanTask `json:"tasks"` // 有序列表；并行可用同一 stage
}

type PlanTask struct {
	TaskID      string                 `json:"task_id"`
	FlowID      string                 `json:"flow_id"`
	NodeKind    string                 `json:"node_kind,omitempty"`    // workflow|skill|tooling|llm
	NodeRef     string                 `json:"node_ref,omitempty"`     // workflow_id/skill_id/tool_id/model_key
	SourceScope string                 `json:"source_scope,omitempty"` // system|agent
	AgentID     string                 `json:"agent_id"`
	Params      map[string]interface{} `json:"params,omitempty"`
	// 引用上游输出作为入参： "{{task.lead_create.output.id}}"
	ParamRefs map[string]string `json:"param_refs,omitempty"`
	Stage     int               `json:"stage"` // 相同 stage 可并行
	DependsOn []string          `json:"depends_on,omitempty"`
}
