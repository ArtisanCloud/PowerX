package evidence

import (
	"context"
	"encoding/json"
	"fmt"
)

const PlanSchema = "powerx.agent.evidence-plan/v1"
const NotesSchema = "powerx.agent.evidence-notes/v1"
const ExtractionSchema = "powerx.agent.evidence-source/v1"

// Plan 只描述输入和计算请求，不接受展示值或说明文字。
type Plan struct {
	Schema       string        `json:"schema"`
	Kind         string        `json:"kind"`
	Data         []DraftDatum  `json:"data"`
	Calculations []Calculation `json:"calculations"`
}

type Notes struct {
	Schema     string   `json:"schema"`
	Hypotheses []string `json:"hypotheses"`
	Gaps       []string `json:"gaps"`
	Actions    []string `json:"actions"`
}

func NotesJSONSchema() map[string]any {
	p := map[string]any{"schema": map[string]any{"const": NotesSchema}}
	for _, key := range []string{"hypotheses", "gaps", "actions"} {
		p[key] = map[string]any{"type": "array", "maxItems": 6, "items": map[string]any{"type": "string", "maxLength": 300}}
	}
	return map[string]any{"type": "object", "additionalProperties": false, "properties": p, "required": []string{"schema", "hypotheses", "gaps", "actions"}}
}

func ExecutePlan(ctx context.Context, plan Plan, payload map[string]any, sources []string, tenant, revision, trace string) (map[string]any, error) {
	if plan.Schema != PlanSchema {
		return nil, fmt.Errorf("evidence.plan_schema_invalid")
	}
	return compileDraft(ctx, Draft{Schema: DraftSchema, Kind: plan.Kind, Data: plan.Data, Calculations: plan.Calculations, Hypotheses: []string{}, Gaps: []string{}, Actions: []string{}}, payload, sources, tenant, revision, trace, true)
}

// AttachNotes 只给本次真实计算结果追加说明，模型没有重写数值和凭证的入口。
func AttachNotes(ctx context.Context, prepared map[string]any, notes Notes) (map[string]any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := Verify(ctx, prepared); err != nil {
		return nil, err
	}
	if notes.Schema != NotesSchema || notes.Hypotheses == nil || notes.Gaps == nil || notes.Actions == nil {
		return nil, fmt.Errorf("evidence.notes_schema_invalid")
	}
	raw, err := json.Marshal(prepared)
	if err != nil {
		return nil, err
	}
	var report Report
	if err := Decode(raw, &report); err != nil {
		return nil, err
	}
	report.Presentation.Hypotheses = notes.Hypotheses
	report.Presentation.Gaps = notes.Gaps
	report.Presentation.Actions = notes.Actions
	return sealReport(ctx, report, false)
}
