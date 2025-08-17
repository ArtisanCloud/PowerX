package validator

import (
	"fmt"
	"github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/ArtisanCloud/PowerX/services/mcp/errors"
	"reflect"
	"strconv"
	"strings"
)

// Validator 参数验证器接口
type Validator interface {
	Validate(data map[string]any, schema *schemas.JSONSchema) error
}

// JSONSchemaValidator JSON Schema 验证器
type JSONSchemaValidator struct{}

// NewJSONSchemaValidator 创建JSON Schema验证器
func NewJSONSchemaValidator() *JSONSchemaValidator {
	return &JSONSchemaValidator{}
}

// Validate 验证数据是否符合JSON Schema
func (v *JSONSchemaValidator) Validate(data map[string]any, schema *schemas.JSONSchema) error {
	validation := errors.NewValidationError()
	v.validateValue(data, schema, "", validation)
	return validation.ToError()
}

// validateValue 验证单个值
func (v *JSONSchemaValidator) validateValue(value any, schema *schemas.JSONSchema, path string, validation *errors.ValidationError) {
	if schema == nil {
		return
	}

	// 检查类型
	if !v.checkType(value, schema.Type) {
		validation.AddInvalid(path, fmt.Sprintf("expected type %s, got %T", schema.Type, value))
		return
	}

	// 根据类型进行具体验证
	switch schema.Type {
	case "object":
		v.validateObject(value, schema, path, validation)
	case "array":
		v.validateArray(value, schema, path, validation)
	case "string":
		v.validateString(value, schema, path, validation)
	case "number", "integer":
		v.validateNumber(value, schema, path, validation)
	case "boolean":
		v.validateBoolean(value, schema, path, validation)
	}

	// 验证枚举值
	if len(schema.Enum) > 0 {
		v.validateEnum(value, schema.Enum, path, validation)
	}
}

// checkType 检查值类型
func (v *JSONSchemaValidator) checkType(value any, expectedType string) bool {
	if value == nil {
		return expectedType == "null"
	}

	switch expectedType {
	case "object":
		_, ok := value.(map[string]any)
		return ok
	case "array":
		_, ok := value.([]any)
		return ok
	case "string":
		_, ok := value.(string)
		return ok
	case "number":
		_, ok1 := value.(float64)
		_, ok2 := value.(int)
		_, ok3 := value.(int64)
		return ok1 || ok2 || ok3
	case "integer":
		_, ok1 := value.(int)
		_, ok2 := value.(int64)
		if f, ok3 := value.(float64); ok3 {
			return f == float64(int64(f)) // 检查是否为整数
		}
		return ok1 || ok2
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "null":
		return value == nil
	default:
		return true // 未知类型，允许通过
	}
}

// validateObject 验证对象
func (v *JSONSchemaValidator) validateObject(value any, schema *schemas.JSONSchema, path string, validation *errors.ValidationError) {
	obj, ok := value.(map[string]any)
	if !ok {
		return
	}

	// 验证必需字段
	for _, required := range schema.Required {
		if _, exists := obj[required]; !exists {
			fieldPath := v.buildPath(path, required)
			validation.AddRequired(fieldPath)
		}
	}

	// 验证属性
	if schema.Properties != nil {
		for fieldName, fieldValue := range obj {
			fieldPath := v.buildPath(path, fieldName)
			if fieldSchema, exists := schema.Properties[fieldName]; exists {
				v.validateValue(fieldValue, fieldSchema, fieldPath, validation)
			}
			// 注意：这里没有检查额外属性，可以根据需要添加
		}
	}
}

// validateArray 验证数组
func (v *JSONSchemaValidator) validateArray(value any, schema *schemas.JSONSchema, path string, validation *errors.ValidationError) {
	arr, ok := value.([]any)
	if !ok {
		return
	}

	// 验证数组项
	if schema.Items != nil {
		for i, item := range arr {
			itemPath := fmt.Sprintf("%s[%d]", path, i)
			v.validateValue(item, schema.Items, itemPath, validation)
		}
	}
}

// validateString 验证字符串
func (v *JSONSchemaValidator) validateString(value any, schema *schemas.JSONSchema, path string, validation *errors.ValidationError) {
	str, ok := value.(string)
	if !ok {
		return
	}

	// 这里可以添加更多字符串验证规则，如长度、格式等
	_ = str // 避免未使用变量警告
}

// validateNumber 验证数字
func (v *JSONSchemaValidator) validateNumber(value any, schema *schemas.JSONSchema, path string, validation *errors.ValidationError) {
	// 这里可以添加数字验证规则，如范围、精度等
	_ = value // 避免未使用变量警告
}

// validateBoolean 验证布尔值
func (v *JSONSchemaValidator) validateBoolean(value any, schema *schemas.JSONSchema, path string, validation *errors.ValidationError) {
	// 布尔值验证通常只需要类型检查
	_ = value // 避免未使用变量警告
}

// validateEnum 验证枚举值
func (v *JSONSchemaValidator) validateEnum(value any, enumValues []any, path string, validation *errors.ValidationError) {
	for _, enumValue := range enumValues {
		if reflect.DeepEqual(value, enumValue) {
			return
		}
	}

	// 构建枚举值字符串
	enumStrs := make([]string, len(enumValues))
	for i, ev := range enumValues {
		enumStrs[i] = fmt.Sprintf("%v", ev)
	}

	validation.AddInvalid(path, fmt.Sprintf("value must be one of: %s", strings.Join(enumStrs, ", ")))
}

// buildPath 构建字段路径
func (v *JSONSchemaValidator) buildPath(basePath, field string) string {
	if basePath == "" {
		return field
	}
	return basePath + "." + field
}

// ToolInputValidator 工具输入验证器
type ToolInputValidator struct {
	validator Validator
}

// NewToolInputValidator 创建工具输入验证器
func NewToolInputValidator() *ToolInputValidator {
	return &ToolInputValidator{
		validator: NewJSONSchemaValidator(),
	}
}

// ValidateToolInput 验证工具输入
func (v *ToolInputValidator) ValidateToolInput(input map[string]any, toolSpec *schemas.ToolSpec) error {
	if toolSpec.InputSchema == nil {
		return nil // 没有输入模式，跳过验证
	}

	return v.validator.Validate(input, toolSpec.InputSchema)
}

// ValidateToolOutput 验证工具输出
func (v *ToolInputValidator) ValidateToolOutput(output map[string]any, toolSpec *schemas.ToolSpec) error {
	if toolSpec.OutputSchema == nil {
		return nil // 没有输出模式，跳过验证
	}

	return v.validator.Validate(output, toolSpec.OutputSchema)
}

// ConvertAndValidate 转换并验证参数
func (v *ToolInputValidator) ConvertAndValidate(params map[string]any, schema *schemas.JSONSchema) (map[string]any, error) {
	if schema == nil {
		return params, nil
	}

	// 首先尝试类型转换
	converted := v.convertTypes(params, schema)

	// 然后验证
	err := v.validator.Validate(converted, schema)
	if err != nil {
		return nil, err
	}

	return converted, nil
}

// convertTypes 类型转换
func (v *ToolInputValidator) convertTypes(params map[string]any, schema *schemas.JSONSchema) map[string]any {
	if schema.Type != "object" || schema.Properties == nil {
		return params
	}

	result := make(map[string]any)
	for key, value := range params {
		if propSchema, exists := schema.Properties[key]; exists {
			result[key] = v.convertValue(value, propSchema)
		} else {
			result[key] = value
		}
	}

	return result
}

// convertValue 转换单个值
func (v *ToolInputValidator) convertValue(value any, schema *schemas.JSONSchema) any {
	if value == nil {
		return value
	}

	switch schema.Type {
	case "string":
		return v.toString(value)
	case "integer":
		return v.toInteger(value)
	case "number":
		return v.toNumber(value)
	case "boolean":
		return v.toBoolean(value)
	default:
		return value
	}
}

// toString 转换为字符串
func (v *ToolInputValidator) toString(value any) any {
	if str, ok := value.(string); ok {
		return str
	}
	return fmt.Sprintf("%v", value)
}

// toInteger 转换为整数
func (v *ToolInputValidator) toInteger(value any) any {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return v
	case float64:
		return int64(v)
	case string:
		if i, err := strconv.ParseInt(v, 10, 64); err == nil {
			return i
		}
	}
	return value
}

// toNumber 转换为数字
func (v *ToolInputValidator) toNumber(value any) any {
	switch v := value.(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case string:
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return value
}

// toBoolean 转换为布尔值
func (v *ToolInputValidator) toBoolean(value any) any {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return value
}
