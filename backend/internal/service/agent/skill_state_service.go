package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	repo "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/repository"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type SkillStateService struct {
	repo *repo.AgentSessionSkillStateRepository
}

type SkillStateUpsertInput struct {
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

func NewSkillStateService(db *gorm.DB) *SkillStateService {
	return &SkillStateService{repo: repo.NewAgentSessionSkillStateRepository(db)}
}

func (s *SkillStateService) Upsert(ctx context.Context, in SkillStateUpsertInput) (*dbmodel.AgentSessionSkillState, error) {
	var expiresAt *time.Time
	if in.TTLSeconds > 0 {
		t := time.Now().Add(time.Duration(in.TTLSeconds) * time.Second)
		expiresAt = &t
	}
	return s.repo.Upsert(ctx, repo.SkillStateUpsert{
		Env:           in.Env,
		TenantUUID:    in.TenantUUID,
		SessionID:     in.SessionID,
		AgentID:       in.AgentID,
		SkillID:       in.SkillID,
		StateKey:      in.StateKey,
		SchemaVersion: in.SchemaVersion,
		Status:        in.Status,
		Action:        in.Action,
		State:         in.State,
		Meta:          in.Meta,
		LastMessageID: in.LastMessageID,
		ExpiresAt:     expiresAt,
	})
}

func (s *SkillStateService) LatestPendingTask(
	ctx context.Context,
	env string,
	tenantUUID *string,
	sessionID uint64,
	agentID uint64,
	boundSkillIDs []string,
) (datatypes.JSONMap, bool, error) {
	row, err := s.repo.LatestBySession(ctx, env, tenantUUID, sessionID, agentID, boundSkillIDs)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, false, nil
		}
		return nil, false, err
	}
	if row == nil || !isPendingSkillStateStatus(row.Status) {
		return nil, false, nil
	}
	return SkillStateToPendingTask(row), true, nil
}

func SkillStateToPendingTask(row *dbmodel.AgentSessionSkillState) datatypes.JSONMap {
	if row == nil {
		return nil
	}
	state := cloneJSONMap(row.State)
	task := datatypes.JSONMap{
		"status":          normalizePendingStatus(row.Status),
		"task_id":         fmt.Sprintf("skill_state_%d", row.ID),
		"node_kind":       "skill",
		"node_ref":        row.SkillID,
		"skill_id":        row.SkillID,
		"source_scope":    "agent",
		"agent_id":        fmt.Sprintf("%d", row.AgentID),
		"session_id":      fmt.Sprintf("%d", row.SessionID),
		"state_key":       row.StateKey,
		"schema_version":  row.SchemaVersion,
		"skill_state_id":  fmt.Sprintf("%d", row.ID),
		"skill_state_ver": row.Version,
	}
	if row.LastMessageID > 0 {
		task["message_id"] = fmt.Sprintf("%d", row.LastMessageID)
	}
	if action := strings.TrimSpace(row.Action); action != "" {
		task["action"] = action
	} else if action = stringFromJSONMap(state, "action"); action != "" {
		task["action"] = action
	}
	if collected := jsonMapFromAny(state["collected"]); len(collected) > 0 {
		task["collected_params"] = collected
	}
	if missing := stringListFromAny(state["missing"]); len(missing) > 0 {
		task["missing_fields"] = missing
	}
	if request := jsonMapFromAny(state["capability_request"]); len(request) > 0 {
		task["capability_request"] = request
		if capabilityID := stringFromJSONMap(request, "capability_id"); capabilityID != "" {
			task["capability_id"] = capabilityID
		}
	}
	for _, key := range []string{"trace_id", "run_id", "plan_id"} {
		if value := stringFromJSONMap(row.Meta, key); value != "" {
			task[key] = value
		}
	}
	return task
}

func isPendingSkillStateStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "awaiting_params", "collecting", "ready", "awaiting_confirmation", "executing":
		return true
	default:
		return false
	}
}

func normalizePendingStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "awaiting_params", "ready", "awaiting_confirmation":
		return "awaiting_params"
	case "executing":
		return "running"
	default:
		return "awaiting_params"
	}
}

func cloneJSONMap(in datatypes.JSONMap) datatypes.JSONMap {
	out := datatypes.JSONMap{}
	for k, v := range in {
		out[k] = v
	}
	return out
}

func jsonMapFromAny(value any) datatypes.JSONMap {
	switch v := value.(type) {
	case datatypes.JSONMap:
		return v
	case map[string]any:
		return datatypes.JSONMap(v)
	default:
		return nil
	}
}

func stringFromJSONMap(in datatypes.JSONMap, key string) string {
	if in == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(in[key]))
}

func stringListFromAny(value any) []string {
	switch v := value.(type) {
	case []string:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if item = strings.TrimSpace(item); item != "" {
				out = append(out, item)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			text := strings.TrimSpace(fmt.Sprint(item))
			if text != "" {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}
