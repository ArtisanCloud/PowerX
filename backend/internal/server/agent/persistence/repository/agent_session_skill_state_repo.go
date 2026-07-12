package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	coreRepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type AgentSessionSkillStateRepository struct {
	*coreRepo.BaseRepository[dbmodel.AgentSessionSkillState]
	db *gorm.DB
}

func NewAgentSessionSkillStateRepository(db *gorm.DB) *AgentSessionSkillStateRepository {
	return &AgentSessionSkillStateRepository{
		BaseRepository: coreRepo.NewBaseRepository[dbmodel.AgentSessionSkillState](db),
		db:             db,
	}
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
	ExpiresAt     *time.Time
}

func (r *AgentSessionSkillStateRepository) Upsert(ctx context.Context, in SkillStateUpsert) (*dbmodel.AgentSessionSkillState, error) {
	if err := validateSkillStateIdentity(in.Env, in.SessionID, in.AgentID, in.SkillID, in.StateKey); err != nil {
		return nil, err
	}
	row := dbmodel.AgentSessionSkillState{
		Env:           strings.TrimSpace(in.Env),
		TenantUUID:    in.TenantUUID,
		SessionID:     in.SessionID,
		AgentID:       in.AgentID,
		SkillID:       strings.TrimSpace(in.SkillID),
		StateKey:      strings.TrimSpace(in.StateKey),
		SchemaVersion: defaultString(in.SchemaVersion, "1.0"),
		Status:        defaultString(in.Status, "collecting"),
		Action:        strings.TrimSpace(in.Action),
		State:         nonNilJSONMap(in.State),
		Meta:          nonNilJSONMap(in.Meta),
		LastMessageID: in.LastMessageID,
		Version:       1,
		ExpiresAt:     in.ExpiresAt,
	}
	now := time.Now()
	err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "env"},
			{Name: "tenant_uuid"},
			{Name: "session_id"},
			{Name: "agent_id"},
			{Name: "skill_id"},
			{Name: "state_key"},
		},
		DoUpdates: clause.Assignments(map[string]any{
			"schema_version":  row.SchemaVersion,
			"status":          row.Status,
			"action":          row.Action,
			"state":           row.State,
			"meta":            row.Meta,
			"last_message_id": row.LastMessageID,
			"expires_at":      row.ExpiresAt,
			"version":         gorm.Expr("agent_session_skill_states.version + 1"),
			"deleted_at":      nil,
			"updated_at":      now,
		}),
	}).Create(&row).Error
	if err != nil {
		return nil, err
	}
	return r.Get(ctx, in.Env, in.TenantUUID, in.SessionID, in.AgentID, in.SkillID, in.StateKey)
}

func (r *AgentSessionSkillStateRepository) Get(
	ctx context.Context,
	env string,
	tenantUUID *string,
	sessionID uint64,
	agentID uint64,
	skillID string,
	stateKey string,
) (*dbmodel.AgentSessionSkillState, error) {
	if err := validateSkillStateIdentity(env, sessionID, agentID, skillID, stateKey); err != nil {
		return nil, err
	}
	var row dbmodel.AgentSessionSkillState
	err := r.db.WithContext(ctx).
		Scopes(dbmodel.WithScope(strings.TrimSpace(env), tenantUUID)).
		Where("session_id = ? AND agent_id = ? AND skill_id = ? AND state_key = ?", sessionID, agentID, strings.TrimSpace(skillID), strings.TrimSpace(stateKey)).
		First(&row).Error
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *AgentSessionSkillStateRepository) LatestBySession(
	ctx context.Context,
	env string,
	tenantUUID *string,
	sessionID uint64,
	agentID uint64,
	skillIDs []string,
) (*dbmodel.AgentSessionSkillState, error) {
	env = strings.TrimSpace(env)
	if env == "" {
		return nil, fmt.Errorf("env is required")
	}
	if sessionID == 0 {
		return nil, fmt.Errorf("session_id is required")
	}
	if agentID == 0 {
		return nil, fmt.Errorf("agent_id is required")
	}
	tx := r.db.WithContext(ctx).
		Scopes(dbmodel.WithScope(env, tenantUUID)).
		Where("session_id = ? AND agent_id = ?", sessionID, agentID)
	if ids := normalizeStateSkillIDs(skillIDs); len(ids) > 0 {
		tx = tx.Where("skill_id IN ?", ids)
	}
	var row dbmodel.AgentSessionSkillState
	if err := tx.Order("updated_at DESC, id DESC").First(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *AgentSessionSkillStateRepository) ListBySession(
	ctx context.Context,
	env string,
	tenantUUID *string,
	sessionID uint64,
	agentID uint64,
) ([]dbmodel.AgentSessionSkillState, error) {
	env = strings.TrimSpace(env)
	if env == "" {
		return nil, fmt.Errorf("env is required")
	}
	if sessionID == 0 {
		return nil, fmt.Errorf("session_id is required")
	}
	if agentID == 0 {
		return nil, fmt.Errorf("agent_id is required")
	}
	var rows []dbmodel.AgentSessionSkillState
	err := r.db.WithContext(ctx).
		Scopes(dbmodel.WithScope(env, tenantUUID)).
		Where("session_id = ? AND agent_id = ?", sessionID, agentID).
		Order("updated_at DESC, id DESC").
		Find(&rows).Error
	return rows, err
}

func validateSkillStateIdentity(env string, sessionID uint64, agentID uint64, skillID string, stateKey string) error {
	if strings.TrimSpace(env) == "" {
		return fmt.Errorf("env is required")
	}
	if sessionID == 0 {
		return fmt.Errorf("session_id is required")
	}
	if agentID == 0 {
		return fmt.Errorf("agent_id is required")
	}
	if strings.TrimSpace(skillID) == "" {
		return fmt.Errorf("skill_id is required")
	}
	if strings.TrimSpace(stateKey) == "" {
		return fmt.Errorf("state_key is required")
	}
	return nil
}

func nonNilJSONMap(in datatypes.JSONMap) datatypes.JSONMap {
	if in == nil {
		return datatypes.JSONMap{}
	}
	return in
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

func normalizeStateSkillIDs(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		id := strings.TrimSpace(value)
		if id == "" {
			continue
		}
		key := strings.ToLower(id)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, id)
	}
	return out
}
