// internal/server/agent/runtime/sink_history.go
package runtime

import (
	"encoding/json"
	"fmt"
	"strings"

	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	agentSvc "github.com/ArtisanCloud/PowerX/internal/service/agent"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
)

type HistorySink struct {
	next       EventSink
	his        *agentSvc.ChatHistoryService
	ginCtx     *gin.Context
	env        string
	tenantUUID *string
	session    *dbmodel.AgentChatSession
	agentID    uint64
	skillState *agentSvc.SkillStateService
	buf        strings.Builder
	runState   map[string]any
	pending    map[string]any
	enabled    bool // 允许某些通道不开启历史
}

func NewHistorySink(next EventSink, his *agentSvc.ChatHistoryService, ginCtx *gin.Context,
	env string, tenantUUID *string, session *dbmodel.AgentChatSession, agentID uint64, enabled bool,
) *HistorySink {
	return &HistorySink{next: next, his: his, ginCtx: ginCtx, env: env, tenantUUID: tenantUUID, session: session, agentID: agentID, enabled: enabled}
}

func (h *HistorySink) WithSkillStateService(skillState *agentSvc.SkillStateService) *HistorySink {
	if h != nil {
		h.skillState = skillState
	}
	return h
}

func (h *HistorySink) Emit(event string, payload any) error {
	// 先向下游发送（保证实时性）
	if err := h.next.Emit(event, payload); err != nil {
		return err
	}

	// 侧效：收集/写库（仅在开启时）
	if !h.enabled || h.session == nil {
		return nil
	}
	switch event {
	case dto.EventToken:
		// 统一处理几种可能字段
		switch p := payload.(type) {
		case map[string]any:
			if s, ok := p["delta"].(string); ok && s != "" {
				h.buf.WriteString(s)
			}
		}
	case dto.EventAgentRunTaskStatus, dto.EventAgentRunTaskStarted, dto.EventAgentRunAwaitingParams, dto.EventAgentRunTaskCompleted, dto.EventAgentRunTaskFailed:
		h.captureRunStateTask(event, payload)
	case dto.EventAgentRunFinal:
		// final 时落库 assistant 文本
		text := extractAssistantText(payload)
		if strings.TrimSpace(text) == "" {
			text = SanitizeAssistantVisibleText(h.buf.String())
		}
		if strings.TrimSpace(text) != "" {
			meta := extractAssistantTraceMeta(payload)
			h.mergeRunStateMeta(meta)
			msg, _ := h.his.AppendMessage(h.ginCtx.Request.Context(),
				h.env, h.tenantUUID, h.session.ID, h.agentID, "assistant", text, "text", 0, 0, false, meta)
			if err := h.persistPendingSkillState(msg, meta); err != nil {
				return err
			}
			_, _ = h.his.SummarizeIfNeeded(h.ginCtx.Request.Context(), h.env, h.tenantUUID, h.session)
		}
	}
	return nil
}

func (h *HistorySink) captureRunStateTask(event string, payload any) {
	if h == nil {
		return
	}
	task := mapFromAny(payload)
	if len(task) == 0 {
		return
	}
	if h.runState == nil {
		h.runState = map[string]any{"tasks": []any{}}
	}
	tasks, _ := h.runState["tasks"].([]any)
	task["event"] = event
	tasks = append(tasks, task)
	h.runState["tasks"] = tasks
	if event == dto.EventAgentRunAwaitingParams || strings.EqualFold(readTraceMetaString(task["status"]), dto.AgentTaskStatusAwaitingParams) {
		task["status"] = dto.AgentTaskStatusAwaitingParams
		h.pending = task
	}
}

func (h *HistorySink) mergeRunStateMeta(meta datatypes.JSONMap) {
	if meta == nil {
		return
	}
	if len(h.runState) > 0 {
		meta["run_state"] = h.runState
	}
	if len(h.pending) > 0 {
		meta["pending_task"] = h.pending
	}
}

func (h *HistorySink) persistPendingSkillState(msg *dbmodel.AgentChatMessage, meta datatypes.JSONMap) error {
	if h == nil || h.skillState == nil || h.ginCtx == nil || h.session == nil || len(h.pending) == 0 {
		return nil
	}
	skillID := strings.TrimSpace(readTraceMetaString(h.pending["skill_id"]))
	if skillID == "" {
		skillID = strings.TrimSpace(readTraceMetaString(h.pending["node_ref"]))
	}
	stateKey := strings.TrimSpace(readTraceMetaString(h.pending["state_key"]))
	if stateKey == "" {
		action := strings.TrimSpace(readTraceMetaString(h.pending["action"]))
		if action == "" {
			action = "default"
		}
		stateKey = skillID + "." + action
	}
	if skillID == "" || stateKey == "" {
		return nil
	}
	state := datatypes.JSONMap{}
	if action := strings.TrimSpace(readTraceMetaString(h.pending["action"])); action != "" {
		state["action"] = action
	}
	if collected := mapFromAny(h.pending["collected_params"]); len(collected) > 0 {
		state["collected"] = collected
	}
	if missing := stringSliceFromAny(h.pending["missing_fields"]); len(missing) > 0 {
		state["missing"] = missing
	}
	if request := mapFromAny(h.pending["capability_request"]); len(request) > 0 {
		state["capability_request"] = request
	}
	skillMeta := datatypes.JSONMap{}
	if trace := mapFromAny(meta["trace"]); len(trace) > 0 {
		for _, key := range []string{"trace_id", "run_id", "plan_id"} {
			if value := strings.TrimSpace(readTraceMetaString(trace[key])); value != "" {
				skillMeta[key] = value
			}
		}
	}
	lastMessageID := uint64(0)
	if msg != nil {
		lastMessageID = msg.ID
	}
	_, err := h.skillState.Upsert(h.ginCtx.Request.Context(), agentSvc.SkillStateUpsertInput{
		Env:           h.env,
		TenantUUID:    h.tenantUUID,
		SessionID:     h.session.ID,
		AgentID:       h.agentID,
		SkillID:       skillID,
		StateKey:      stateKey,
		SchemaVersion: "1.0",
		Status:        strings.TrimSpace(readTraceMetaString(h.pending["status"])),
		Action:        strings.TrimSpace(readTraceMetaString(h.pending["action"])),
		State:         state,
		Meta:          skillMeta,
		LastMessageID: lastMessageID,
		TTLSeconds:    int64(h.session.TTLDays) * 24 * 3600,
	})
	if err != nil {
		return fmt.Errorf("persist pending skill state: %w", err)
	}
	return nil
}

func mapFromAny(payload any) map[string]any {
	switch v := payload.(type) {
	case nil:
		return nil
	case map[string]any:
		return v
	case datatypes.JSONMap:
		out := make(map[string]any, len(v))
		for k, item := range v {
			out[k] = item
		}
		return out
	case string:
		text := strings.TrimSpace(v)
		if text == "" {
			return nil
		}
		var out map[string]any
		if err := json.Unmarshal([]byte(text), &out); err == nil {
			return out
		}
	case dto.AgentTaskState:
		return map[string]any{
			"run_id":           v.RunID,
			"session_id":       v.SessionID,
			"message_id":       v.MessageID,
			"trace_id":         v.TraceID,
			"task_id":          v.TaskID,
			"parent_task_id":   v.ParentTaskID,
			"team_id":          v.TeamID,
			"agent_id":         v.AgentID,
			"agent_key":        v.AgentKey,
			"agent_name":       v.AgentName,
			"node_kind":        v.NodeKind,
			"node_ref":         v.NodeRef,
			"skill_id":         v.SkillID,
			"capability_id":    v.CapabilityID,
			"action":           v.Action,
			"status":           v.Status,
			"collected_params": v.CollectedParams,
			"missing_fields":   v.MissingFields,
			"result":           v.Result,
			"links":            v.Links,
			"error":            v.Error,
			"updated_at":       v.UpdatedAt,
		}
	default:
		return nil
	}
	return nil
}

func extractAssistantText(payload any) string {
	// Agent Run State Protocol final is wrapped as dto.AgentRunEvent{Payload:{data:{content:"..."}}}.
	switch m := payload.(type) {
	case dto.AgentRunEvent:
		return extractAssistantText(m.Payload)
	case map[string]any:
		if p, ok := m["payload"].(map[string]any); ok {
			if s := extractAssistantText(p); strings.TrimSpace(s) != "" {
				return s
			}
		}
		if d, ok := m["data"].(map[string]any); ok {
			if s, ok := d["content"].(string); ok {
				return SanitizeAssistantVisibleText(s)
			}
			if r, ok := d["result"].(map[string]any); ok {
				if s, ok := r["content"].(string); ok {
					return SanitizeAssistantVisibleText(s)
				}
				if s, ok := r["message"].(string); ok {
					return SanitizeAssistantVisibleText(s)
				}
			}
		}
		if s, ok := m["content"].(string); ok {
			return SanitizeAssistantVisibleText(s)
		}
		if s, ok := m["message"].(string); ok {
			return SanitizeAssistantVisibleText(s)
		}
	}
	return ""
}

func extractAssistantTraceMeta(payload any) datatypes.JSONMap {
	out := datatypes.JSONMap{}
	if event, ok := payload.(dto.AgentRunEvent); ok {
		if strings.TrimSpace(event.TraceID) != "" {
			out["trace"] = datatypes.JSONMap{
				"trace_id":   event.TraceID,
				"run_id":     event.RunID,
				"session_id": event.SessionID,
				"message_id": event.MessageID,
				"run_event":  event.Event,
			}
		}
		nested := extractAssistantTraceMeta(event.Payload)
		for key, value := range nested {
			if key == "trace" {
				trace, _ := out["trace"].(datatypes.JSONMap)
				if trace == nil {
					trace = datatypes.JSONMap{}
				}
				if nestedTrace, ok := value.(datatypes.JSONMap); ok {
					for nestedKey, nestedValue := range nestedTrace {
						trace[nestedKey] = nestedValue
					}
				}
				if len(trace) > 0 {
					out["trace"] = trace
				}
				continue
			}
			out[key] = value
		}
		return out
	}
	m, ok := payload.(map[string]any)
	if !ok {
		return out
	}
	if p, ok := m["payload"].(map[string]any); ok {
		nested := extractAssistantTraceMeta(p)
		for key, value := range nested {
			out[key] = value
		}
	}
	md := map[string]any{}
	if raw, ok := m["metadata"].(map[string]any); ok {
		md = raw
	}
	trace := datatypes.JSONMap{}
	for _, key := range []string{"tenant_uuid", "trace_id", "run_id", "session_id", "session_uuid", "message_id", "plan_id", "agent_id"} {
		if v, ok := md[key]; ok && strings.TrimSpace(readTraceMetaString(v)) != "" {
			trace[key] = v
			continue
		}
		if v, ok := m[key]; ok && strings.TrimSpace(readTraceMetaString(v)) != "" {
			trace[key] = v
		}
	}
	if len(trace) > 0 {
		out["trace"] = trace
	}
	if raw, ok := md["response_plan"].(map[string]any); ok {
		out["response_plan"] = raw
		copyResponseMeta(out, raw, md)
	} else if raw, ok := m["response_plan"].(map[string]any); ok {
		out["response_plan"] = raw
		copyResponseMeta(out, raw, md)
	} else {
		copyResponseMeta(out, md, md)
	}
	return out
}

func copyResponseMeta(out datatypes.JSONMap, plan map[string]any, md map[string]any) {
	if out == nil {
		return
	}
	copyIfPresent := func(dstKey, srcKey string) {
		if v, ok := plan[srcKey]; ok {
			out[dstKey] = v
			return
		}
		if v, ok := md[srcKey]; ok {
			out[dstKey] = v
		}
	}
	copyIfPresent("response_mode", "response_mode")
	copyIfPresent("capability_ids", "target_capability_ids")
	copyIfPresent("response_plan_id", "response_plan_id")
	copyIfPresent("used_context_layers", "used_context_layers")
	copyIfPresent("tool_calls", "tool_calls")
	copyIfPresent("final_response_model", "final_response_model")
	copyIfPresent("model_selection", "model_selection")
}

func readTraceMetaString(v any) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(v))
}
