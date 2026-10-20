package evidence

import (
	"context"
	"encoding/json"
	"reflect"
)

const StructuredToolInputSchema = "powerx.agent.structured-tool-input/v1"

type StructuredToolInput struct {
	Schema string            `json:"schema"`
	Values map[string]string `json:"values"`
}
type structuredToolInputKey struct{}
type frozenToolInput struct {
	declaration StructuredToolInput
	principal   toolPrincipal
}

// WithStructuredToolInput 只能在请求入口或已校验的持久化输入恢复处调用。
// 数值以十进制字符串传递，避免 HTTP/WS/存储通道先转 float64 丢失原词面值。
func WithStructuredToolInput(ctx context.Context, value any) (context.Context, error) {
	if value == nil {
		return ctx, nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return ctx, toolError("evidence.source_invalid", "source_resolution", err)
	}
	var declared StructuredToolInput
	if DecodeToolJSON(raw, &declared) != nil || declared.Schema != StructuredToolInputSchema || len(declared.Values) == 0 || len(declared.Values) > 32 {
		return ctx, toolError("evidence.source_invalid", "source_resolution", nil)
	}
	for key, literal := range declared.Values {
		if !keyPattern.MatchString(key) || len(literal) > 80 || !toolFiniteDecimal.MatchString(literal) {
			return ctx, toolError("evidence.source_invalid", "source_resolution", nil)
		}
	}
	principal := toolContextPrincipal(ctx)
	principal.agent = 0
	return context.WithValue(ctx, structuredToolInputKey{}, frozenToolInput{declaration: declared, principal: principal}), nil
}

// StructuredToolInputDeclaration 只导出原始声明用于受控持久化，不能序列化私有权威。
func StructuredToolInputDeclaration(ctx context.Context) *StructuredToolInput {
	input, ok := ctx.Value(structuredToolInputKey{}).(frozenToolInput)
	if !ok {
		return nil
	}
	copy := input.declaration
	copy.Values = map[string]string{}
	for k, v := range input.declaration.Values {
		copy.Values[k] = v
	}
	return &copy
}

func TrustedToolInputValues(ctx context.Context) (map[string]any, error) {
	input, ok := ctx.Value(structuredToolInputKey{}).(frozenToolInput)
	principal := toolContextPrincipal(ctx)
	principal.agent = 0
	if !ok || !reflect.DeepEqual(input.principal, principal) {
		return nil, toolError("evidence.source_invalid", "source_resolution", nil)
	}
	values := map[string]any{}
	for key, value := range input.declaration.Values {
		values[key] = json.Number(value)
	}
	return values, nil
}

// WithDirectToolInput 供明确的结构化用户调用入口使用；不得在 Planner 后面调用。
func WithDirectToolInput(ctx context.Context, payload map[string]any) (context.Context, error) {
	frozen, err := FreezeEvidenceInput(payload)
	if err != nil {
		return ctx, err
	}
	declaration := StructuredToolInput{Schema: StructuredToolInputSchema, Values: map[string]string{}}
	for key, value := range frozen {
		number, ok := value.(json.Number)
		if !ok {
			return ctx, toolError("evidence.source_invalid", "source_resolution", nil)
		}
		declaration.Values[key] = number.String()
	}
	return WithStructuredToolInput(ctx, declaration)
}
