package runtime

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"gorm.io/datatypes"
)

type SkillStateStore interface {
	UpsertSkillState(ctx context.Context, in SkillStateUpsert) error
}

type SkillStateUpsert struct {
	Env           string
	TenantUUID    *string
	SessionID     uint64
	AgentID       uint64
	SkillID       string
	StateKey      string
	SchemaVersion string
	Status        string
	Action        string
	State         datatypes.JSONMap
	Meta          datatypes.JSONMap
	LastMessageID uint64
	TTLSeconds    int64
}

type skillStateStoreContextKey struct{}

func ContextWithSkillStateStore(ctx context.Context, store SkillStateStore) context.Context {
	if ctx == nil || store == nil {
		return ctx
	}
	return context.WithValue(ctx, skillStateStoreContextKey{}, store)
}

func skillStateStoreFromContext(ctx context.Context) SkillStateStore {
	if ctx == nil {
		return nil
	}
	store, _ := ctx.Value(skillStateStoreContextKey{}).(SkillStateStore)
	return store
}

func persistAwaitingSkillState(ctx context.Context, task map[string]any) error {
	return persistRuntimeSkillState(ctx, task, dto.AgentTaskStatusAwaitingParams, nil)
}

func persistTaskSkillState(ctx context.Context, task flowschema.PlanTask, status string, out any, runErr error) error {
	payload := map[string]any{
		"task_id":          task.TaskID,
		"node_kind":        normalizeNodeKind(task.NodeKind),
		"node_ref":         normalizeNodeRef(task),
		"skill_id":         skillIDFromPlanTask(task),
		"source_scope":     normalizeSourceScope(task.SourceScope),
		"action":           taskAction(task),
		"collected_params": clonePlanParams(task.Params),
	}
	if task.Params != nil {
		payload["capability_id"] = strings.TrimSpace(fmt.Sprint(task.Params["capability_id"]))
		if stateKey := strings.TrimSpace(fmt.Sprint(task.Params["state_key"])); stateKey != "" {
			payload["state_key"] = stateKey
		}
	}
	if runErr != nil {
		payload["error"] = runErr.Error()
	}
	if out != nil {
		payload["result"] = out
	}
	return persistRuntimeSkillState(ctx, payload, status, runErr)
}

func persistRuntimeSkillState(ctx context.Context, payload map[string]any, status string, runErr error) error {
	store := skillStateStoreFromContext(ctx)
	if store == nil || len(payload) == 0 {
		return nil
	}
	skillID := firstNonEmpty(anyToString(payload["skill_id"]), anyToString(payload["node_ref"]))
	if strings.TrimSpace(skillID) == "" || !strings.EqualFold(normalizeNodeKind(anyToString(payload["node_kind"])), dto.NodeKindSkill) {
		return nil
	}
	action := strings.TrimSpace(anyToString(payload["action"]))
	stateKey := strings.TrimSpace(anyToString(payload["state_key"]))
	if stateKey == "" {
		if action == "" {
			action = "default"
		}
		stateKey = strings.TrimSpace(skillID) + "." + action
	}
	state := datatypes.JSONMap{}
	if action != "" {
		state["action"] = action
	}
	if collected := mapFromAny(payload["collected_params"]); len(collected) > 0 {
		state["collected"] = collected
	}
	if missing := normalizeStringList(anyStringSlice(payload["missing_fields"])); len(missing) > 0 {
		state["missing"] = missing
	}
	if request := mapFromAny(payload["capability_request"]); len(request) > 0 {
		state["capability_request"] = request
	}
	if result := mapFromAny(payload["result"]); len(result) > 0 {
		state["result"] = result
	}
	if runErr != nil {
		state["error"] = runErr.Error()
	} else if errText := strings.TrimSpace(anyToString(payload["error"])); errText != "" {
		state["error"] = errText
	}
	meta := datatypes.JSONMap{}
	for _, key := range []string{"trace_id", "run_id", "plan_id", "task_id", "capability_id"} {
		if value := strings.TrimSpace(anyToString(payload[key])); value != "" {
			meta[key] = value
		}
	}
	in := SkillStateUpsert{
		Env:           firstNonEmpty(ctxStringValue(ctx, "env"), ctxStringValue(ctx, "agent_env")),
		TenantUUID:    stringPtrOrNil(ctxStringValue(ctx, "tenant_uuid")),
		SessionID:     parseUint64String(ctxStringValue(ctx, "session_id")),
		AgentID:       parseUint64String(firstNonEmpty(ctxStringValue(ctx, "agent_id"), anyToString(payload["agent_id"]))),
		SkillID:       skillID,
		StateKey:      stateKey,
		SchemaVersion: "1.0",
		Status:        normalizeSkillStateStatus(status),
		Action:        action,
		State:         state,
		Meta:          meta,
		LastMessageID: parseUint64String(ctxStringValue(ctx, "message_id")),
		TTLSeconds:    int64(3 * 24 * time.Hour / time.Second),
	}
	if err := validateRuntimeSkillStateScope(in); err != nil {
		return err
	}
	if err := store.UpsertSkillState(ctx, in); err != nil {
		return fmt.Errorf("persist skill state: %w", err)
	}
	return nil
}

func validateRuntimeSkillStateScope(in SkillStateUpsert) error {
	if strings.TrimSpace(in.Env) == "" {
		return fmt.Errorf("env is required for runtime skill state")
	}
	if in.SessionID == 0 {
		return fmt.Errorf("session_id is required for runtime skill state")
	}
	if in.AgentID == 0 {
		return fmt.Errorf("agent_id is required for runtime skill state")
	}
	if in.LastMessageID == 0 {
		return fmt.Errorf("message_id is required for runtime skill state")
	}
	if strings.TrimSpace(in.SkillID) == "" {
		return fmt.Errorf("skill_id is required for runtime skill state")
	}
	if strings.TrimSpace(in.StateKey) == "" {
		return fmt.Errorf("state_key is required for runtime skill state")
	}
	return nil
}

func normalizeSkillStateStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case dto.AgentTaskStatusAwaitingParams:
		return "collecting"
	case dto.AgentTaskStatusRunning:
		return "executing"
	case dto.AgentTaskStatusCompleted:
		return "completed"
	case dto.AgentTaskStatusFailed:
		return "failed"
	default:
		return strings.TrimSpace(status)
	}
}

func ctxStringValue(ctx context.Context, key string) string {
	if ctx == nil || strings.TrimSpace(key) == "" {
		return ""
	}
	raw := ctx.Value(key)
	if raw == nil {
		return ""
	}
	value := strings.TrimSpace(fmt.Sprint(raw))
	if value == "<nil>" {
		return ""
	}
	return value
}

func stringPtrOrNil(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" || value == "<nil>" {
		return nil
	}
	return &value
}

func parseUint64String(value string) uint64 {
	value = strings.TrimSpace(value)
	if value == "" || value == "<nil>" {
		return 0
	}
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return 0
	}
	return parsed
}
