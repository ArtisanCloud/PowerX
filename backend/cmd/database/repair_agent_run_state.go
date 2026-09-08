package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	agentmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	agenttrace "github.com/ArtisanCloud/PowerX/internal/service/agent_trace"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// agentRunStateRepairResult deliberately exposes every skipped or refused row:
// a historical snapshot is only overwritten when the exact same run has a
// canonical local Trace state. No inferred task state is ever written.
type agentRunStateRepairResult struct {
	Scanned       int      `json:"scanned"`
	Candidates    int      `json:"candidates"`
	Repaired      int      `json:"repaired"`
	Unchanged     int      `json:"unchanged"`
	Refused       int      `json:"refused"`
	MessageIDs    []uint64 `json:"message_ids,omitempty"`
	RefusalReason []string `json:"refusal_reasons,omitempty"`
}

type agentRunStateRepairCandidate struct {
	message *agentmodel.AgentChatMessage
	state   datatypes.JSONMap
}

func repairLegacyAgentRunStates(ctx context.Context, db *gorm.DB, localTraceDir string, apply bool) (*agentRunStateRepairResult, error) {
	if db == nil {
		return nil, fmt.Errorf("agent_run_state_repair_db_required")
	}
	var messages []agentmodel.AgentChatMessage
	if err := db.WithContext(ctx).
		Where("role = ?", "assistant").
		Where("meta ? 'run_state'").
		Order("id ASC").
		Find(&messages).Error; err != nil {
		return nil, fmt.Errorf("agent_run_state_repair_list_messages: %w", err)
	}

	result := &agentRunStateRepairResult{Scanned: len(messages)}
	source := agenttrace.NewLocalSink(localTraceDir)
	candidates := make([]agentRunStateRepairCandidate, 0)
	for index := range messages {
		message := &messages[index]
		legacyState := jsonMap(message.Meta["run_state"])
		if !hasDuplicateRunStateTaskID(legacyState) {
			result.Unchanged++
			continue
		}
		result.Candidates++
		canonical, err := canonicalRunStateForMessage(ctx, source, message)
		if err != nil {
			result.Refused++
			result.RefusalReason = append(result.RefusalReason, fmt.Sprintf("message=%d:%v", message.ID, err))
			continue
		}
		candidates = append(candidates, agentRunStateRepairCandidate{message: message, state: canonical})
	}
	if !apply {
		return result, nil
	}
	if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, candidate := range candidates {
			meta := cloneJSONMap(candidate.message.Meta)
			meta["run_state"] = candidate.state
			if err := tx.Model(&agentmodel.AgentChatMessage{}).
				Where("id = ?", candidate.message.ID).
				Update("meta", meta).Error; err != nil {
				return fmt.Errorf("agent_run_state_repair_update_message_%d: %w", candidate.message.ID, err)
			}
			result.Repaired++
			result.MessageIDs = append(result.MessageIDs, candidate.message.ID)
		}
		return nil
	}); err != nil {
		return result, err
	}
	return result, nil
}

func canonicalRunStateForMessage(ctx context.Context, source *agenttrace.LocalSink, message *agentmodel.AgentChatMessage) (datatypes.JSONMap, error) {
	if message == nil {
		return nil, fmt.Errorf("agent_run_state_repair_message_required")
	}
	trace := jsonMap(message.Meta["trace"])
	tenantUUID := firstNonBlank(stringValue(trace["tenant_uuid"]), stringPointerValue(message.TenantUUID))
	sessionID := firstNonBlank(stringValue(trace["session_id"]), fmt.Sprintf("%d", message.SessionID))
	messageID := strings.TrimSpace(stringValue(trace["message_id"]))
	runID := strings.TrimSpace(stringValue(trace["run_id"]))
	traceID := strings.TrimSpace(stringValue(trace["trace_id"]))
	if tenantUUID == "" || sessionID == "" || messageID == "" || runID == "" || traceID == "" {
		return nil, fmt.Errorf("agent_run_state_repair_trace_identity_missing")
	}
	report, err := source.BuildReport(ctx, agenttrace.AgentReportQuery{
		TenantUUID: tenantUUID,
		SessionID:  sessionID,
		MessageID:  messageID,
		RunID:      runID,
		TraceID:    traceID,
		Source:     "local",
		Format:     "json",
	})
	if err != nil {
		return nil, fmt.Errorf("agent_run_state_repair_trace_report: %w", err)
	}
	if report == nil || report.RunState == nil || !report.RunState.Ended || len(report.RunState.Tasks) == 0 {
		return nil, fmt.Errorf("agent_run_state_repair_canonical_state_unavailable")
	}
	if report.RunID != runID || report.TraceID != traceID || report.TenantUUID != tenantUUID || report.SessionID != sessionID || report.MessageID != messageID {
		return nil, fmt.Errorf("agent_run_state_repair_trace_identity_mismatch")
	}
	raw, err := json.Marshal(report.RunState)
	if err != nil {
		return nil, fmt.Errorf("agent_run_state_repair_encode_canonical_state: %w", err)
	}
	state := datatypes.JSONMap{}
	if err := json.Unmarshal(raw, &state); err != nil {
		return nil, fmt.Errorf("agent_run_state_repair_decode_canonical_state: %w", err)
	}
	if hasDuplicateRunStateTaskID(state) {
		return nil, fmt.Errorf("agent_run_state_repair_canonical_state_invalid")
	}
	return state, nil
}

func hasDuplicateRunStateTaskID(state map[string]any) bool {
	tasks, ok := state["tasks"].([]any)
	if !ok {
		return false
	}
	seen := make(map[string]struct{}, len(tasks))
	for _, raw := range tasks {
		taskID := strings.TrimSpace(stringValue(jsonMap(raw)["task_id"]))
		if taskID == "" {
			continue
		}
		if _, exists := seen[taskID]; exists {
			return true
		}
		seen[taskID] = struct{}{}
	}
	return false
}

func jsonMap(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		return typed
	case datatypes.JSONMap:
		return map[string]any(typed)
	default:
		return map[string]any{}
	}
}

func cloneJSONMap(value datatypes.JSONMap) datatypes.JSONMap {
	raw, _ := json.Marshal(value)
	cloned := datatypes.JSONMap{}
	_ = json.Unmarshal(raw, &cloned)
	return cloned
}

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func stringPointerValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
