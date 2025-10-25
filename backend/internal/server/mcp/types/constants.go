package types

// 工具名称常量
const (
	ToolListBlueprints = "list_blueprints"
	ToolLoadBlueprint  = "load_blueprint"
	ToolPlanFlow       = "plan_flow"
	ToolRenderPlan     = "render_plan"
)

// 参数名称常量
const (
	ParamFlowID    = "flow_id"
	ParamFlow      = "flow"
	ParamPlan      = "plan"
	ParamPath      = "path"
	ParamBlueprint = "blueprint"
)

// 字段名称常量
const (
	FieldID        = "id"
	FieldFlowData  = "flow_data"
	FieldStatus    = "status"
	FieldTasks     = "tasks"
	FieldCreatedAt = "created_at"
	FieldType      = "type"
	FieldText      = "text"
	FieldPlanned   = "planned"
)

const (
	HandlerTypeNative = "native"
	HandlerTypeScript = "script"
	HandlerTypeAPI    = "api"
)

// 错误码常量
const (
	ErrorCodeInvalidArgument = "INVALID_ARGUMENT"
	ErrorCodeNotFound        = "NOT_FOUND"
	ErrorCodeUnauthorized    = "UNAUTHORIZED"
	ErrorCodeForbidden       = "FORBIDDEN"
	ErrorCodeInternal        = "INTERNAL"
)

// HTTP状态码映射
var ErrorCodeToHTTPStatus = map[string]int{
	ErrorCodeInvalidArgument: 400,
	ErrorCodeNotFound:        404,
	ErrorCodeUnauthorized:    401,
	ErrorCodeForbidden:       403,
	ErrorCodeInternal:        500,
}
