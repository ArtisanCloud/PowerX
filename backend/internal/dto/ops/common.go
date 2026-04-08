package ops

// OperatorContext 表示运维操作人上下文。
type OperatorContext struct {
	Operator string `json:"operator"`
	TraceID  string `json:"trace_id"`
}
