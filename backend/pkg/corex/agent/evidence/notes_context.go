package evidence

import "encoding/json"

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
	return map[string]any{"source_claims_independently_verified": false, "available_inputs": inputs, "executed_calculations": calculations, "conflicts": conflicts, "missing_inputs": missing}, nil
}
