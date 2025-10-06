package runtime

import (
	"encoding/json"

	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	dto "github.com/ArtisanCloud/PowerX/pkg/dto"
)

// 尝试把任意 raw 规整为 flowschema.ExecutionPlan。
func NormalizeExecPlan(raw any) (*flowschema.ExecutionPlan, bool) {
	switch v := raw.(type) {
	case *flowschema.ExecutionPlan:
		if v != nil {
			return v, true
		}
		return nil, false
	case flowschema.ExecutionPlan:
		ep := v
		return &ep, true

	case *dto.PlanPayload:
		if v == nil {
			return nil, false
		}
		return planPayloadToExecPlan(v), true
	case dto.PlanPayload:
		pp := v
		return planPayloadToExecPlan(&pp), true

	default:
		if raw == nil {
			return nil, false
		}
		b, err := json.Marshal(raw)
		if err != nil {
			return nil, false
		}
		var ep flowschema.ExecutionPlan
		if err := json.Unmarshal(b, &ep); err == nil && len(ep.Tasks) > 0 {
			return &ep, true
		}
		var pp dto.PlanPayload
		if err := json.Unmarshal(b, &pp); err == nil && len(pp.Tasks) > 0 {
			return planPayloadToExecPlan(&pp), true
		}
		return nil, false
	}
}

// 如果规整成功返回 plan，否则回退为 JSON 友好结构返回 raw。
func PlanOrRaw(plan *flowschema.ExecutionPlan, raw any) any {
	if plan != nil {
		return plan
	}
	if raw == nil {
		return nil
	}
	switch raw.(type) {
	case map[string]any,
		*flowschema.ExecutionPlan, flowschema.ExecutionPlan,
		*dto.PlanPayload, dto.PlanPayload:
		return raw
	}
	var m any
	if b, err := json.Marshal(raw); err == nil {
		if err := json.Unmarshal(b, &m); err == nil {
			return m
		}
	}
	return raw
}

func planPayloadToExecPlan(pp *dto.PlanPayload) *flowschema.ExecutionPlan {
	ep := &flowschema.ExecutionPlan{
		PlanID: pp.PlanID,
		Tasks:  make([]flowschema.PlanTask, 0, len(pp.Tasks)),
	}
	for _, t := range pp.Tasks {
		ep.Tasks = append(ep.Tasks, flowschema.PlanTask{
			TaskID:    t.TaskID,
			FlowID:    t.FlowID,
			AgentID:   t.AgentID,
			Params:    t.Params,
			ParamRefs: t.ParamRefs,
			Stage:     t.Stage,
			DependsOn: t.DependsOn,
		})
	}
	return ep
}
