package evidence

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// NotesContext 是说明阶段的显式投影，不传原文、操作数、算式或展示值。
// 数值仍完整保存在 prepared 中，最终报告不经过该投影，也不删除任何证据。
func NotesContext(prepared map[string]any, missing []MissingOperands) (map[string]any, error) {
	raw, err := json.Marshal(prepared)
	if err != nil {
		return nil, err
	}
	var report Report
	if err := Decode(raw, &report); err != nil {
		return nil, err
	}
	inputs, calculations, conflicts := []map[string]string{}, []map[string]string{}, []map[string]string{}
	labels := map[string]string{}
	for _, d := range report.Presentation.Reported {
		labels[d.Key] = d.Label
		inputs = append(inputs, map[string]string{"key": d.Key, "label": d.Label, "kind": d.Kind})
	}
	for _, c := range report.Presentation.Computed {
		labels[c.Request.Key] = c.Request.Label
		calculations = append(calculations, map[string]string{"key": c.Request.Key, "label": c.Request.Label})
	}
	for _, c := range report.Presentation.Conflicts {
		conflicts = append(conflicts, map[string]string{"calculation_label": labels[c.CalculationKey], "reported_label": labels[c.ReportedKey]})
	}
	checks := []map[string]any{}
	for _, c := range report.Presentation.Computed {
		operands := []string{}
		for _, operand := range c.Operands {
			operands = append(operands, operand.Label)
		}
		sort.Strings(operands)
		status := "not_compared"
		if c.Request.CompareTo != "" {
			status = "matched"
			for _, conflict := range report.Presentation.Conflicts {
				if conflict.CalculationKey == c.Request.Key {
					status = "different_under_declared_formula"
				}
			}
		}
		checks = append(checks, map[string]any{"label": c.Request.Label, "available_operands": operands, "comparison_status": status, "reported_label": labels[c.Request.CompareTo]})
	}
	return map[string]any{"source_claims_independently_verified": false, "available_inputs": inputs, "executed_calculations": calculations, "calculation_checks": checks, "conflicts": conflicts, "missing_inputs": missing}, nil
}

// AddNotesSourceContext 保留原文业务背景和显式传入的上游意见，同时屏蔽
// 数字；上游意见不成为数值来源，也不能覆盖计算结果或缺失操作数。
func AddNotesSourceContext(projection map[string]any, payload map[string]any, sources []string) error {
	context := []map[string]any{}
	total := 0
	appendText := func(ref, text, kind string) error {
		total += len(text)
		if total > 128*1024 {
			return fmt.Errorf("evidence.notes_context_limit_exceeded")
		}
		if strings.TrimSpace(text) != "" {
			context = append(context, map[string]any{"reference": ref, "kind": kind, "independently_verified": false, "text": sourceNumberPattern.ReplaceAllString(text, "〔原文数值〕")})
		}
		return nil
	}
	for _, ref := range sources {
		value, err := pointer(payload, ref)
		if err != nil {
			return err
		}
		text, ok := value.(string)
		if !ok {
			return fmt.Errorf("evidence.source_text_required")
		}
		if err := appendText(ref, text, "original_source"); err != nil {
			return err
		}
	}
	keys := []string{}
	for key := range payload {
		if strings.HasPrefix(key, "upstream_") {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		if err := appendText(key, notesAnalysisText(payload[key], 0), "upstream_opinion"); err != nil {
			return err
		}
	}
	projection["source_context"] = context
	return nil
}

func notesAnalysisText(value any, depth int) string {
	if depth > 8 {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	if object, ok := value.(map[string]any); ok {
		for _, key := range []string{"content", "text", "result", "data"} {
			if text := notesAnalysisText(object[key], depth+1); text != "" {
				return text
			}
		}
	}
	return ""
}
