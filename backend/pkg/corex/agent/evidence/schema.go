package evidence

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

// DraftJSONSchema 是模型请求的唯一结构来源；业务字段解释由 Skill locale 定义。
func DraftJSONSchema(sources []string) map[string]any {
	str := func() map[string]any { return map[string]any{"type": "string"} }
	object := func(p map[string]any, required []string) map[string]any {
		return map[string]any{"type": "object", "additionalProperties": false, "properties": p, "required": required}
	}
	array := func(item map[string]any) map[string]any { return map[string]any{"type": "array", "items": item} }
	source := object(map[string]any{"pointer": map[string]any{"type": "string", "enum": sources}, "quote": str(), "literal": str()}, []string{"pointer", "quote", "literal"})
	datum := object(map[string]any{"key": str(), "label": str(), "unit": str(), "scope": str(), "kind": map[string]any{"enum": []string{"quantity", "reported"}}, "source": source}, []string{"key", "label", "unit", "scope", "kind", "source"})
	calculation := object(map[string]any{"key": str(), "label": str(), "expression": str(), "bindings": map[string]any{"type": "object", "additionalProperties": str()}, "precision": map[string]any{"type": "integer", "minimum": 0, "maximum": 12}, "percent": map[string]any{"type": "boolean"}, "compare_to": str()}, []string{"key", "label", "expression", "bindings", "precision", "percent", "compare_to"})
	for _, item := range []map[string]any{datum, calculation} {
		item["properties"].(map[string]any)["key"] = map[string]any{"type": "string", "pattern": "^[a-z][a-z0-9_]{0,63}$"}
	}
	// Business/numeric validation remains server-side; the provider schema defines JSON structure.
	narrative := str()
	return object(map[string]any{"schema": map[string]any{"const": DraftSchema}, "kind": str(), "data": array(datum), "calculations": array(calculation), "hypotheses": array(narrative), "gaps": array(narrative), "actions": array(narrative)}, []string{"schema", "kind", "data", "calculations", "hypotheses", "gaps", "actions"})
}

// ValidateReport 校验持久化结构；运行时还必须调用 Verify 对照本次实际执行记录。
func ValidateReport(value any) (map[string]any, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var report Report
	if err = Decode(raw, &report); err != nil {
		return nil, err
	}
	if report.Schema != ReportSchema || report.Kind == "" || report.SourceDigest == "" || report.Presentation.Reported == nil || report.Presentation.Computed == nil || report.Presentation.Conflicts == nil || report.Presentation.Hypotheses == nil || report.Presentation.Gaps == nil || report.Presentation.Actions == nil {
		return nil, fmt.Errorf("agent.response_contract_invalid")
	}
	if report.Outcome != "completed" && report.Outcome != "needs_action" && report.Outcome != "blocked" && report.Outcome != "failed" {
		return nil, fmt.Errorf("agent.response_contract_invalid")
	}
	if report.Outcome == "completed" && (len(report.Presentation.Conflicts) > 0 || len(report.Presentation.Gaps) > 0 || len(report.Presentation.Hypotheses) > 0) {
		return nil, fmt.Errorf("agent.response_contract_invalid")
	}
	if len(report.Presentation.Reported) == 0 && len(report.Presentation.Computed) == 0 && len(report.Presentation.Gaps) == 0 {
		return nil, fmt.Errorf("evidence.empty_report")
	}
	if _, err := uuid.Parse(report.TenantUUID); err != nil {
		return nil, fmt.Errorf("evidence.tenant_uuid_required")
	}
	if _, err := uuid.Parse(report.RevisionUUID); err != nil {
		return nil, fmt.Errorf("evidence.revision_uuid_required")
	}
	if report.TraceID == "" {
		return nil, fmt.Errorf("evidence.trace_required")
	}
	for _, r := range report.Presentation.Computed {
		if r.ToolKey != CalculatorKey || r.ToolVersion != CalculatorVersion || r.SourceDigest != report.SourceDigest || r.TenantUUID != report.TenantUUID || r.RevisionUUID != report.RevisionUUID || r.TraceID != report.TraceID || r.EndedAt.Before(r.StartedAt) || r.StartedAt.IsZero() {
			return nil, fmt.Errorf("agent.response_contract_invalid")
		}
		if _, err := uuid.Parse(r.UUID); err != nil {
			return nil, fmt.Errorf("agent.response_contract_invalid")
		}
		bindings := map[string]string{}
		for k, d := range r.Operands {
			bindings[k] = d.Value
		}
		result, err := Calculate(context.Background(), r.Request.Expression, bindings, r.Request.Precision, r.Request.Percent)
		if err != nil || result != r.DisplayValue {
			return nil, fmt.Errorf("agent.response_contract_invalid")
		}
	}
	var out map[string]any
	err = json.Unmarshal(raw, &out)
	return out, err
}
