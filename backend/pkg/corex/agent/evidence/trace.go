package evidence

import "encoding/json"

// TraceSummary excludes source prose. The full report belongs to tenant-scoped message storage.
func TraceSummary(report map[string]any) map[string]any {
	raw, err := json.Marshal(report)
	if err != nil {
		return nil
	}
	var r Report
	if err := Decode(raw, &r); err != nil {
		return nil
	}
	computed := []map[string]any{}
	for _, c := range r.Presentation.Computed {
		operands := map[string]any{}
		for key, d := range c.Operands {
			operands[key] = map[string]any{"datum_key": d.Key, "value": d.Value, "unit": d.Unit, "scope": d.Scope, "source_pointer": d.Source.Pointer}
		}
		computed = append(computed, map[string]any{"uuid": c.UUID, "tool_key": c.ToolKey, "tool_version": c.ToolVersion, "request": c.Request, "operands": operands, "display_value": c.DisplayValue, "started_at": c.StartedAt, "ended_at": c.EndedAt})
	}
	return map[string]any{"tenant_uuid": r.TenantUUID, "revision_uuid": r.RevisionUUID, "trace_id": r.TraceID, "source_digest": r.SourceDigest, "outcome": r.Outcome, "computed": computed, "conflicts": r.Presentation.Conflicts}
}
