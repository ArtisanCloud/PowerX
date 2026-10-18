package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	agentauthz "github.com/ArtisanCloud/PowerX/internal/service/agent_authz"
	"github.com/ArtisanCloud/PowerX/pkg/corex/agent/evidence"
	registrymodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	skillrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/skills"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/utils"
	"github.com/google/uuid"
)

// ToolDependencySchemaV2 是 T199 固定身份、Schema、实现及权限的依赖格式。
const ToolDependencySchemaV2 = evidence.ToolRequirementsSchema

// DependencyStage 区分发布者检查、目标 Agent 绑定和每次实际执行。
type DependencyStage string

const (
	DependencyPublish DependencyStage = "publish"
	DependencyBind    DependencyStage = "bind"
	DependencyExecute DependencyStage = "execute"
)

// DependencyCheckInput 由服务端传入阶段与 Agent，身份权限只从可信请求上下文读取。
type DependencyCheckInput struct {
	TenantUUID string
	Stage      DependencyStage
	Env        string
	AgentID    uint64
	Definition map[string]any
}

// V2 DTO 与可信执行边界共享，避免预检和回执分别维护协议。
type ToolSchemaRef = evidence.ToolSchemaRef
type ToolPermission = evidence.ToolPermission
type ToolBinding = evidence.ToolBinding
type ToolDependencyV2 = evidence.ToolDependency
type ToolRequirementsV2 = evidence.ToolRequirements

// ToolCatalog 支持三阶段共用仓储及测试替身，不把 DB IO 放入检查逻辑。
type ToolCatalog interface {
	LoadToolCapability(context.Context, string, string, string) (*skillrepo.ToolCapabilitySnapshot, error)
}

// ToolAuthorizer 根据当前主体、阶段及目标 Agent 重新判定权限。
type ToolAuthorizer func(context.Context, DependencyCheckInput, ToolDependencyV2, *skillrepo.ToolCapabilitySnapshot) error

// PinnedToolChecker 只有在实际 invoker 能消费并固定同一版本/实现/Schema 时才允许装配。
// 目录元数据或 provider 自报版本不构成实现证明。当前生产支持内置实现与显式固定的原生插件 HTTP 计算合同，其余端口保持失败关闭。
type PinnedToolChecker func(context.Context, DependencyCheckInput, ToolDependencyV2, *skillrepo.ToolCapabilitySnapshot) error

// DependencyServiceOptions 仅由受信任装配提供端口和部署支持集合。
type DependencyServiceOptions struct {
	Catalog           ToolCatalog
	Authorize         ToolAuthorizer
	PinnedTools       PinnedToolChecker
	Executors         map[string]bool
	ResponseContracts map[string]bool
	EvidenceReady     func(context.Context) error
}

// DependencyService 统一治理态 Skill 的发布、绑定和执行前检查。
type DependencyService struct{ options DependencyServiceOptions }

// NewDependencyService 冻结部署能力集合；不缓存目录或授权允许结果。
func NewDependencyService(options DependencyServiceOptions) *DependencyService {
	options.Executors = cloneDependencyFlags(options.Executors)
	options.ResponseContracts = cloneDependencyFlags(options.ResponseContracts)
	return &DependencyService{options: options}
}

func cloneDependencyFlags(values map[string]bool) map[string]bool {
	out := make(map[string]bool, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

// DependencyError 保留稳定错误和检查阶段，避免把底层 SQL/对象存在性直接暴露给调用方。
type DependencyError struct {
	Code  string
	Stage DependencyStage
	Key   string
	Cause error
}

func (e *DependencyError) Error() string {
	return fmt.Sprintf("%s: stage=%s dependency=%s", e.Code, e.Stage, e.Key)
}
func (e *DependencyError) Unwrap() error { return e.Cause }
func (e *DependencyError) EvidenceFailure() evidence.ToolFailure {
	stage := "dependency_check"
	if strings.Contains(e.Code, "denied") {
		stage = "permission_check"
	}
	return evidence.ToolFailure{Code: e.Code, Stage: stage}
}

func dependencyError(in DependencyCheckInput, code, key string, cause error) error {
	return &DependencyError{Code: code, Stage: in.Stage, Key: key, Cause: cause}
}

// Check 在每个阶段重新读取依赖与授权，不缓存发布时的允许决定。
func (s *DependencyService) Check(ctx context.Context, in DependencyCheckInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	fail := func(code string) error { return dependencyError(in, code, "", nil) }
	if s == nil {
		return fail("skill.tool_dependency_unavailable")
	}
	if in.Stage != DependencyPublish && in.Stage != DependencyBind && in.Stage != DependencyExecute {
		return fail("skill.tool_dependency_invalid")
	}
	executor := nestedManifestMap(in.Definition, "executor")
	typ := strings.TrimSpace(asStringInterface(executor["type"]))
	if typ == "instruction_only" {
		return fail("skill.executor_instruction_only_not_runnable")
	}
	if !s.options.Executors[typ] {
		return fail("skill.executor_unavailable")
	}
	if _, err := ManifestLLMParameters(nestedManifestMap(executor, "model_policy")); err != nil {
		return err
	}
	raw, exists := in.Definition["tool_dependencies"]
	if !exists {
		if typ == "capability" {
			return fail("skill.tool_dependencies_required")
		}
		return CheckToolDependencies(ctx, in.TenantUUID, in.Definition, nil)
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return fail("skill.tool_dependency_invalid")
	}
	if len(encoded) > 1024*1024 {
		return fail("skill.tool_dependency_invalid")
	}
	var header struct {
		Schema string `json:"schema"`
	}
	if err := json.Unmarshal(encoded, &header); err != nil {
		return fail("skill.tool_dependency_invalid")
	}
	if header.Schema == ToolDependencySchema {
		// 兼容窗口仅保留已实现的 V4 内置计算；旧外部 key-only 声明须显式升级。
		var legacy ToolRequirements
		if err := utils.DecodeStrictJSON(encoded, &legacy); err != nil {
			return fail("skill.tool_dependency_invalid")
		}
		for _, tool := range legacy.Tools {
			if tool.ToolKey != evidence.CalculatorKey {
				return fail("skill.tool_contract_upgrade_required")
			}
		}
		if typ == "capability" {
			return fail("skill.tool_contract_upgrade_required")
		}
		return CheckToolDependencies(ctx, in.TenantUUID, in.Definition, nil)
	}
	if header.Schema != ToolDependencySchemaV2 {
		return fail("skill.tool_contract_upgrade_required")
	}
	if !canonicalDependencyUUID(in.TenantUUID) {
		return fail("skill.definition_runtime_context_invalid")
	}
	var requirements ToolRequirementsV2
	if err := utils.DecodeStrictJSON(encoded, &requirements); err != nil {
		return fail("skill.tool_dependency_invalid")
	}
	if len(requirements.Tools) == 0 || len(requirements.Tools) > 64 {
		return fail("skill.tool_dependency_invalid")
	}
	seenKeys, seenTools := map[string]bool{}, map[string]bool{}
	capabilityCount := 0
	for _, tool := range requirements.Tools {
		if !validToolDependency(tool) || seenKeys[tool.DependencyKey] || seenTools[tool.ToolKey] {
			return fail("skill.tool_dependency_invalid")
		}
		seenKeys[tool.DependencyKey], seenTools[tool.ToolKey] = true, true
		if tool.Binding.Kind == "capability" {
			capabilityCount++
		}
	}
	if typ == "capability" && (capabilityCount != 1 || len(requirements.Tools) != 1 || requirements.Tools[0].ToolKey != asStringInterface(executor["capability_id"])) {
		return fail("skill.tool_dependency_invalid")
	}
	// 外部多工具计划尚未接入；只允许已经接线的 typed 内置计算流程。
	if typ != "capability" && capabilityCount > 0 {
		return fail("skill.executor_unavailable")
	}
	for _, tool := range requirements.Tools {
		if err := s.checkTool(ctx, in, tool); err != nil {
			return err
		}
	}
	// V4 只能消费旧工具；普通 prompt 也没有调用 V2 计算工具的阶段。
	if typ == "llm_prompt" && (asStringInterface(executor["output_mode"]) != "response_envelope" || asStringInterface(executor["response_contract"]) != evidence.ReportSchemaV5 || !s.options.ResponseContracts[evidence.ReportSchemaV5]) {
		return fail("skill.executor_unavailable")
	}
	if typ == "llm_prompt" {
		// 当前受控历史只支持用户态，未装配 service_actor/API-key 的读取授权。
		if in.Stage == DependencyExecute && (reqctx.GetTenantUUID(ctx) != in.TenantUUID || !canonicalDependencyUUID(reqctx.GetUserUUID(ctx)) || reqctx.AuthenticatedAPIKeyHash(ctx) != "" || reqctx.GetSubject(ctx) == "service_actor") {
			return fail("skill.tool_dependency_denied")
		}
		policy, err := evidence.ReadCalculationPolicy(executor["calculation_policy"])
		if err != nil {
			return err
		}
		if policy.Schema != evidence.PolicySchemaV3 {
			return fail("skill.calculation_policy_upgrade_required")
		}
		if len(evidenceSourcePointers(executor)) == 0 {
			return fail("skill.evidence_sources_required")
		}
		if _, err := evidence.ReadReviewPolicy(executor["review_policy"], policy); err != nil {
			return err
		}
		if err := validateV5PolicyLocales(executor, policy); err != nil {
			return err
		}
		if s.options.EvidenceReady == nil {
			return fail("skill.executor_unavailable")
		}
		if err := s.options.EvidenceReady(ctx); err != nil {
			return dependencyError(in, "evidence.storage_unavailable", "", err)
		}
	}
	if typ == "capability" {
		if asStringInterface(executor["output_mode"]) != "response_envelope" || asStringInterface(executor["response_contract"]) != evidence.ReportSchemaV5 || !s.options.ResponseContracts[evidence.ReportSchemaV5] {
			return fail("skill.executor_unavailable")
		}
		if s.options.EvidenceReady == nil {
			return fail("evidence.storage_unavailable")
		}
		if err := s.options.EvidenceReady(ctx); err != nil {
			return dependencyError(in, "evidence.storage_unavailable", "", err)
		}
	}
	return nil
}

func validateV5PolicyLocales(executor map[string]any, policy evidence.CalculationPolicy) error {
	prompts := nestedManifestMap(executor, "prompt_template_i18n")
	if len(prompts) == 0 {
		return fmt.Errorf("skill.definition_llm_prompt_i18n_required")
	}
	for locale := range prompts {
		if locale != "en-US" && locale != "zh-CN" {
			return fmt.Errorf("skill.executor_locale_not_supported")
		}
		for _, profile := range policy.ActivityProfiles {
			if profile.LabelI18n[locale] == "" || len(profile.EvidenceAnyI18n[locale]) == 0 {
				return fmt.Errorf("skill.calculation_locale_required")
			}
		}
		for _, field := range policy.InputFields {
			if field.LabelI18n[locale] == "" || field.DescriptionI18n[locale] == "" || field.ScopeI18n[locale] == "" || len(field.EvidenceTermsI18n[locale]) == 0 {
				return fmt.Errorf("skill.calculation_locale_required")
			}
		}
		for _, formula := range policy.Formulas {
			if formula.LabelI18n[locale] == "" {
				return fmt.Errorf("skill.calculation_locale_required")
			}
		}
		review, err := evidence.ReadReviewPolicy(executor["review_policy"], policy)
		if err != nil {
			return err
		}
		if review != nil {
			for _, rule := range review.Rules {
				if rule.TextI18n[locale] == "" {
					return fmt.Errorf("skill.review_rule_locale_required")
				}
			}
		}
	}
	return nil
}

func (s *DependencyService) checkTool(ctx context.Context, in DependencyCheckInput, tool ToolDependencyV2) error {
	fail := func(code string, cause error) error { return dependencyError(in, code, tool.DependencyKey, cause) }
	if tool.Binding.Kind == "builtin" {
		if tool.ToolKey != evidence.CalculatorKey {
			return fail("skill.tool_dependency_unavailable", nil)
		}
		if tool.Version != evidence.TypedCalculatorVersion || tool.InputSchema.ID != evidence.CalculationRequestSchema || tool.OutputSchema.ID != evidence.CalculationResultSchema {
			return fail("skill.tool_dependency_version_or_contract_mismatch", nil)
		}
		if s.options.PinnedTools == nil {
			return fail("skill.tool_dependency_unavailable", nil)
		}
		return s.options.PinnedTools(ctx, in, tool, nil)
	}
	if s.options.Catalog == nil {
		return fail("skill.tool_dependency_unavailable", nil)
	}
	snapshot, err := s.options.Catalog.LoadToolCapability(ctx, in.TenantUUID, tool.Binding.CapabilityUUID, tool.Binding.ContractUUID)
	if err != nil || snapshot == nil {
		return fail("skill.tool_dependency_unavailable", err)
	}
	r, c, reg := snapshot.Record, snapshot.Contract, snapshot.Registration
	now := time.Now()
	if r.PublishedAt == nil || r.PublishedAt.IsZero() || r.PublishedAt.After(now) || !slices.Contains([]string{"active", "published", "ready"}, r.Status) || c.Status != 1 || c.LifecycleState != "published" || (c.EffectiveAt != nil && c.EffectiveAt.After(now)) || (c.DeprecatedAt != nil && !c.DeprecatedAt.After(now)) {
		return fail("skill.tool_dependency_unavailable", nil)
	}
	if r.UUID.String() != tool.Binding.CapabilityUUID || c.ContractUUID.String() != tool.Binding.ContractUUID || c.TenantUUID != in.TenantUUID || c.CapabilityKey != tool.ToolKey || r.CapabilityID != tool.ToolKey {
		return fail("skill.tool_dependency_invalid", nil)
	}
	if r.PluginVersion != tool.Binding.ProviderVersion || c.Version != tool.Version || c.ProviderID != r.PluginID || !matchesToolSchemas(tool, snapshot) {
		return fail("skill.tool_dependency_version_or_contract_mismatch", nil)
	}
	// 现有 registration.contract_ref 存的是工具合同版本；UUID 已由同租户
	// (capability_key, version) 唯一合同核对，不改变 registration 字段原义。
	if reg.TenantUUID != in.TenantUUID || reg.CapabilityID != tool.ToolKey || reg.PublishedAt == nil || reg.PublishedAt.IsZero() || reg.PublishedAt.After(now) || !slices.Contains([]string{"active", "published", "ready", "enabled"}, reg.Status) || reg.ContractRef != c.Version {
		return fail("skill.tool_dependency_denied", nil)
	}
	if !slices.Contains(agentauthz.CapabilityPermissionCodes(r), tool.Permission.Code) {
		return fail("skill.tool_dependency_denied", nil)
	}
	if !agentauthz.CapabilityAgentUsable(r) {
		return fail("skill.tool_dependency_denied", nil)
	}
	var protocols []registrymodel.ProtocolBinding
	if err := json.Unmarshal(r.Protocols, &protocols); err != nil {
		return fail("skill.tool_dependency_invalid", err)
	}
	found := false
	for _, protocol := range protocols {
		if protocol.Channel == tool.Binding.Protocol && protocol.ActorContext == tool.Permission.Actor {
			if protocol.Channel == "core_internal" && (!strings.HasPrefix(protocol.Endpoint, "core://") || protocol.Method != "INVOKE") {
				return fail("skill.tool_dependency_invalid", nil)
			}
			found = true
		}
	}
	adapterFound := false
	contractTransport := false
	for _, transport := range c.TransportProfiles {
		if transport != nil && transport.Status == 1 && transport.TenantUUID == in.TenantUUID && transport.ContractID == c.ID && transport.CapabilityKey == tool.ToolKey && transport.Transport == tool.Binding.Protocol {
			contractTransport = true
		}
	}
	for _, adapter := range reg.Adapters {
		if adapter.IsActive && adapter.TransportType == tool.Binding.Protocol {
			adapterFound = true
		}
	}
	if !found || !adapterFound || !contractTransport {
		return fail("skill.tool_protocol_unavailable", nil)
	}
	if s.options.Authorize == nil {
		return fail("skill.tool_dependency_denied", nil)
	}
	if err := s.options.Authorize(ctx, in, tool, snapshot); err != nil {
		return fail("skill.tool_dependency_denied", err)
	}
	// 安装目录和 contract 不是二进制证明；缺少实际 pinning 端口时始终拒绝。
	if s.options.PinnedTools == nil {
		return fail("skill.tool_protocol_unavailable", nil)
	}
	return s.options.PinnedTools(ctx, in, tool, snapshot)
}

func matchesToolSchemas(tool ToolDependencyV2, snapshot *skillrepo.ToolCapabilitySnapshot) bool {
	input, output := false, false
	for _, schema := range snapshot.Contract.IOSchemas {
		if schema == nil || schema.Status != 1 || schema.TenantUUID != snapshot.Contract.TenantUUID || schema.ContractID != snapshot.Contract.ID {
			continue
		}
		if schema.Direction == "input" && schema.SchemaURI == tool.InputSchema.ID && schema.SchemaHash == tool.InputSchema.SHA256 {
			input = true
		}
		if schema.Direction == "output" && schema.SchemaURI == tool.OutputSchema.ID && schema.SchemaHash == tool.OutputSchema.SHA256 {
			output = true
		}
	}
	return input && output
}

var toolLocalKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

func canonicalDependencyUUID(value string) bool {
	parsed, err := uuid.Parse(value)
	return err == nil && parsed != uuid.Nil && parsed.String() == value
}

func validToolDependency(tool ToolDependencyV2) bool { return evidence.ValidToolDependency(tool) }
