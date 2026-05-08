package logger

import "context"

const contextLogFieldsKey = "log_fields"

// WithLogFields 将业务字段挂到 context，供 Logger 自动提取。
// 约束：仅建议传低风险的可序列化标量字段，避免大对象。
func WithLogFields(ctx context.Context, fields map[string]interface{}) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if len(fields) == 0 {
		return ctx
	}
	return context.WithValue(ctx, contextLogFieldsKey, fields)
}
