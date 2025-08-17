package schemas

// services/agent/schemas/plan.go
// —— 供 ExecutePlan 使用的“可执行计划”与“任务” —— //

//type ExecutionPlan struct {
//	PlanID string     `json:"plan_id"`
//	Tasks  []PlanTask `json:"tasks"`
//	// 可选：计划级元数据（例如 WireRule 编译信息）
//	Metadata map[string]interface{} `json:"metadata,omitempty"`
//}
//
//type PlanTask struct {
//	TaskID    string                 `json:"task_id"`              // = ResolvedStep.StepID
//	FlowID    string                 `json:"flow_id"`              // = ResolvedStep.Use
//	AgentID   string                 `json:"agent_id,omitempty"`   // 若不填，按 routesByFlow 路由
//	Params    map[string]interface{} `json:"params,omitempty"`     // 纯字面量参数
//	Bindings  []ParamBinding         `json:"bindings,omitempty"`   // 依赖绑定：从上游输出取值填充到入参
//	DependsOn []string               `json:"depends_on,omitempty"` // 执行依赖（TaskID 列表）
//	Stage     int                    `json:"stage"`                // 同一 Stage 并发，跨 Stage 串行
//}
//
//// —— 从依赖输出里“取值填参”的绑定规则 —— //
//type ParamBinding struct {
//	Param     string      `json:"param"`               // 要写入的参数名
//	FromTask  string      `json:"from_task"`           // 依赖的 TaskID（必须在 DependsOn 或前一Stage）
//	Path      string      `json:"path"`                // 在上游输出中的路径（如 "data.id" / "metadata.user.name"）
//	Optional  bool        `json:"optional,omitempty"`  // 缺失是否忽略
//	Default   interface{} `json:"default,omitempty"`   // Optional=true 时可用
//	ReadFrom  string      `json:"read_from,omitempty"` // 默认 data，可填 "data"/"metadata"
//	Transform string      `json:"transform,omitempty"` // 轻量表达式（如 "string", "int", "json"），可选
//}
