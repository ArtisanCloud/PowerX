package evidence

import (
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/utils"
)

const (
	ToolRequirementsSchema   = "powerx.skill-tools/v2"
	ToolReceiptSchema        = "powerx.tool-receipt/v1"
	CalculationRequestSchema = "powerx.calculation.request/v1"
	CalculationResultSchema  = "powerx.calculation.result/v1"
	TypedCalculatorVersion   = "2.0.0"
	HalfAwayFromZero         = "half_away_from_zero"
	toolJSONLimit            = 1024 * 1024
)

//go:embed contracts/tool-evidence.v1.schema.json
var toolContractJSON []byte

// ToolContractBytes 返回随 Core 发布的完整合同原件；不读取模型提供的 Schema 地址。
func ToolContractBytes() []byte { return append([]byte(nil), toolContractJSON...) }

func toolDigest(raw []byte) string { return fmt.Sprintf("sha256:%x", sha256.Sum256(raw)) }

var exactToolVersion = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-((0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*))(\.(0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*))*)?(\+[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$`)
var versionedToolSchema = regexp.MustCompile(`^.+/v[1-9][0-9]*$`)

// ValidToolDependency 同时供发布预检和实际执行使用，避免两套协议校验漂移。
func ValidToolDependency(tool ToolDependency) bool {
	if !keyPattern.MatchString(tool.DependencyKey) || !boundedText(tool.ToolKey, 512) || strings.TrimSpace(tool.ToolKey) == "" || len(tool.Version) > 128 || !exactToolVersion.MatchString(tool.Version) || !toolDigestPattern.MatchString(tool.Binding.ImplementationSHA256) {
		return false
	}
	for _, schema := range []ToolSchemaRef{tool.InputSchema, tool.OutputSchema} {
		if !versionedToolSchema.MatchString(schema.ID) || len(schema.ID) > 256 || !toolDigestPattern.MatchString(schema.SHA256) {
			return false
		}
	}
	if tool.Binding.Kind == "builtin" {
		return tool.Permission.Actor == "runtime_internal" && tool.Permission.Code == "pure_calculation" && tool.Binding.CapabilityUUID == "" && tool.Binding.ContractUUID == "" && tool.Binding.ProviderVersion == "" && tool.Binding.Protocol == ""
	}
	return tool.Binding.Kind == "capability" && canonicalUUID(tool.Binding.CapabilityUUID) && canonicalUUID(tool.Binding.ContractUUID) && len(tool.Binding.ProviderVersion) <= 128 && exactToolVersion.MatchString(tool.Binding.ProviderVersion) && slices.Contains([]string{"core_internal", "http", "rest", "grpc", "mcp"}, tool.Binding.Protocol) && slices.Contains([]string{"admin_user", "service_actor"}, tool.Permission.Actor) && boundedText(tool.Permission.Code, 512) && strings.TrimSpace(tool.Permission.Code) != ""
}

type ToolSchemaRef struct {
	ID     string `json:"id"`
	SHA256 string `json:"sha256"`
}
type ToolPermission struct {
	Actor string `json:"actor"`
	Code  string `json:"code"`
}
type ToolBinding struct {
	Kind                 string `json:"kind"`
	ImplementationSHA256 string `json:"implementation_sha256"`
	CapabilityUUID       string `json:"capability_uuid,omitempty"`
	ContractUUID         string `json:"contract_uuid,omitempty"`
	ProviderVersion      string `json:"provider_version,omitempty"`
	Protocol             string `json:"protocol,omitempty"`
}

func (b *ToolBinding) UnmarshalJSON(raw []byte) error {
	var tag struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(raw, &tag); err != nil {
		return err
	}
	if tag.Kind == "builtin" {
		var value struct {
			Kind                 string `json:"kind"`
			ImplementationSHA256 string `json:"implementation_sha256"`
		}
		if err := utils.DecodeStrictJSON(raw, &value); err != nil {
			return err
		}
		*b = ToolBinding{Kind: value.Kind, ImplementationSHA256: value.ImplementationSHA256}
		return nil
	}
	type binding ToolBinding
	var value binding
	if err := utils.DecodeStrictJSON(raw, &value); err != nil {
		return err
	}
	*b = ToolBinding(value)
	return nil
}

type ToolDependency struct {
	DependencyKey string         `json:"dependency_key"`
	ToolKey       string         `json:"tool_key"`
	Version       string         `json:"version"`
	InputSchema   ToolSchemaRef  `json:"input_schema"`
	OutputSchema  ToolSchemaRef  `json:"output_schema"`
	Permission    ToolPermission `json:"permission"`
	Binding       ToolBinding    `json:"binding"`
}
type ToolRequirements struct {
	Schema string           `json:"schema"`
	Tools  []ToolDependency `json:"tools"`
}
type ByteRange struct {
	StartByte int `json:"start_byte"`
	EndByte   int `json:"end_byte"`
}
type DatumSource struct {
	SourceUUID     string    `json:"source_uuid"`
	Kind           string    `json:"kind"`
	ObjectType     string    `json:"object_type"`
	ObjectUUID     string    `json:"object_uuid"`
	SnapshotUUID   string    `json:"snapshot_uuid"`
	SnapshotSHA256 string    `json:"snapshot_sha256"`
	Pointer        string    `json:"pointer"`
	Quote          string    `json:"quote"`
	Range          ByteRange `json:"range"`
	ExecutionUUID  string    `json:"execution_uuid,omitempty"`
}

// ResolvedDatum 的 Value/InputState/UUID 只能由来源解析器产生。
type ResolvedDatum struct {
	DatumUUID  string      `json:"datum_uuid"`
	FieldKey   string      `json:"field_key"`
	LabelKey   string      `json:"label_key"`
	DatumKind  string      `json:"datum_kind"`
	Literal    string      `json:"literal"`
	Value      string      `json:"value"`
	Unit       string      `json:"unit"`
	Scope      string      `json:"scope"`
	InputState string      `json:"input_state"`
	Source     DatumSource `json:"source"`
}
type ToolArtifact struct {
	ArtifactUUID string `json:"artifact_uuid"`
	SHA256       string `json:"sha256"`
}
type ToolInvocationBinding struct {
	Kind                string `json:"kind"`
	RegistrationVersion uint64 `json:"registration_version,omitempty"`
	AdapterID           string `json:"adapter_id,omitempty"`
	CatalogSHA256       string `json:"catalog_sha256,omitempty"`
}

func (b *ToolInvocationBinding) UnmarshalJSON(raw []byte) error {
	var tag struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(raw, &tag); err != nil {
		return err
	}
	if tag.Kind == "builtin" {
		var value struct {
			Kind string `json:"kind"`
		}
		if err := utils.DecodeStrictJSON(raw, &value); err != nil {
			return err
		}
		*b = ToolInvocationBinding{Kind: value.Kind}
		return nil
	}
	type binding ToolInvocationBinding
	var value binding
	if err := utils.DecodeStrictJSON(raw, &value); err != nil {
		return err
	}
	*b = ToolInvocationBinding(value)
	return nil
}

type ToolAuthorization struct {
	DecisionUUID string `json:"decision_uuid"`
	SubjectUUID  string `json:"subject_uuid"`
	Actor        string `json:"actor"`
}
type ToolFailure struct {
	Code      string `json:"code"`
	Stage     string `json:"stage"`
	Retryable bool   `json:"retryable"`
}

// ToolEvidenceError 不向调用方泄露来源是否存在、底层 SQL、凭据或 provider 响应。
type ToolEvidenceError struct {
	ToolFailure
	Cause error
}

func (e *ToolEvidenceError) Error() string { return e.Code }
func (e *ToolEvidenceError) Unwrap() error { return e.Cause }
func toolError(code, stage string, cause error) error {
	return &ToolEvidenceError{ToolFailure: ToolFailure{Code: code, Stage: stage}, Cause: cause}
}

type ToolReceipt struct {
	Schema            string                `json:"schema"`
	ExecutionUUID     string                `json:"execution_uuid"`
	TenantUUID        string                `json:"tenant_uuid"`
	RevisionUUID      string                `json:"revision_uuid"`
	RunID             string                `json:"run_id"`
	TraceID           string                `json:"trace_id"`
	NodeID            string                `json:"node_id"`
	Attempt           int                   `json:"attempt"`
	Tool              ToolDependency        `json:"tool"`
	InvocationBinding ToolInvocationBinding `json:"invocation_binding"`
	Authorization     ToolAuthorization     `json:"authorization"`
	Inputs            []ResolvedDatum       `json:"inputs"`
	Request           ToolArtifact          `json:"request"`
	StartedAt         time.Time             `json:"started_at"`
	EndedAt           time.Time             `json:"ended_at"`
	Status            string                `json:"status"`
	Result            *ToolArtifact         `json:"result,omitempty"`
	Error             *ToolFailure          `json:"error,omitempty"`
}
type ArithmeticOperand struct {
	DatumUUID string `json:"datum_uuid"`
	Value     string `json:"value"`
	Unit      string `json:"unit"`
	Scope     string `json:"scope"`
}
type CalculationRequest struct {
	Schema     string                       `json:"schema"`
	Expression string                       `json:"expression"`
	Operands   map[string]ArithmeticOperand `json:"operands"`
	Precision  int                          `json:"precision"`
	Percent    bool                         `json:"percent"`
	Rounding   string                       `json:"rounding"`
	ResultUnit string                       `json:"result_unit"`
}
type CalculationResult struct {
	Schema    string `json:"schema"`
	Value     string `json:"value"`
	Unit      string `json:"unit"`
	Precision int    `json:"precision"`
	Percent   bool   `json:"percent"`
	Rounding  string `json:"rounding"`
}

// DecodeToolJSON 对原始 wire 字节执行限制和必填校验，避免零值掩盖缺失字段。
func DecodeToolJSON(raw []byte, target any) error {
	if len(raw) > toolJSONLimit {
		return fmt.Errorf("evidence.tool_contract_limit_exceeded")
	}
	if err := utils.DecodeStrictJSON(raw, target); err != nil {
		return err
	}
	var shape any
	if err := json.Unmarshal(raw, &shape); err != nil {
		return err
	}
	return requireFields(shape, reflect.TypeOf(target))
}
