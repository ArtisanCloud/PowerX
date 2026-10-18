package evidence

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/utils"
	"github.com/google/uuid"
)

// ToolRunIdentity 由运行入口提供；不从模型 payload/context map 中恢复身份。
type ToolRunIdentity struct{ TenantUUID, RevisionUUID, RunID, TraceID, AgentUUID string }

// SourceField 来自已发布策略或 typed resolver，不接受模型授予单位、口径或可信状态。
// Unit="1" 表示无量纲；百分比保持 "%" 或 "％"，只能作为 reported。
type SourceField struct {
	FieldKey, LabelKey, DatumKind, Unit, Scope string
	// NumericString 仅供 typed resolver 的十进制字符串，不表示抽取自然语言已经核实。
	NumericString bool
}

// SourceLocation 只选择现有来源，不接收 value/input_state、快照或授权决定。
type SourceLocation struct {
	Kind, ObjectType, ObjectUUID, ExecutionUUID, Pointer, Quote, Literal string
	Range                                                                ByteRange
}

type ObjectSourceRequest struct{ TenantUUID, ObjectType, ObjectUUID, Pointer string }

// AuthorizedSourceSnapshot 必须已完成对象和所请求字段的授权，且不含未授权字段。
// Values 是受控 DTO 的 JSON；禁止任意 URL/文件/SQL resolver。
type AuthorizedSourceSnapshot struct {
	TenantUUID, ObjectType, ObjectUUID string
	Values                             json.RawMessage
	Fields                             map[string]SourceField
}

// AuthorizedObjectSource 只由 DI 按固定 object_type 登记。每次调用（包括再次验证）
// 都必须重查实时主体、对象和字段权限；所有不存在/无权限均由本层统一脱敏。
type AuthorizedObjectSource interface {
	ReadAuthorizedSource(context.Context, ObjectSourceRequest) (*AuthorizedSourceSnapshot, error)
}

type sourceSnapshot struct {
	uuid, digest string
	raw          []byte
	fields       map[string]SourceField
}

// ToolRun 持有同次运行私有来源、请求、输出及回执字节；没有导入 provider/历史回执的入口。
// 持久化和历史展示由上层负责；丢失本 run 对象后不能通过相同 UUID 重建信任。
type ToolRun struct {
	identity     ToolRunIdentity
	service      *ToolService
	inputUUID    string
	mu           sync.Mutex
	snapshots    map[string]sourceSnapshot
	datums       map[string]ResolvedDatum
	receipts     map[string][]byte
	rechecks     map[string]func(context.Context) error
	artifacts    map[string][]byte
	outputFields map[string]map[string]SourceField
	policy       ToolRunPolicy
	principal    toolPrincipal
	bytesStored  int
	calls        int
}

type toolPrincipal struct {
	tenant, user, subject, apiKey string
	member                        uint64
	agent                         uint64
	root                          bool
}

func toolContextPrincipal(ctx context.Context) toolPrincipal {
	return toolPrincipal{tenant: reqctx.GetTenantUUID(ctx), user: reqctx.GetUserUUID(ctx), subject: reqctx.GetSubject(ctx), apiKey: reqctx.AuthenticatedAPIKeyHash(ctx), member: reqctx.GetMemberID(ctx), agent: ToolAgentID(ctx), root: reqctx.IsRoot(ctx)}
}

func canonicalUUID(value string) bool {
	id, err := uuid.Parse(value)
	return err == nil && id != uuid.Nil && id.String() == value
}
func boundedText(value string, max int) bool {
	return value != "" && utf8.ValidString(value) && utf8.RuneCountInString(value) <= max
}
func snapshotKey(kind, objectType, objectUUID string) string {
	return kind + "\x00" + objectType + "\x00" + objectUUID
}

// FreezeEvidenceInput 在任何 LLM/adapter 回调之前冻结 JSON；保留 json.Number 精度。
func FreezeEvidenceInput(input map[string]any) (map[string]any, error) {
	if !validInputStrings(reflect.ValueOf(input), 0) {
		return nil, toolError("evidence.source_invalid", "source_resolution", nil)
	}
	raw, err := json.Marshal(input)
	if err != nil || len(raw) > toolJSONLimit {
		return nil, toolError("evidence.source_invalid", "source_resolution", err)
	}
	var result map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&result); err != nil || result == nil {
		return nil, toolError("evidence.source_invalid", "source_resolution", err)
	}
	return result, nil
}

func validInputStrings(value reflect.Value, depth int) bool {
	if depth > 128 {
		return false
	}
	if !value.IsValid() {
		return true
	}
	switch value.Kind() {
	case reflect.String:
		return utf8.ValidString(value.String())
	case reflect.Interface, reflect.Pointer:
		if value.IsNil() {
			return true
		}
		return validInputStrings(value.Elem(), depth+1)
	case reflect.Map:
		iterator := value.MapRange()
		for iterator.Next() {
			if !validInputStrings(iterator.Key(), depth+1) || !validInputStrings(iterator.Value(), depth+1) {
				return false
			}
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < value.Len(); i++ {
			if !validInputStrings(value.Index(i), depth+1) {
				return false
			}
		}
	case reflect.Struct:
		for i := 0; i < value.NumField(); i++ {
			if value.Type().Field(i).IsExported() && !validInputStrings(value.Field(i), depth+1) {
				return false
			}
		}
	}
	return true
}

func (r *ToolRun) Identity() ToolRunIdentity { return r.identity }
func (r *ToolRun) InputObjectUUID() string   { return r.inputUUID }

func (r *ToolRun) checkContext(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	// 内置纯计算不要求登录；一旦有受信任租户，就必须与本 run 相同。
	if tenant := reqctx.GetTenantUUID(ctx); tenant != "" && tenant != r.identity.TenantUUID {
		return toolError("evidence.source_access_denied", "source_resolution", nil)
	}
	if toolContextPrincipal(ctx) != r.principal {
		return toolError("evidence.source_access_denied", "source_resolution", nil)
	}
	return nil
}

func (r *ToolRun) authorized(ctx context.Context, location SourceLocation) (*AuthorizedSourceSnapshot, error) {
	deny := func(cause error) (*AuthorizedSourceSnapshot, error) {
		return nil, toolError("evidence.source_access_denied", "source_resolution", cause)
	}
	if reqctx.GetTenantUUID(ctx) != r.identity.TenantUUID {
		return deny(nil)
	}
	reader := r.service.objects[location.ObjectType]
	if reader == nil {
		return deny(nil)
	}
	value, err := reader.ReadAuthorizedSource(ctx, ObjectSourceRequest{TenantUUID: r.identity.TenantUUID, ObjectType: location.ObjectType, ObjectUUID: location.ObjectUUID, Pointer: location.Pointer})
	if err != nil || value == nil {
		return deny(err)
	}
	if value.TenantUUID != r.identity.TenantUUID || value.ObjectType != location.ObjectType || value.ObjectUUID != location.ObjectUUID {
		return deny(nil)
	}
	if _, ok := value.Fields[location.Pointer]; !ok {
		return deny(nil)
	}
	return value, nil
}

func (r *ToolRun) source(ctx context.Context, location SourceLocation) (sourceSnapshot, error) {
	deny := func() (sourceSnapshot, error) {
		return sourceSnapshot{}, toolError("evidence.source_access_denied", "source_resolution", nil)
	}
	if err := r.checkContext(ctx); err != nil {
		return sourceSnapshot{}, err
	}
	if !canonicalUUID(location.ObjectUUID) || !boundedText(location.ObjectType, 512) {
		return deny()
	}
	key := snapshotKey(location.Kind, location.ObjectType, location.ObjectUUID)
	var authorized *AuthorizedSourceSnapshot
	if location.Kind == "authorized_object" {
		if location.ExecutionUUID != "" {
			return deny()
		}
		var err error
		authorized, err = r.authorized(ctx, location)
		if err != nil {
			return sourceSnapshot{}, err
		}
	} else if location.Kind == "run_input" {
		if location.ObjectType != "powerx.run_input" || location.ObjectUUID != r.inputUUID || location.ExecutionUUID != "" {
			return deny()
		}
	} else if location.Kind == "tool_output" {
		r.mu.Lock()
		raw := append([]byte(nil), r.receipts[location.ExecutionUUID]...)
		r.mu.Unlock()
		var receipt ToolReceipt
		if location.ObjectType != "powerx.tool_output" || DecodeToolJSON(raw, &receipt) != nil || receipt.Status != "succeeded" || receipt.Result == nil || receipt.Result.ArtifactUUID != location.ObjectUUID {
			return deny()
		}
		if err := r.VerifyReceipt(ctx, receipt); err != nil {
			return sourceSnapshot{}, err
		}
		if err := r.checkExecutionAccess(ctx, receipt); err != nil {
			return sourceSnapshot{}, err
		}
		for _, datum := range receipt.Inputs {
			if err := r.VerifyDatum(ctx, datum); err != nil {
				return sourceSnapshot{}, err
			}
		}
	} else {
		return deny()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.snapshots[key]; ok {
		return existing, nil
	}
	var raw []byte
	fields := map[string]SourceField{}
	if authorized != nil {
		if len(authorized.Values) > toolJSONLimit || len(authorized.Fields) > 512 {
			return sourceSnapshot{}, toolError("evidence.source_invalid", "source_resolution", nil)
		}
		raw = append([]byte(nil), authorized.Values...)
		// 一次 resolver 可只返回一个获准字段；按字段冻结，避免依赖后续读扩大授权范围。
		for pointer, field := range authorized.Fields {
			fields[pointer] = field
		}
	} else if location.Kind == "tool_output" {
		raw = append([]byte(nil), r.artifacts[location.ObjectUUID]...)
		for pointer, field := range r.outputFields[location.ObjectUUID] {
			fields[pointer] = field
		}
	} else {
		return deny()
	}
	var object any
	if len(raw) > toolJSONLimit || utils.DecodeStrictJSON(raw, &object) != nil {
		return sourceSnapshot{}, toolError("evidence.source_invalid", "source_resolution", nil)
	}
	snapshot := sourceSnapshot{uuid: uuid.NewString(), digest: toolDigest(raw), raw: raw, fields: fields}
	if r.bytesStored+len(raw) > maxToolRunBytes {
		return sourceSnapshot{}, toolError("evidence.source_invalid", "source_resolution", nil)
	}
	r.bytesStored += len(raw)
	r.snapshots[key] = snapshot
	return snapshot, nil
}

// ResolveDatum 从冻结来源解析十进制值和可信状态；模型不能提交重复 value 或提升状态。
// field 必须由调用方从发布策略取得；authorized_object 还必须与 resolver 的字段合同一致。
func (r *ToolRun) ResolveDatum(ctx context.Context, location SourceLocation, field SourceField) (ResolvedDatum, error) {
	invalid := func(cause error) (ResolvedDatum, error) {
		return ResolvedDatum{}, toolError("evidence.source_invalid", "source_resolution", cause)
	}
	snapshot, err := r.source(ctx, location)
	if err != nil {
		return ResolvedDatum{}, err
	}
	if !keyPattern.MatchString(field.FieldKey) || !boundedText(field.LabelKey, 512) || !boundedText(field.Unit, 512) || !boundedText(field.Scope, 4096) || (field.DatumKind != "quantity" && field.DatumKind != "reported") {
		return invalid(nil)
	}
	if location.Kind != "run_input" {
		if declared, ok := snapshot.fields[location.Pointer]; !ok || declared != field {
			return invalid(nil)
		}
	} else if declared, ok := r.policy.Fields[field.FieldKey]; !ok || declared != field {
		return invalid(nil)
	}
	value, err := snapshotPointer(snapshot.raw, location.Pointer)
	if err != nil {
		return invalid(err)
	}
	text, isText := value.(string)
	if !isText {
		number, ok := value.(json.Number)
		if !ok {
			return invalid(nil)
		}
		text = number.String()
	}
	span := location.Range
	if span.StartByte < 0 || span.EndByte <= span.StartByte || span.EndByte > len(text) || !utf8.ValidString(text[:span.StartByte]) || !utf8.ValidString(text[:span.EndByte]) || text[span.StartByte:span.EndByte] != location.Quote || !boundedText(location.Quote, 4096) {
		return invalid(nil)
	}
	parsed, err := normalizeSourceLiteral(location.Literal)
	if err != nil {
		return invalid(err)
	}
	structuredString := isText && field.NumericString && location.Kind != "run_input"
	if isText && !structuredString {
		// 按原文数值的完整边界验证，不能截断 27.8% 或把 27.8 视为 278 人。
		found := false
		for _, numeric := range sourceNumberPattern.FindAllStringIndex(text, -1) {
			if numeric[0] < span.StartByte || numeric[1] > span.EndByte || text[numeric[0]:numeric[1]] != location.Literal {
				continue
			}
			tail := text[numeric[1]:]
			trimmed := trimNumericSpace(tail)
			unit := field.Unit
			if unit == "1" {
				unit = ""
			}
			if !strings.HasPrefix(trimmed, unit) {
				continue
			}
			end := numeric[1]
			if unit != "" {
				end += len(tail) - len(trimmed) + len(unit)
			}
			if end > span.EndByte || !numericUnitBoundary(text, numeric[0], end, unit) {
				continue
			}
			found = true
			break
		}
		if !found {
			return invalid(nil)
		}
	} else if location.Literal != text || location.Quote != text || span.StartByte != 0 || span.EndByte != len(text) {
		return invalid(nil)
	}
	if field.DatumKind == "quantity" && (field.Unit == "%" || field.Unit == "％") {
		return invalid(nil)
	}
	state := "user_supplied_unverified"
	if isText {
		state = "extracted_unverified"
	}
	if location.Kind == "authorized_object" && (!isText || structuredString) {
		state = "authorized_record"
	}
	if location.Kind == "tool_output" {
		state = "tool_result"
	}
	datum := ResolvedDatum{DatumUUID: uuid.NewString(), FieldKey: field.FieldKey, LabelKey: field.LabelKey, DatumKind: field.DatumKind, Literal: location.Literal, Value: parsed, Unit: field.Unit, Scope: field.Scope, InputState: state,
		Source: DatumSource{SourceUUID: uuid.NewString(), Kind: location.Kind, ObjectType: location.ObjectType, ObjectUUID: location.ObjectUUID, SnapshotUUID: snapshot.uuid, SnapshotSHA256: snapshot.digest, Pointer: location.Pointer, Quote: location.Quote, Range: span, ExecutionUUID: location.ExecutionUUID}}
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.datums) >= 512 {
		return invalid(nil)
	}
	r.datums[datum.DatumUUID] = datum
	return datum, nil
}

// 原词面值保留正号/千分位/前导零；运行和历史结构校验共用同一 value 规范。
func normalizeSourceLiteral(literal string) (string, error) {
	if !literalPattern.MatchString(literal) {
		return "", fmt.Errorf("evidence.source_invalid")
	}
	parsed := strings.ReplaceAll(literal, ",", "")
	if _, err := decimal(parsed); err != nil {
		return "", err
	}
	negative := strings.HasPrefix(parsed, "-")
	unsigned := strings.TrimPrefix(strings.TrimPrefix(parsed, "+"), "-")
	parts := strings.SplitN(unsigned, ".", 2)
	parts[0] = strings.TrimLeft(parts[0], "0")
	if parts[0] == "" {
		parts[0] = "0"
	}
	parsed = strings.Join(parts, ".")
	if negative && strings.Trim(parsed, "0.") != "" {
		parsed = "-" + parsed
	}
	return parsed, nil
}

// VerifyDatum 验证平台创建的完整记录，并重新检查受控对象的当前权限。
func (r *ToolRun) VerifyDatum(ctx context.Context, datum ResolvedDatum) error {
	return r.verifyDatum(ctx, datum, 0)
}
func (r *ToolRun) verifyDatum(ctx context.Context, datum ResolvedDatum, depth int) error {
	if err := r.checkContext(ctx); err != nil {
		return err
	}
	if depth > 64 {
		return toolError("evidence.source_invalid", "source_resolution", nil)
	}
	r.mu.Lock()
	original, ok := r.datums[datum.DatumUUID]
	r.mu.Unlock()
	if !ok {
		return toolError("evidence.source_access_denied", "source_resolution", nil)
	}
	if original != datum {
		return toolError("evidence.source_invalid", "source_resolution", nil)
	}
	r.mu.Lock()
	snapshot, exists := r.snapshots[snapshotKey(datum.Source.Kind, datum.Source.ObjectType, datum.Source.ObjectUUID)]
	r.mu.Unlock()
	if !exists || snapshot.uuid != datum.Source.SnapshotUUID || snapshot.digest != datum.Source.SnapshotSHA256 || toolDigest(snapshot.raw) != snapshot.digest {
		return toolError("evidence.source_invalid", "source_resolution", nil)
	}
	if datum.Source.Kind == "authorized_object" {
		_, err := r.authorized(ctx, SourceLocation{ObjectType: datum.Source.ObjectType, ObjectUUID: datum.Source.ObjectUUID, Pointer: datum.Source.Pointer})
		return err
	}
	if datum.Source.Kind == "tool_output" {
		r.mu.Lock()
		raw := append([]byte(nil), r.receipts[datum.Source.ExecutionUUID]...)
		r.mu.Unlock()
		var receipt ToolReceipt
		if DecodeToolJSON(raw, &receipt) != nil || receipt.Status != "succeeded" {
			return toolError("evidence.tool_receipt_untrusted", "evidence_validation", nil)
		}
		if err := r.VerifyReceipt(ctx, receipt); err != nil {
			return err
		}
		if err := r.checkExecutionAccess(ctx, receipt); err != nil {
			return err
		}
		for _, input := range receipt.Inputs {
			if err := r.verifyDatum(ctx, input, depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}

// snapshotPointer 严格实现 RFC 6901；支持数组，拒绝非法转义和前导零下标。
func snapshotPointer(raw []byte, pointer string) (any, error) {
	if len(pointer) > 1024 || (pointer != "" && !strings.HasPrefix(pointer, "/")) {
		return nil, fmt.Errorf("pointer.invalid")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	if pointer == "" {
		return value, nil
	}
	for _, segment := range strings.Split(pointer[1:], "/") {
		for i := 0; i < len(segment); i++ {
			if segment[i] == '~' {
				if i+1 >= len(segment) || (segment[i+1] != '0' && segment[i+1] != '1') {
					return nil, fmt.Errorf("pointer.invalid")
				}
				i++
			}
		}
		segment = strings.ReplaceAll(strings.ReplaceAll(segment, "~1", "/"), "~0", "~")
		switch current := value.(type) {
		case map[string]any:
			var ok bool
			value, ok = current[segment]
			if !ok {
				return nil, fmt.Errorf("pointer.missing")
			}
		case []any:
			index, err := strconv.Atoi(segment)
			if err != nil || index < 0 || index >= len(current) || strconv.Itoa(index) != segment {
				return nil, fmt.Errorf("pointer.invalid")
			}
			value = current[index]
		default:
			return nil, fmt.Errorf("pointer.scalar")
		}
	}
	return value, nil
}
