package evidence

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math/big"
	"reflect"
	"sort"
	"strings"
)

const ReportSchemaV5 = "powerx.agent.response/v5"

type ExpressionDerivation struct {
	Kind            string            `json:"kind"`
	Expression      string            `json:"expression"`
	OperandBindings map[string]string `json:"operand_bindings"`
	Precision       int               `json:"precision"`
	Percent         bool              `json:"percent"`
	Rounding        string            `json:"rounding"`
}
type AlgorithmDerivation struct {
	Kind             string `json:"kind"`
	AlgorithmID      string `json:"algorithm_id"`
	ParametersSHA256 string `json:"parameters_sha256"`
}

// Derivation 保留两种关闭字段集合；不能用 expression 冒充任意领域算法。
type Derivation struct {
	Expression *ExpressionDerivation
	Algorithm  *AlgorithmDerivation
}

func (d Derivation) MarshalJSON() ([]byte, error) {
	if d.Expression != nil && d.Algorithm == nil {
		return json.Marshal(d.Expression)
	}
	if d.Algorithm != nil && d.Expression == nil {
		return json.Marshal(d.Algorithm)
	}
	return nil, fmt.Errorf("agent.response_contract_invalid")
}
func (d *Derivation) UnmarshalJSON(raw []byte) error {
	var tag struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(raw, &tag); err != nil {
		return err
	}
	*d = Derivation{}
	if tag.Kind == "expression" {
		var value ExpressionDerivation
		if err := DecodeToolJSON(raw, &value); err != nil {
			return err
		}
		d.Expression = &value
		return nil
	}
	if tag.Kind == "domain_algorithm" {
		var value AlgorithmDerivation
		if err := DecodeToolJSON(raw, &value); err != nil {
			return err
		}
		d.Algorithm = &value
		return nil
	}
	return fmt.Errorf("agent.response_contract_invalid")
}

type ComputedDatum struct {
	DatumUUID     string     `json:"datum_uuid"`
	FieldKey      string     `json:"field_key"`
	LabelKey      string     `json:"label_key"`
	ReceiptUUID   string     `json:"receipt_uuid"`
	OutputPointer string     `json:"output_pointer"`
	Value         string     `json:"value"`
	Unit          string     `json:"unit"`
	Scope         string     `json:"scope"`
	Derivation    Derivation `json:"derivation"`
}
type EvidenceConflict struct {
	ConflictUUID      string `json:"conflict_uuid"`
	ReportedDatumUUID string `json:"reported_datum_uuid"`
	ComputedDatumUUID string `json:"computed_datum_uuid"`
	Reason            string `json:"reason"`
}
type EvidenceGap struct {
	Key           string   `json:"key"`
	LabelKey      string   `json:"label_key"`
	MissingFields []string `json:"missing_fields"`
}
type PresentationV5 struct {
	Reported   []ResolvedDatum    `json:"reported"`
	Computed   []ComputedDatum    `json:"computed"`
	Conflicts  []EvidenceConflict `json:"conflicts"`
	Gaps       []EvidenceGap      `json:"gaps"`
	Hypotheses []string           `json:"hypotheses"`
	Actions    []string           `json:"actions"`
}
type ReportV5 struct {
	Schema       string         `json:"schema"`
	Kind         string         `json:"kind"`
	Outcome      string         `json:"outcome"`
	TenantUUID   string         `json:"tenant_uuid"`
	RevisionUUID string         `json:"revision_uuid"`
	RunID        string         `json:"run_id"`
	TraceID      string         `json:"trace_id"`
	SourceDigest string         `json:"source_digest"`
	Receipts     []ToolReceipt  `json:"receipts"`
	Presentation PresentationV5 `json:"presentation"`
}

// ResponseLabels 是发布 Revision 的 locale 快照，随消息 metadata 保存，不改变 V5 wire。
type ResponseLabels struct {
	Schema       string                       `json:"schema"`
	RevisionUUID string                       `json:"revision_uuid"`
	Locales      map[string]map[string]string `json:"locales"`
}

func (r *ToolRun) sourceDigest() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	keys := make([]string, 0, len(r.snapshots))
	for key := range r.snapshots {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	refs := []string{}
	for _, key := range keys {
		snapshot := r.snapshots[key]
		refs = append(refs, snapshot.uuid+":"+snapshot.digest)
	}
	raw, _ := json.Marshal(refs)
	return toolDigest(raw)
}

// SealReportV5 对来源、实时权限、真实结果及 derivation 验真后登记到本次 Runtime ledger。
func (r *ToolRun) SealReportV5(ctx context.Context, report ReportV5) (map[string]any, error) {
	if report.TenantUUID != r.identity.TenantUUID || report.RevisionUUID != r.identity.RevisionUUID || report.RunID != r.identity.RunID || report.TraceID != r.identity.TraceID {
		return nil, toolError("evidence.tool_receipt_untrusted", "evidence_validation", nil)
	}
	report.Schema = ReportSchemaV5
	report.SourceDigest = r.sourceDigest()
	report.Outcome = "completed"
	p := report.Presentation
	if len(p.Conflicts)+len(p.Gaps)+len(p.Hypotheses) > 0 {
		report.Outcome = "needs_action"
	}
	for _, datum := range p.Reported {
		if err := r.VerifyDatum(ctx, datum); err != nil {
			return nil, err
		}
	}
	receipts := map[string]ToolReceipt{}
	for _, receipt := range report.Receipts {
		if err := r.VerifyReceipt(ctx, receipt); err != nil {
			return nil, err
		}
		receipts[receipt.ExecutionUUID] = receipt
	}
	for _, computed := range p.Computed {
		receipt, ok := receipts[computed.ReceiptUUID]
		if !ok {
			return nil, toolError("evidence.tool_receipt_untrusted", "evidence_validation", nil)
		}
		if err := r.VerifyResultReference(ctx, receipt, ToolResultReference{Pointer: computed.OutputPointer, Value: computed.Value, Unit: computed.Unit, Scope: computed.Scope}); err != nil {
			return nil, err
		}
		if err := r.verifyDerivation(ctx, receipt, computed.Derivation); err != nil {
			return nil, err
		}
	}
	for _, list := range [][]string{p.Hypotheses, p.Actions} {
		for _, text := range list {
			if strings.ContainsAny(text, "0123456789") {
				return nil, toolError("agent.response_contract_invalid", "final_validation", nil)
			}
		}
	}
	out, err := ValidateReportV5(report)
	if err != nil {
		return nil, err
	}
	ledger, ok := ctx.Value(ledgerKey{}).(*Ledger)
	if !ok {
		return nil, toolError("evidence.tool_receipt_untrusted", "evidence_validation", nil)
	}
	raw, _ := json.Marshal(out)
	digest := fmt.Sprintf("%x", sha256.Sum256(raw))
	ledger.mu.Lock()
	ledger.reports[digest] = raw
	ledger.mu.Unlock()
	return out, nil
}

func (r *ToolRun) verifyDerivation(ctx context.Context, receipt ToolReceipt, d Derivation) error {
	fail := func() error { return toolError("evidence.tool_result_invalid", "evidence_validation", nil) }
	if d.Algorithm != nil {
		if receipt.Tool.Binding.Kind != "capability" || d.Algorithm.AlgorithmID != receipt.Tool.ToolKey || d.Algorithm.ParametersSHA256 != receipt.Request.SHA256 {
			return fail()
		}
		return nil
	}
	if d.Expression == nil || receipt.Tool.ToolKey != CalculatorKey || receipt.Tool.Version != TypedCalculatorVersion {
		return fail()
	}
	raw, err := r.ReadArtifact(ctx, receipt.Request)
	if err != nil {
		return err
	}
	var request CalculationRequest
	if DecodeToolJSON(raw, &request) != nil {
		return fail()
	}
	want := ExpressionDerivation{Kind: "expression", Expression: request.Expression, OperandBindings: map[string]string{}, Precision: request.Precision, Percent: request.Percent, Rounding: request.Rounding}
	for variable, operand := range request.Operands {
		want.OperandBindings[variable] = operand.DatumUUID
	}
	if !reflect.DeepEqual(want, *d.Expression) {
		return fail()
	}
	return nil
}

// ValidateReportV5 只校验历史/传输结构及关联，不执行工具、不访问当前业务对象、不恢复 ledger。
func ValidateReportV5(value any) (map[string]any, error) {
	fail := func() (map[string]any, error) {
		return nil, toolError("agent.response_contract_invalid", "final_validation", nil)
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return fail()
	}
	var report ReportV5
	if DecodeToolJSON(raw, &report) != nil {
		return fail()
	}
	if report.Schema != ReportSchemaV5 || !boundedText(report.Kind, 512) || !canonicalUUID(report.TenantUUID) || !canonicalUUID(report.RevisionUUID) || !boundedText(report.RunID, 512) || !boundedText(report.TraceID, 512) || !toolDigestPattern.MatchString(report.SourceDigest) || (report.Outcome != "completed" && report.Outcome != "needs_action") {
		return fail()
	}
	p := report.Presentation
	if len(report.Receipts) > 64 || len(p.Reported) > 512 || len(p.Computed) > 64 || len(p.Conflicts) > 64 || len(p.Gaps) > 64 || len(p.Hypotheses) > 6 || len(p.Actions) > 6 || len(p.Reported)+len(p.Computed)+len(p.Gaps) == 0 {
		return fail()
	}
	if report.Outcome == "completed" && len(p.Conflicts)+len(p.Gaps)+len(p.Hypotheses) > 0 {
		return fail()
	}
	seen := map[string]bool{}
	reported := map[string]ResolvedDatum{}
	computed := map[string]ComputedDatum{}
	receipts := map[string]ToolReceipt{}
	for _, receipt := range report.Receipts {
		if !canonicalUUID(receipt.ExecutionUUID) || seen[receipt.ExecutionUUID] || receipt.Schema != ToolReceiptSchema || receipt.TenantUUID != report.TenantUUID || receipt.RevisionUUID != report.RevisionUUID || receipt.RunID != report.RunID || receipt.TraceID != report.TraceID || receipt.StartedAt.IsZero() || receipt.EndedAt.Before(receipt.StartedAt) || !ValidToolDependency(receipt.Tool) || receipt.Attempt < 1 || !boundedText(receipt.NodeID, 512) || !canonicalUUID(receipt.Authorization.SubjectUUID) || !canonicalUUID(receipt.Authorization.DecisionUUID) || receipt.Authorization.Actor != receipt.Tool.Permission.Actor || receipt.InvocationBinding.Kind != receipt.Tool.Binding.Kind || !validArtifact(receipt.Request) {
			return fail()
		}
		// 业务报告不能携带系统失败回执；失败保存在独立归档和失败节点。
		if receipt.Status != "succeeded" {
			return fail()
		}
		if receipt.Result == nil || receipt.Error != nil || !validArtifact(*receipt.Result) {
			return fail()
		}
		if len(receipt.Inputs) > 512 {
			return fail()
		}
		if receipt.InvocationBinding.Kind == "capability" && (receipt.InvocationBinding.RegistrationVersion == 0 || !boundedText(receipt.InvocationBinding.AdapterID, 512) || !toolDigestPattern.MatchString(receipt.InvocationBinding.CatalogSHA256)) {
			return fail()
		}
		for _, datum := range receipt.Inputs {
			if !validResolvedDatum(datum) {
				return fail()
			}
		}
		seen[receipt.ExecutionUUID] = true
		receipts[receipt.ExecutionUUID] = receipt
	}
	validOutputSource := func(datum ResolvedDatum) bool {
		if datum.Source.Kind != "tool_output" {
			return true
		}
		receipt, ok := receipts[datum.Source.ExecutionUUID]
		return ok && receipt.Status == "succeeded" && receipt.Result != nil && receipt.Result.ArtifactUUID == datum.Source.ObjectUUID && receipt.Result.SHA256 == datum.Source.SnapshotSHA256
	}
	for _, receipt := range report.Receipts {
		for _, datum := range receipt.Inputs {
			if !validOutputSource(datum) {
				return fail()
			}
		}
	}
	for _, datum := range p.Reported {
		if !validOutputSource(datum) || !validResolvedDatum(datum) || seen[datum.DatumUUID] {
			return fail()
		}
		seen[datum.DatumUUID] = true
		reported[datum.DatumUUID] = datum
	}
	for _, datum := range p.Computed {
		receipt, ok := receipts[datum.ReceiptUUID]
		if !ok || receipt.Status != "succeeded" || !canonicalUUID(datum.DatumUUID) || seen[datum.DatumUUID] || !keyPattern.MatchString(datum.FieldKey) || !boundedText(datum.LabelKey, 512) || !boundedText(datum.Unit, 512) || !boundedText(datum.Scope, 4096) || !validJSONPointer(datum.OutputPointer) || len(datum.Value) > 1280 || !toolFiniteDecimal.MatchString(datum.Value) {
			return fail()
		}
		if d := datum.Derivation.Expression; d != nil {
			bindings := map[string]string{}
			for variable, id := range d.OperandBindings {
				if !keyPattern.MatchString(variable) || !canonicalUUID(id) {
					return fail()
				}
				found := false
				for _, input := range receipt.Inputs {
					if input.DatumUUID == id && input.DatumKind == "quantity" {
						found = true
					}
				}
				if !found {
					return fail()
				}
				bindings[variable] = id
			}
			if receipt.Tool.Binding.Kind != "builtin" || receipt.Tool.ToolKey != CalculatorKey || receipt.Tool.Version != TypedCalculatorVersion || d.Kind != "expression" || len(bindings) == 0 || len(bindings) > 32 || validateBoundExpression(d.Expression, bindings) != nil || d.Precision < 0 || d.Precision > 12 || d.Rounding != HalfAwayFromZero || (d.Percent != (datum.Unit == "%")) {
				return fail()
			}
		} else if d := datum.Derivation.Algorithm; d == nil || d.Kind != "domain_algorithm" || !boundedText(d.AlgorithmID, 512) || !toolDigestPattern.MatchString(d.ParametersSHA256) || receipt.Tool.Binding.Kind != "capability" || d.AlgorithmID != receipt.Tool.ToolKey || d.ParametersSHA256 != receipt.Request.SHA256 {
			return fail()
		}
		seen[datum.DatumUUID] = true
		computed[datum.DatumUUID] = datum
	}
	for _, conflict := range p.Conflicts {
		a, aok := reported[conflict.ReportedDatumUUID]
		b, bok := computed[conflict.ComputedDatumUUID]
		if !canonicalUUID(conflict.ConflictUUID) || seen[conflict.ConflictUUID] || !aok || !bok {
			return fail()
		}
		if reason := conflictReason(a, b); reason == "" || conflict.Reason != reason {
			return fail()
		}
		seen[conflict.ConflictUUID] = true
	}
	for _, gap := range p.Gaps {
		if !keyPattern.MatchString(gap.Key) || !boundedText(gap.LabelKey, 512) || len(gap.MissingFields) == 0 || len(gap.MissingFields) > 32 {
			return fail()
		}
		keys := map[string]bool{}
		for _, field := range gap.MissingFields {
			if !keyPattern.MatchString(field) || keys[field] {
				return fail()
			}
			keys[field] = true
		}
	}
	for _, list := range [][]string{p.Hypotheses, p.Actions} {
		for _, text := range list {
			if !boundedText(text, 4096) {
				return fail()
			}
		}
	}
	var out map[string]any
	err = json.Unmarshal(raw, &out)
	return out, err
}

func validArtifact(ref ToolArtifact) bool {
	return canonicalUUID(ref.ArtifactUUID) && toolDigestPattern.MatchString(ref.SHA256)
}
func validFailureStage(stage string) bool {
	switch stage {
	case "dependency_check", "permission_check", "source_resolution", "input_validation", "tool_invoke", "result_validation", "evidence_validation", "final_validation":
		return true
	}
	return false
}
func validJSONPointer(pointer string) bool {
	if len(pointer) > 1024 || (pointer != "" && !strings.HasPrefix(pointer, "/")) {
		return false
	}
	for i := 0; i < len(pointer); i++ {
		if pointer[i] == '~' {
			if i+1 >= len(pointer) || (pointer[i+1] != '0' && pointer[i+1] != '1') {
				return false
			}
			i++
		}
	}
	return true
}
func validResolvedDatum(d ResolvedDatum) bool {
	parsed, err := normalizeSourceLiteral(d.Literal)
	if err != nil || parsed != d.Value {
		return false
	}
	s := d.Source
	if !canonicalUUID(d.DatumUUID) || !keyPattern.MatchString(d.FieldKey) || !boundedText(d.LabelKey, 512) || !boundedText(d.Unit, 512) || !boundedText(d.Scope, 4096) || !boundedText(d.Literal, 4096) || len(d.Value) > 80 || !toolFiniteDecimal.MatchString(d.Value) || (d.DatumKind != "quantity" && d.DatumKind != "reported") || !canonicalUUID(s.SourceUUID) || !canonicalUUID(s.ObjectUUID) || !canonicalUUID(s.SnapshotUUID) || !boundedText(s.ObjectType, 512) || !toolDigestPattern.MatchString(s.SnapshotSHA256) || !validJSONPointer(s.Pointer) || !boundedText(s.Quote, 4096) || s.Range.StartByte < 0 || s.Range.EndByte <= s.Range.StartByte || s.Range.EndByte-s.Range.StartByte != len(s.Quote) {
		return false
	}
	if d.DatumKind == "quantity" && (d.Unit == "%" || d.Unit == "％") {
		return false
	}
	switch s.Kind {
	case "run_input":
		return s.ObjectType == "powerx.run_input" && s.ExecutionUUID == "" && (d.InputState == "user_supplied_unverified" || d.InputState == "extracted_unverified")
	case "authorized_object":
		return s.ExecutionUUID == "" && (d.InputState == "authorized_record" || d.InputState == "extracted_unverified")
	case "tool_output":
		return s.ObjectType == "powerx.tool_output" && canonicalUUID(s.ExecutionUUID) && d.InputState == "tool_result"
	}
	return false
}

func conflictReason(a ResolvedDatum, b ComputedDatum) string {
	if a.Scope != b.Scope {
		return "scope_mismatch"
	}
	if a.Unit != b.Unit {
		return "unit_mismatch"
	}
	// 只对已经产生的十进制值作精确比较；历史校验不会重新调用工具。
	av, aok := new(big.Rat).SetString(a.Value)
	bv, bok := new(big.Rat).SetString(b.Value)
	if !aok || !bok || av.Cmp(bv) != 0 {
		return "value_mismatch"
	}
	return ""
}

// CompareEvidenceValues 仅比较同类证据值及单位/口径，不重算工具或推断业务事实。
func CompareEvidenceValues(a ResolvedDatum, b ComputedDatum) string { return conflictReason(a, b) }
