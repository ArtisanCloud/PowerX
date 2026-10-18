package evidence

import "encoding/json"

// TraceSummary excludes source prose. The full report belongs to tenant-scoped message storage.
func TraceSummary(report map[string]any) map[string]any {
	if report["schema"] == ReportSchemaV5 {
		raw, err := json.Marshal(report)
		if err != nil {
			return nil
		}
		var r ReportV5
		if DecodeToolJSON(raw, &r) != nil {
			return nil
		}
		receipts := []map[string]any{}
		for _, receipt := range r.Receipts {
			receipts = append(receipts, map[string]any{"execution_uuid": receipt.ExecutionUUID, "node_id": receipt.NodeID, "attempt": receipt.Attempt, "tool_key": receipt.Tool.ToolKey, "tool_version": receipt.Tool.Version, "status": receipt.Status, "request": receipt.Request, "result": receipt.Result, "error": receipt.Error})
		}
		return map[string]any{"tenant_uuid": r.TenantUUID, "revision_uuid": r.RevisionUUID, "run_id": r.RunID, "trace_id": r.TraceID, "source_digest": r.SourceDigest, "outcome": r.Outcome, "receipts": receipts, "computed": r.Presentation.Computed, "conflicts": r.Presentation.Conflicts, "gaps": r.Presentation.Gaps}
	}
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
