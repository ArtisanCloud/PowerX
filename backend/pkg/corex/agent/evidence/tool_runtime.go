package evidence

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/utils"
	"github.com/google/uuid"
)

//go:embed *.go
var calculatorPackage embed.FS

// CalculatorPackageBytes 是受信任 Core 构建内嵌的实现包，跨 app/database 入口一致。
// 原件包含执行器/typed adapter/DTO/来源校验、严格 JSON 及实际 Go Runtime 版本。
func CalculatorPackageBytes() ([]byte, error) {
	files := map[string][]byte{}
	entries, err := calculatorPackage.ReadDir(".")
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || strings.HasSuffix(name, "_test.go") {
			continue
		}
		raw, err := calculatorPackage.ReadFile(name)
		if err != nil {
			return nil, err
		}
		files[name] = raw
	}
	files["utils/json.go"] = utils.StrictJSONImplementationBytes()
	return json.Marshal(struct {
		Schema    string            `json:"schema"`
		GoVersion string            `json:"go_version"`
		Files     map[string][]byte `json:"files"`
	}{"powerx.calculation.builtin-package/v1", runtime.Version(), files})
}

// BoundTool 只能由受信任 DI 提供；provider 只返回输出，不能创建授权或回执。
// ValidateRequest 必须校验正式 Schema、实际来源绑定及发布参数；不能仅返回 true。
// Recheck 在执行后重查授权及固定版本，不能将目录的自报 digest 当作实现证明。
type ToolDescription struct {
	InputFields  map[string]SourceField
	ResultFields map[string]SourceField
	Labels       map[string]map[string]string
	AgentUUID    string
}

type BoundTool struct {
	Description     ToolDescription
	Binding         ToolInvocationBinding
	Authorization   ToolAuthorization
	ValidateRequest func(context.Context, []byte, []ResolvedDatum) error
	Invoke          func(context.Context, []byte) ([]byte, error)
	ValidateResult  func(context.Context, []byte, []byte) error
	Recheck         func(context.Context) error
	OutputFields    func([]byte, []byte) map[string]SourceField
}
type CapabilityToolPreparer func(context.Context, ToolRunIdentity, ToolDependency) (*BoundTool, error)
type ToolServiceOptions struct {
	Objects    map[string]AuthorizedObjectSource
	Capability CapabilityToolPreparer
}

var toolDigestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
var toolFiniteDecimal = regexp.MustCompile(`^-?(?:0|[1-9][0-9]*)(?:\.[0-9]+)?$`)

const maxToolRunBytes = 64 * 1024 * 1024

// CalculationPlan 来自已发布策略，绑定字段而非模型给出的数值。
type CalculationPlan struct {
	Expression    string
	OperandFields map[string]string
	Precision     int
	Percent       bool
	ResultUnit    string
}

// ToolRunPolicy 由 Runtime 从固定 Revision 编译；不接收模型临时改变工具/公式/字段。
type ToolRunPolicy struct {
	Tools        ToolRequirements
	Fields       map[string]SourceField
	Calculations map[string]CalculationPlan // key 为发布策略的 node ID
}

// ToolService 不缓存授权结果。未明确装配的对象/外部协议始终拒绝。
type ToolService struct {
	objects            map[string]AuthorizedObjectSource
	capability         CapabilityToolPreparer
	implementationOnce sync.Once
	implementation     string
	implementationErr  error
	subjectUUID        string
}

func NewToolService(options ToolServiceOptions) *ToolService {
	objects := make(map[string]AuthorizedObjectSource, len(options.Objects))
	for key, reader := range options.Objects {
		objects[key] = reader
	}
	return &ToolService{objects: objects, capability: options.Capability, subjectUUID: uuid.NewString()}
}

// CalculatorDependency 固定实际内嵌实现包与完整 Schema；不以版本标签或外层入口二进制代替。
func (s *ToolService) CalculatorDependency(key string) (ToolDependency, error) {
	s.implementationOnce.Do(func() {
		bundle, err := CalculatorPackageBytes()
		s.implementationErr = err
		if err == nil {
			s.implementation = toolDigest(bundle)
		}
	})
	if s.implementationErr != nil || !keyPattern.MatchString(key) {
		return ToolDependency{}, toolError("skill.tool_dependency_unavailable", "dependency_check", s.implementationErr)
	}
	schemaDigest := toolDigest(toolContractJSON)
	return ToolDependency{DependencyKey: key, ToolKey: CalculatorKey, Version: TypedCalculatorVersion,
		InputSchema: ToolSchemaRef{ID: CalculationRequestSchema, SHA256: schemaDigest}, OutputSchema: ToolSchemaRef{ID: CalculationResultSchema, SHA256: schemaDigest},
		Permission: ToolPermission{Actor: "runtime_internal", Code: "pure_calculation"}, Binding: ToolBinding{Kind: "builtin", ImplementationSHA256: s.implementation}}, nil
}

func (s *ToolService) CheckCalculator(tool ToolDependency) error {
	expected, err := s.CalculatorDependency(tool.DependencyKey)
	if err != nil {
		return err
	}
	if expected != tool {
		return toolError("skill.tool_dependency_version_or_contract_mismatch", "dependency_check", nil)
	}
	return nil
}

// NewRun 不接受历史 receipts/snapshots；input UUID 表示本次 HTTP/Agent 输入对象，
// 不伪造聊天消息 UUID。raw 快照以精确 JSON 字节存放于当前运行的私有 ledger。
func (s *ToolService) NewRun(ctx context.Context, identity ToolRunIdentity, input map[string]any, policy ToolRunPolicy) (*ToolRun, error) {
	if s == nil || !canonicalUUID(identity.TenantUUID) || !canonicalUUID(identity.RevisionUUID) || !boundedText(identity.RunID, 512) || !boundedText(identity.TraceID, 512) {
		return nil, toolError("evidence.source_invalid", "source_resolution", nil)
	}
	if policy.Tools.Schema != ToolRequirementsSchema || len(policy.Tools.Tools) == 0 || len(policy.Tools.Tools) > 64 {
		return nil, toolError("skill.tool_dependency_invalid", "dependency_check", nil)
	}
	copyPolicy := ToolRunPolicy{Tools: ToolRequirements{Schema: policy.Tools.Schema, Tools: append([]ToolDependency(nil), policy.Tools.Tools...)}, Fields: map[string]SourceField{}, Calculations: map[string]CalculationPlan{}}
	for key, field := range policy.Fields {
		copyPolicy.Fields[key] = field
	}
	for node, plan := range policy.Calculations {
		bindings := map[string]string{}
		for key, field := range plan.OperandFields {
			bindings[key] = field
		}
		plan.OperandFields = bindings
		copyPolicy.Calculations[node] = plan
	}
	seenKeys, seenTools := map[string]bool{}, map[string]bool{}
	for _, tool := range copyPolicy.Tools.Tools {
		if !ValidToolDependency(tool) || seenKeys[tool.DependencyKey] || seenTools[tool.ToolKey] {
			return nil, toolError("skill.tool_dependency_invalid", "dependency_check", nil)
		}
		seenKeys[tool.DependencyKey], seenTools[tool.ToolKey] = true, true
	}
	frozen, err := FreezeEvidenceInput(input)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(frozen)
	if err != nil {
		return nil, err
	}
	run := &ToolRun{identity: identity, service: s, inputUUID: uuid.NewString(), snapshots: map[string]sourceSnapshot{}, datums: map[string]ResolvedDatum{}, receipts: map[string][]byte{}, rechecks: map[string]func(context.Context) error{}, artifacts: map[string][]byte{}, outputFields: map[string]map[string]SourceField{}, policy: copyPolicy, principal: toolContextPrincipal(ctx), bytesStored: len(raw)}
	if err := run.checkContext(ctx); err != nil {
		return nil, err
	}
	run.snapshots[snapshotKey("run_input", "powerx.run_input", run.inputUUID)] = sourceSnapshot{uuid: uuid.NewString(), digest: toolDigest(raw), raw: raw}
	return run, nil
}

type ToolCall struct {
	Tool    ToolDependency
	NodeID  string
	Attempt int
	Inputs  []ResolvedDatum
	Request json.RawMessage
}

// Execute 创建且只创建实际调用的回执。所有前置校验通过后才生成 execution UUID。
// 调用方必须从已发布 Revision 取 Tool、参数及公式，不能把模型临时计划直接传入。
func (r *ToolRun) Execute(ctx context.Context, call ToolCall) (*ToolReceipt, error) {
	if err := r.checkContext(ctx); err != nil {
		return nil, err
	}
	if !boundedText(call.NodeID, 512) || call.Attempt < 1 || len(call.Inputs) > 512 || len(call.Request) > toolJSONLimit {
		return nil, toolError("evidence.calculation_plan_invalid", "input_validation", nil)
	}
	declared := false
	for _, tool := range r.policy.Tools.Tools {
		if tool == call.Tool {
			declared = true
			break
		}
	}
	if !declared {
		return nil, toolError("skill.tool_dependency_version_or_contract_mismatch", "dependency_check", nil)
	}
	// 回调持有的副本不能改写本次已验证输入/请求。
	call.Request = append([]byte(nil), call.Request...)
	call.Inputs = append([]ResolvedDatum{}, call.Inputs...)
	seen := map[string]bool{}
	for _, datum := range call.Inputs {
		if seen[datum.DatumUUID] {
			return nil, toolError("evidence.source_invalid", "source_resolution", nil)
		}
		seen[datum.DatumUUID] = true
		if err := r.VerifyDatum(ctx, datum); err != nil {
			return nil, err
		}
	}
	bound, err := r.prepare(ctx, call.Tool)
	if err != nil {
		if failure, ok := FailureDetails(err); ok {
			return nil, &ToolEvidenceError{ToolFailure: failure, Cause: err}
		}
		return nil, toolError("skill.tool_dependency_unavailable", "dependency_check", err)
	}
	if bound == nil || bound.Invoke == nil || bound.ValidateRequest == nil || bound.ValidateResult == nil || bound.Recheck == nil || bound.OutputFields == nil || bound.Binding.Kind != call.Tool.Binding.Kind || !canonicalUUID(bound.Authorization.SubjectUUID) || !canonicalUUID(bound.Authorization.DecisionUUID) || bound.Authorization.Actor != call.Tool.Permission.Actor {
		return nil, toolError("skill.tool_protocol_unavailable", "dependency_check", nil)
	}
	prepared := *bound
	bound = &prepared
	if bound.Binding.Kind == "capability" && (bound.Binding.RegistrationVersion == 0 || !boundedText(bound.Binding.AdapterID, 512) || !toolDigestPattern.MatchString(bound.Binding.CatalogSHA256)) {
		return nil, toolError("skill.tool_protocol_unavailable", "dependency_check", nil)
	}
	if bound.Binding.Kind == "builtin" && (bound.Binding.RegistrationVersion != 0 || bound.Binding.AdapterID != "" || bound.Binding.CatalogSHA256 != "") {
		return nil, toolError("skill.tool_protocol_unavailable", "dependency_check", nil)
	}
	if bound.Authorization.Actor == "admin_user" && bound.Authorization.SubjectUUID != reqctx.GetUserUUID(ctx) {
		return nil, toolError("skill.tool_dependency_denied", "permission_check", nil)
	}
	if bound.Authorization.Actor == "service_actor" && (reqctx.GetUserUUID(ctx) != "" || bound.Authorization.SubjectUUID != reqctx.GetSubject(ctx)) {
		return nil, toolError("skill.tool_dependency_denied", "permission_check", nil)
	}
	var requestShape any
	if err := utils.DecodeStrictJSON(call.Request, &requestShape); err != nil {
		return nil, toolError("evidence.calculation_plan_invalid", "input_validation", err)
	}
	if err := bound.ValidateRequest(ctx, append([]byte(nil), call.Request...), append([]ResolvedDatum{}, call.Inputs...)); err != nil {
		var typed *ToolEvidenceError
		if errors.As(err, &typed) {
			return nil, err
		}
		return nil, toolError("evidence.calculation_plan_invalid", "input_validation", err)
	}
	if call.Tool.Binding.Kind == "builtin" {
		if err := r.checkCalculationPlan(call); err != nil {
			return nil, err
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// 在校验输入后、发起副作用前重查；prepare 与网络调用之间也可能撤权或替换实现。
	if err := bound.Recheck(ctx); err != nil {
		if failure, ok := FailureDetails(err); ok {
			return nil, &ToolEvidenceError{ToolFailure: failure, Cause: err}
		}
		return nil, toolError("skill.tool_dependency_unavailable", "dependency_check", err)
	}
	probe, err := json.Marshal(struct {
		Tool   ToolDependency
		Inputs []ResolvedDatum
	}{call.Tool, call.Inputs})
	if err != nil || len(probe) > toolJSONLimit-32768 {
		return nil, toolError("evidence.calculation_plan_invalid", "input_validation", err)
	}
	// 每次调用预留请求、最大输出及回执空间；避免调用发生后才无法记录事实。
	r.mu.Lock()
	if r.calls >= 64 || r.bytesStored+3*toolJSONLimit > maxToolRunBytes {
		r.mu.Unlock()
		return nil, toolError("evidence.calculation_plan_invalid", "input_validation", nil)
	}
	r.calls++
	r.bytesStored += 3 * toolJSONLimit
	r.mu.Unlock()
	receipt := ToolReceipt{Schema: ToolReceiptSchema, ExecutionUUID: uuid.NewString(), TenantUUID: r.identity.TenantUUID, RevisionUUID: r.identity.RevisionUUID, RunID: r.identity.RunID, TraceID: r.identity.TraceID,
		NodeID: call.NodeID, Attempt: call.Attempt, Tool: call.Tool, InvocationBinding: bound.Binding, Authorization: bound.Authorization, Inputs: call.Inputs, StartedAt: time.Now().UTC(), Status: "succeeded"}
	receipt.Request = r.storeArtifact(call.Request)
	result, invokeErr := bound.Invoke(ctx, append([]byte(nil), call.Request...))
	resultOverLimit := len(result) > toolJSONLimit
	if resultOverLimit {
		result = nil
	} else {
		result = append([]byte(nil), result...)
	}
	stage := "tool_invoke"
	if invokeErr == nil {
		invokeErr = ctx.Err()
	}
	if invokeErr == nil {
		stage = "result_validation"
		invokeErr = bound.Recheck(ctx)
		// 也重新检查来源权限；执行中撤权不能将已有副作用隐藏为成功。
		if invokeErr == nil {
			for _, datum := range call.Inputs {
				if err := r.VerifyDatum(ctx, datum); err != nil {
					invokeErr = err
					break
				}
			}
		}
		var resultShape any
		if invokeErr == nil && (resultOverLimit || utils.DecodeStrictJSON(result, &resultShape) != nil) {
			invokeErr = toolError("evidence.tool_result_invalid", stage, nil)
		}
		if invokeErr == nil {
			invokeErr = bound.ValidateResult(ctx, append([]byte(nil), call.Request...), append([]byte(nil), result...))
		}
	}
	fields := map[string]SourceField{}
	if invokeErr == nil {
		for pointer, field := range bound.OutputFields(append([]byte(nil), call.Request...), append([]byte(nil), result...)) {
			fields[pointer] = field
		}
		artifact := r.storeArtifact(result)
		receipt.Result = &artifact
		r.mu.Lock()
		r.outputFields[artifact.ArtifactUUID] = fields
		r.mu.Unlock()
	} else {
		failure := ToolFailure{Code: "evidence.tool_result_invalid", Stage: stage}
		if stage == "tool_invoke" {
			failure.Code = "evidence.calculation_failed"
		}
		if typed, ok := FailureDetails(invokeErr); ok {
			failure = typed
			failure.Stage = stage
		}
		if !boundedText(failure.Code, 512) {
			failure.Code = "evidence.tool_result_invalid"
		}
		receipt.Status = "failed"
		if errors.Is(invokeErr, context.DeadlineExceeded) {
			failure = ToolFailure{Code: "skill.tool_timeout", Stage: stage, Retryable: true}
			receipt.Status = "timed_out"
		}
		if errors.Is(invokeErr, context.Canceled) {
			failure = ToolFailure{Code: "skill.tool_cancelled", Stage: stage}
			receipt.Status = "cancelled"
		}
		receipt.Error = &failure
		invokeErr = &ToolEvidenceError{ToolFailure: failure, Cause: invokeErr}
	}
	receipt.EndedAt = time.Now().UTC()
	raw, err := json.Marshal(receipt)
	if err != nil || len(raw) > toolJSONLimit {
		return nil, toolError("evidence.tool_result_invalid", "result_validation", err)
	}
	r.mu.Lock()
	r.receipts[receipt.ExecutionUUID] = append([]byte(nil), raw...)
	r.rechecks[receipt.ExecutionUUID] = bound.Recheck
	storedResultSize := 0
	if receipt.Result != nil {
		storedResultSize = len(result)
	}
	r.bytesStored -= 3*toolJSONLimit - len(call.Request) - storedResultSize - len(raw)
	r.mu.Unlock()
	// 防止返回的 slice/pointer 修改 ledger 中原件。
	var out ToolReceipt
	if err := DecodeToolJSON(raw, &out); err != nil {
		return nil, toolError("evidence.tool_result_invalid", "result_validation", err)
	}
	return &out, invokeErr
}

func (r *ToolRun) checkCalculationPlan(call ToolCall) error {
	plan, ok := r.policy.Calculations[call.NodeID]
	var request CalculationRequest
	if !ok || DecodeToolJSON(call.Request, &request) != nil || request.Expression != plan.Expression || request.Precision != plan.Precision || request.Percent != plan.Percent || request.ResultUnit != plan.ResultUnit || len(request.Operands) != len(plan.OperandFields) {
		return toolError("evidence.calculation_plan_invalid", "input_validation", nil)
	}
	fields := map[string]string{}
	for _, datum := range call.Inputs {
		fields[datum.DatumUUID] = datum.FieldKey
	}
	for variable, field := range plan.OperandFields {
		if operand, ok := request.Operands[variable]; !ok || fields[operand.DatumUUID] != field {
			return toolError("evidence.calculation_plan_invalid", "input_validation", nil)
		}
	}
	return nil
}

func (r *ToolRun) storeArtifact(raw []byte) ToolArtifact {
	artifact := ToolArtifact{ArtifactUUID: uuid.NewString(), SHA256: toolDigest(raw)}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.artifacts[artifact.ArtifactUUID] = append([]byte(nil), raw...)
	return artifact
}

// VerifyReceipt 只核对同次执行可信记录与字节；不会从回执恢复权限或导入其它 run。
func (r *ToolRun) VerifyReceipt(ctx context.Context, receipt ToolReceipt) error {
	if err := r.checkContext(ctx); err != nil {
		return err
	}
	deny := func() error { return toolError("evidence.tool_receipt_untrusted", "evidence_validation", nil) }
	if receipt.TenantUUID != r.identity.TenantUUID || receipt.RevisionUUID != r.identity.RevisionUUID || receipt.RunID != r.identity.RunID || receipt.TraceID != r.identity.TraceID {
		return deny()
	}
	raw, err := json.Marshal(receipt)
	if err != nil {
		return deny()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !bytes.Equal(raw, r.receipts[receipt.ExecutionUUID]) || toolDigest(r.artifacts[receipt.Request.ArtifactUUID]) != receipt.Request.SHA256 {
		return deny()
	}
	if receipt.Result != nil && toolDigest(r.artifacts[receipt.Result.ArtifactUUID]) != receipt.Result.SHA256 {
		return deny()
	}
	return nil
}

// VerifyResult 同时检查真实回执、实际输出 Pointer 和精确标量；数值相等不能替代回执。
func (r *ToolRun) VerifyResult(ctx context.Context, receipt ToolReceipt, pointer, value string) error {
	if err := r.VerifyReceipt(ctx, receipt); err != nil {
		return err
	}
	if receipt.Status != "succeeded" || receipt.Result == nil {
		return toolError("evidence.tool_receipt_untrusted", "evidence_validation", nil)
	}
	r.mu.Lock()
	raw := append([]byte(nil), r.artifacts[receipt.Result.ArtifactUUID]...)
	_, declared := r.outputFields[receipt.Result.ArtifactUUID][pointer]
	r.mu.Unlock()
	if !declared || len(value) > 1280 || !toolFiniteDecimal.MatchString(value) {
		return toolError("evidence.tool_result_invalid", "result_validation", nil)
	}
	actual, err := snapshotPointer(raw, pointer)
	if err != nil {
		return toolError("evidence.tool_result_invalid", "result_validation", err)
	}
	var scalar string
	switch v := actual.(type) {
	case string:
		scalar = v
	case json.Number:
		scalar = v.String()
	default:
		return toolError("evidence.tool_result_invalid", "result_validation", nil)
	}
	if scalar != value {
		return toolError("evidence.tool_result_invalid", "result_validation", nil)
	}
	return nil
}

// ToolResultReference 是报告引用的最小核验单元，单位和 scope 同样不能由展示层改写。
type ToolResultReference struct{ Pointer, Value, Unit, Scope string }

func (r *ToolRun) VerifyResultReference(ctx context.Context, receipt ToolReceipt, ref ToolResultReference) error {
	if err := r.VerifyResult(ctx, receipt, ref.Pointer, ref.Value); err != nil {
		return err
	}
	r.mu.Lock()
	field := r.outputFields[receipt.Result.ArtifactUUID][ref.Pointer]
	r.mu.Unlock()
	if field.Unit != ref.Unit || field.Scope != ref.Scope {
		return toolError("evidence.tool_result_invalid", "result_validation", nil)
	}
	if err := r.checkExecutionAccess(ctx, receipt); err != nil {
		return err
	}
	// ledger 不能替代实时源对象授权；历史展示不得调用此执行入口。
	for _, datum := range receipt.Inputs {
		if err := r.VerifyDatum(ctx, datum); err != nil {
			return err
		}
	}
	return nil
}

func (r *ToolRun) checkExecutionAccess(ctx context.Context, receipt ToolReceipt) error {
	r.mu.Lock()
	recheck := r.rechecks[receipt.ExecutionUUID]
	r.mu.Unlock()
	if recheck == nil {
		return toolError("evidence.tool_receipt_untrusted", "evidence_validation", nil)
	}
	if err := recheck(ctx); err != nil {
		var typed *ToolEvidenceError
		if errors.As(err, &typed) {
			return err
		}
		return toolError("skill.tool_dependency_denied", "permission_check", err)
	}
	return nil
}

// ReadArtifact 供受信任平台持久化原始执行事实，返回本次字节的副本；不是客户端授权入口。
// 对外消费必须先 VerifyResultReference/VerifyDatum；没有外部 UUID/URL fallback。
func (r *ToolRun) ReadArtifact(ctx context.Context, artifact ToolArtifact) ([]byte, error) {
	if err := r.checkContext(ctx); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	raw, ok := r.artifacts[artifact.ArtifactUUID]
	if !ok || toolDigest(raw) != artifact.SHA256 {
		return nil, toolError("evidence.tool_receipt_untrusted", "evidence_validation", nil)
	}
	return append([]byte(nil), raw...), nil
}

func (r *ToolRun) prepare(ctx context.Context, tool ToolDependency) (*BoundTool, error) {
	if tool.Binding.Kind != "builtin" {
		if tool.Binding.Kind != "capability" || r.service.capability == nil {
			return nil, toolError("skill.tool_protocol_unavailable", "dependency_check", nil)
		}
		return r.service.capability(ctx, r.identity, tool)
	}
	if err := r.service.CheckCalculator(tool); err != nil {
		return nil, err
	}
	return &BoundTool{Binding: ToolInvocationBinding{Kind: "builtin"}, Authorization: ToolAuthorization{DecisionUUID: uuid.NewString(), SubjectUUID: r.service.subjectUUID, Actor: "runtime_internal"},
		ValidateRequest: validateCalculationRequest,
		Invoke: func(ctx context.Context, raw []byte) ([]byte, error) {
			var request CalculationRequest
			if err := DecodeToolJSON(raw, &request); err != nil {
				return nil, err
			}
			bindings := map[string]string{}
			for key, operand := range request.Operands {
				bindings[key] = operand.Value
			}
			value, err := Calculate(ctx, request.Expression, bindings, request.Precision, request.Percent)
			if err != nil {
				return nil, err
			}
			return json.Marshal(CalculationResult{Schema: CalculationResultSchema, Value: strings.TrimSuffix(value, "%"), Unit: request.ResultUnit, Precision: request.Precision, Percent: request.Percent, Rounding: request.Rounding})
		},
		ValidateResult: func(_ context.Context, requestRaw, resultRaw []byte) error {
			var request CalculationRequest
			var result CalculationResult
			if DecodeToolJSON(requestRaw, &request) != nil || DecodeToolJSON(resultRaw, &result) != nil || result.Schema != CalculationResultSchema || result.Unit != request.ResultUnit || result.Precision != request.Precision || result.Percent != request.Percent || result.Rounding != request.Rounding || len(result.Value) > 1280 {
				return toolError("evidence.tool_result_invalid", "result_validation", nil)
			}
			return nil
		},
		Recheck: func(ctx context.Context) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			return r.service.CheckCalculator(tool)
		},
		OutputFields: func(requestRaw, _ []byte) map[string]SourceField {
			var request CalculationRequest
			_ = DecodeToolJSON(requestRaw, &request)
			scope := ""
			for _, operand := range request.Operands {
				scope = operand.Scope
				break
			}
			kind := "quantity"
			if request.Percent {
				kind = "reported"
			}
			return map[string]SourceField{"/value": {FieldKey: "value", LabelKey: "calculation.value", DatumKind: kind, Unit: request.ResultUnit, Scope: scope, NumericString: true}}
		},
	}, nil
}

func validateCalculationRequest(_ context.Context, raw []byte, inputs []ResolvedDatum) error {
	fail := func(cause error) error {
		return toolError("evidence.calculation_plan_invalid", "input_validation", cause)
	}
	var request CalculationRequest
	if err := DecodeToolJSON(raw, &request); err != nil {
		return fail(err)
	}
	if request.Schema != CalculationRequestSchema || request.Rounding != HalfAwayFromZero || request.Precision < 0 || request.Precision > 12 || (request.Percent != (request.ResultUnit == "%")) || !boundedText(request.ResultUnit, 512) || len(request.Operands) == 0 || len(request.Operands) > 32 {
		return fail(nil)
	}
	byUUID := map[string]ResolvedDatum{}
	for _, datum := range inputs {
		byUUID[datum.DatumUUID] = datum
	}
	used := map[string]bool{}
	bindings := map[string]string{}
	scope, unit := "", ""
	for variable, operand := range request.Operands {
		datum, ok := byUUID[operand.DatumUUID]
		if !keyPattern.MatchString(variable) || !ok || datum.DatumKind != "quantity" || datum.Unit == "%" || datum.Unit == "％" || datum.Value != operand.Value || datum.Unit != operand.Unit || datum.Scope != operand.Scope {
			return fail(nil)
		}
		if scope != "" && (scope != operand.Scope || unit != operand.Unit) {
			return fail(nil)
		}
		scope, unit = operand.Scope, operand.Unit
		bindings[variable] = datum.Value
		used[datum.DatumUUID] = true
	}
	if len(used) != len(byUUID) {
		return fail(nil)
	}
	if err := validateBoundExpression(request.Expression, bindings); err != nil {
		return fail(err)
	}
	return nil
}

// Receipts 返回稳定排序的副本供上层持久化；绝不导入历史到当前 ledger。
func (r *ToolRun) Receipts(ctx context.Context) ([]ToolReceipt, error) {
	if err := r.checkContext(ctx); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	keys := make([]string, 0, len(r.receipts))
	for key := range r.receipts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]ToolReceipt, 0, len(keys))
	for _, key := range keys {
		var receipt ToolReceipt
		if err := DecodeToolJSON(r.receipts[key], &receipt); err != nil {
			return nil, err
		}
		result = append(result, receipt)
	}
	return result, nil
}

// DescribeTool 只从受信任 adapter 读取固定合同元数据，不接受 payload 或模型补齐字段。
func (s *ToolService) DescribeTool(ctx context.Context, id ToolRunIdentity, tool ToolDependency) (ToolDescription, error) {
	if tool.Binding.Kind != "capability" || s.capability == nil {
		return ToolDescription{}, toolError("skill.tool_protocol_unavailable", "dependency_check", nil)
	}
	bound, err := s.capability(ctx, id, tool)
	if err != nil {
		return ToolDescription{}, err
	}
	return bound.Description, nil
}
