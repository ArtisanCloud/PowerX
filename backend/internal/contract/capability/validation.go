package capability

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Severity 表示校验问题的严重级别。
type Severity string

const (
	SeverityInfo    Severity = "INFO"
	SeverityWarning Severity = "WARNING"
	SeverityError   Severity = "ERROR"
	SeverityFatal   Severity = "FATAL"
)

// ValidationIssue 描述单条校验结果。
type ValidationIssue struct {
	Code     string                 `json:"code"`
	Message  string                 `json:"message"`
	Severity Severity               `json:"severity"`
	Details  map[string]interface{} `json:"details,omitempty"`
}

// CapabilityContractDraft 封装契约发布/更新时需要校验的信息。
type CapabilityContractDraft struct {
	TenantUUID           string
	CapabilityKey        string
	Version              string
	ProviderID           string
	DisplayName          string
	LifecycleState       string
	SecurityScope        string
	ToolGrantRequired    bool
	ObservabilityConfig  map[string]interface{}
	IOSchemas            []IOSchemaDescriptor
	TransportPreferences []TransportPreference
	TransportProfiles    []TransportProfile
	ErrorTaxonomy        []ErrorTaxonomyEntry
}

// IOSchemaDescriptor 表示输入或输出 schema 定义。
type IOSchemaDescriptor struct {
	Direction       string
	Format          string
	SchemaURI       string
	SchemaHash      string
	ValidationRules map[string]interface{}
}

// TransportPreference 描述能力在不同协议的偏好。
type TransportPreference struct {
	Transport string
	Mode      string
}

// TransportProfile 描述协议具体的 QoS 与超时策略。
type TransportProfile struct {
	Transport        string
	Mode             string
	TimeoutMillis    int
	Streaming        bool
	Retry            map[string]interface{}
	QoS              map[string]interface{}
	EndpointSelector map[string]interface{}
}

// ErrorTaxonomyEntry 提供标准错误映射信息。
type ErrorTaxonomyEntry struct {
	Namespace string
	Category  string
	Code      string
	Severity  string
	Stage     string
}

// ScopeVerifier 用于校验 IAM Scope 是否存在。
type ScopeVerifier interface {
	ScopeExists(ctx context.Context, scope string) (bool, error)
}

// ToolGrantVerifier 用于校验 Tool Grant 是否存在或可申请。
type ToolGrantVerifier interface {
	ToolGrantExists(ctx context.Context, scope string) (bool, error)
}

// ValidatorOptions 允许注入外部校验依赖。
type ValidatorOptions struct {
	ScopeVerifier     ScopeVerifier
	ToolGrantVerifier ToolGrantVerifier
}

// Validator 负责执行契约校验。
type Validator struct {
	scopeVerifier     ScopeVerifier
	toolGrantVerifier ToolGrantVerifier
}

// NewValidator 创建校验器实例。
func NewValidator(opts ValidatorOptions) *Validator {
	return &Validator{
		scopeVerifier:     opts.ScopeVerifier,
		toolGrantVerifier: opts.ToolGrantVerifier,
	}
}

var (
	keyPattern         = regexp.MustCompile(`^[a-z0-9]+(\.[a-z0-9]+)*$`)
	semverPattern      = regexp.MustCompile(`^\d+\.\d+\.\d+$`)
	lifecycleSet       = map[string]struct{}{"draft": {}, "published": {}, "deprecated": {}}
	ioDirectionSet     = map[string]struct{}{"input": {}, "output": {}}
	ioFormatSet        = map[string]struct{}{"json_schema": {}, "protobuf": {}, "avro": {}}
	transportSet       = map[string]struct{}{"http": {}, "grpc": {}, "mcp": {}, "agent": {}}
	modeSet            = map[string]struct{}{"prefer": {}, "only": {}, "fallback": {}}
	errorSeverity      = map[string]struct{}{"INFO": {}, "WARNING": {}, "ERROR": {}, "FATAL": {}}
	errorStage         = map[string]struct{}{"validate": {}, "invoke": {}, "stream": {}, "observe": {}}
	requiredTransports = []string{"http", "grpc"}
)

// ValidateContractDraft 运行所有契约校验规则并返回问题列表。
func (v *Validator) ValidateContractDraft(ctx context.Context, draft *CapabilityContractDraft) ([]ValidationIssue, error) {
	if draft == nil {
		return []ValidationIssue{{
			Code:     "contract.nil",
			Message:  "契约内容不能为空",
			Severity: SeverityFatal,
		}}, nil
	}

	var issues []ValidationIssue

	// 基础字段校验
	if draft.CapabilityKey == "" {
		issues = append(issues, newError("contract.capability_key.empty", "能力标识 capability_key 不能为空"))
	} else if !keyPattern.MatchString(draft.CapabilityKey) {
		issues = append(issues, newError("contract.capability_key.format", "能力标识必须使用小写点分格式，例如 crm.lead.create"))
	}

	if draft.Version == "" {
		issues = append(issues, newError("contract.version.empty", "版本号不能为空"))
	} else if !semverPattern.MatchString(draft.Version) {
		issues = append(issues, newError("contract.version.format", "版本号必须符合 SemVer 格式，例如 1.2.3"))
	}

	if draft.DisplayName == "" {
		issues = append(issues, newError("contract.display_name.empty", "展示名称 display_name 不能为空"))
	}

	if draft.ProviderID == "" {
		issues = append(issues, newWarning("contract.provider.empty", "未填写 provider_id，可能影响调用方识别提供者"))
	}

	if draft.LifecycleState != "" {
		if _, ok := lifecycleSet[strings.ToLower(draft.LifecycleState)]; !ok {
			issues = append(issues, newError("contract.lifecycle.invalid", "生命周期状态无效，必须是 draft/published/deprecated"))
		}
	}

	if draft.SecurityScope == "" {
		issues = append(issues, newError("contract.scope.empty", "安全 Scope 不能为空"))
	} else if v.scopeVerifier != nil {
		ok, err := v.scopeVerifier.ScopeExists(ctx, draft.SecurityScope)
		if err != nil {
			issues = append(issues, newWarning("contract.scope.verify_failed", fmt.Sprintf("Scope 校验失败: %v", err)))
		} else if !ok {
			issues = append(issues, newError("contract.scope.missing", fmt.Sprintf("指定的 Scope %s 未在 IAM 中注册", draft.SecurityScope)))
		}
	}

	if draft.ToolGrantRequired && v.toolGrantVerifier != nil {
		ok, err := v.toolGrantVerifier.ToolGrantExists(ctx, draft.SecurityScope)
		if err != nil {
			issues = append(issues, newWarning("contract.tool_grant.verify_failed", fmt.Sprintf("Tool Grant 校验失败: %v", err)))
		} else if !ok {
			issues = append(issues, newError("contract.tool_grant.missing", "标记需要 Tool Grant，但未找到对应授权"))
		}
	}

	issues = append(issues, v.validateIOSchemas(draft.IOSchemas)...)
	issues = append(issues, v.validateTransportPrefs(draft.TransportPreferences)...)
	issues = append(issues, v.validateTransportProfiles(draft.TransportProfiles)...)
	issues = append(issues, v.validateErrorTaxonomy(draft.ErrorTaxonomy)...)

	sortIssues(issues)
	return issues, nil
}

func (v *Validator) validateIOSchemas(items []IOSchemaDescriptor) []ValidationIssue {
	if len(items) == 0 {
		return []ValidationIssue{newError("contract.io_schema.empty", "必须至少提供输入与输出的 Schema 描述")}
	}

	var issues []ValidationIssue
	seenDirection := map[string]struct{}{}
	for idx, schema := range items {
		dir := strings.ToLower(schema.Direction)
		if _, ok := ioDirectionSet[dir]; !ok {
			issues = append(issues, newError("contract.io_schema.direction", fmt.Sprintf("第 %d 个 schema 的 direction 无效，仅支持 input/output", idx)))
		} else if _, dup := seenDirection[dir]; dup {
			issues = append(issues, newError("contract.io_schema.duplicate_direction", fmt.Sprintf("schema direction %s 重复，请合并或调整", dir)))
		} else {
			seenDirection[dir] = struct{}{}
		}

		if _, ok := ioFormatSet[strings.ToLower(schema.Format)]; !ok {
			issues = append(issues, newError("contract.io_schema.format", fmt.Sprintf("第 %d 个 schema 的 format 无效，仅支持 json_schema/protobuf/avro", idx)))
		}

		if schema.SchemaURI == "" && schema.SchemaHash == "" {
			issues = append(issues, newWarning("contract.io_schema.reference", fmt.Sprintf("第 %d 个 schema 未提供 schema_uri 或 schema_hash，建议补充以便校验", idx)))
		}
	}

	if _, ok := seenDirection["input"]; !ok {
		issues = append(issues, newError("contract.io_schema.missing_input", "缺少输入 (direction=input) 的 schema 描述"))
	}
	if _, ok := seenDirection["output"]; !ok {
		issues = append(issues, newError("contract.io_schema.missing_output", "缺少输出 (direction=output) 的 schema 描述"))
	}
	return issues
}

func (v *Validator) validateTransportPrefs(items []TransportPreference) []ValidationIssue {
	if len(items) == 0 {
		return []ValidationIssue{newWarning("contract.transport_pref.empty", "未填写传输偏好，将使用默认 prefer 顺序")}
	}
	var issues []ValidationIssue
	cache := map[string]string{}
	for _, pref := range items {
		transport := strings.ToLower(pref.Transport)
		if _, ok := transportSet[transport]; !ok {
			issues = append(issues, newError("contract.transport_pref.transport", fmt.Sprintf("传输协议 %s 不受支持", pref.Transport)))
			continue
		}
		mode := strings.ToLower(pref.Mode)
		if _, ok := modeSet[mode]; !ok {
			issues = append(issues, newError("contract.transport_pref.mode", fmt.Sprintf("传输模式 %s 无效，仅允许 prefer/only/fallback", pref.Mode)))
			continue
		}
		if existing, dup := cache[transport]; dup {
			issues = append(issues, newError("contract.transport_pref.duplicate", fmt.Sprintf("协议 %s 已配置为 %s，不可重复声明", transport, existing)))
		}
		cache[transport] = mode
	}
	return issues
}

func (v *Validator) validateTransportProfiles(items []TransportProfile) []ValidationIssue {
	if len(items) == 0 {
		return []ValidationIssue{newError("contract.transport_profile.empty", "必须配置至少一个传输通道的运行参数")}
	}
	var issues []ValidationIssue
	seen := map[string]struct{}{}
	for idx, profile := range items {
		transport := strings.ToLower(profile.Transport)
		if _, ok := transportSet[transport]; !ok {
			issues = append(issues, newError("contract.transport_profile.transport", fmt.Sprintf("第 %d 项传输配置的协议 %s 不受支持", idx, profile.Transport)))
		}
		mode := strings.ToLower(profile.Mode)
		if _, ok := modeSet[mode]; !ok {
			issues = append(issues, newError("contract.transport_profile.mode", fmt.Sprintf("第 %d 项传输配置的 mode %s 无效", idx, profile.Mode)))
		}
		if profile.TimeoutMillis <= 0 {
			issues = append(issues, newError("contract.transport_profile.timeout", fmt.Sprintf("第 %d 项传输配置的 timeout_ms 必须大于 0", idx)))
		}
		if _, dup := seen[transport]; dup {
			issues = append(issues, newError("contract.transport_profile.duplicate", fmt.Sprintf("传输配置中协议 %s 重复声明", transport)))
		}
		seen[transport] = struct{}{}
	}
	for _, required := range requiredTransports {
		if _, ok := seen[required]; !ok {
			issues = append(issues, newError("contract.transport_profile.required", fmt.Sprintf("必须提供 %s 协议的传输配置", required)))
		}
	}
	return issues
}

func (v *Validator) validateErrorTaxonomy(items []ErrorTaxonomyEntry) []ValidationIssue {
	if len(items) == 0 {
		return []ValidationIssue{newError("contract.error_taxonomy.empty", "必须声明至少一条错误映射 (ErrorTaxonomy)")}
	}
	var issues []ValidationIssue
	seenCode := map[string]struct{}{}
	for idx, entry := range items {
		if entry.Namespace == "" || entry.Category == "" || entry.Code == "" {
			issues = append(issues, newError("contract.error_taxonomy.required_fields", fmt.Sprintf("第 %d 条错误映射需包含 namespace/category/code", idx)))
		}
		if entry.Severity == "" {
			issues = append(issues, newWarning("contract.error_taxonomy.severity.empty", fmt.Sprintf("第 %d 条错误映射未注明 severity，将默认为 ERROR", idx)))
		} else if _, ok := errorSeverity[strings.ToUpper(entry.Severity)]; !ok {
			issues = append(issues, newError("contract.error_taxonomy.severity.invalid", fmt.Sprintf("第 %d 条错误映射的 severity 无效", idx)))
		}
		if entry.Stage != "" {
			if _, ok := errorStage[strings.ToLower(entry.Stage)]; !ok {
				issues = append(issues, newError("contract.error_taxonomy.stage.invalid", fmt.Sprintf("第 %d 条错误映射的 stage 无效", idx)))
			}
		}
		key := strings.ToLower(entry.Namespace + ":" + entry.Category + ":" + entry.Code)
		if _, dup := seenCode[key]; dup {
			issues = append(issues, newError("contract.error_taxonomy.duplicate", fmt.Sprintf("错误码 %s 在同一命名空间下重复出现", entry.Code)))
		}
		seenCode[key] = struct{}{}
	}
	return issues
}

func sortIssues(issues []ValidationIssue) {
	sort.SliceStable(issues, func(i, j int) bool {
		if issues[i].Severity == issues[j].Severity {
			return issues[i].Code < issues[j].Code
		}
		return severityOrder(issues[i].Severity) > severityOrder(issues[j].Severity)
	})
}

func severityOrder(s Severity) int {
	switch s {
	case SeverityFatal:
		return 4
	case SeverityError:
		return 3
	case SeverityWarning:
		return 2
	case SeverityInfo:
		return 1
	default:
		return 0
	}
}

func newError(code, msg string) ValidationIssue {
	return ValidationIssue{
		Code:     code,
		Message:  msg,
		Severity: SeverityError,
	}
}

func newWarning(code, msg string) ValidationIssue {
	return ValidationIssue{
		Code:     code,
		Message:  msg,
		Severity: SeverityWarning,
	}
}
