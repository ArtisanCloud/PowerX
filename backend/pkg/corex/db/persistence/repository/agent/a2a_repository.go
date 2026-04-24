package agent

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/agent"
	baseRepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrAgentTeamNotFound       = errors.New("agent team not found")
	ErrAgentTeamMemberNotFound = errors.New("agent team member not found")
	ErrContextRefNotFound      = errors.New("agent context ref not found")
)

type AgentTeamRepository struct {
	*baseRepo.BaseRepository[model.AgentTeam]
	db *gorm.DB
}

func NewAgentTeamRepository(db *gorm.DB) *AgentTeamRepository {
	if db == nil {
		panic("agent team repository requires db")
	}
	return &AgentTeamRepository{BaseRepository: baseRepo.NewBaseRepository[model.AgentTeam](db), db: db}
}

func (r *AgentTeamRepository) GetByID(ctx context.Context, id uint64) (*model.AgentTeam, error) {
	var rec model.AgentTeam
	err := r.db.WithContext(ctx).Where("id = ?", id).Take(&rec).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrAgentTeamNotFound
	}
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

func (r *AgentTeamRepository) ListByTenantParent(ctx context.Context, tenantUUID string, parentAgentID uint64, includeDisabled bool) ([]model.AgentTeam, error) {
	q := r.db.WithContext(ctx).Model(&model.AgentTeam{}).
		Where("tenant_uuid = ?", strings.ToLower(strings.TrimSpace(tenantUUID))).
		Where("parent_agent_id = ?", parentAgentID)
	if !includeDisabled {
		q = q.Where("status = ?", model.TeamStatusActive)
	}
	var rows []model.AgentTeam
	if err := q.Order("id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *AgentTeamRepository) ListByTenant(ctx context.Context, tenantUUID string, includeDisabled bool) ([]model.AgentTeam, error) {
	q := r.db.WithContext(ctx).Model(&model.AgentTeam{}).
		Where("tenant_uuid = ?", strings.ToLower(strings.TrimSpace(tenantUUID)))
	if !includeDisabled {
		q = q.Where("status = ?", model.TeamStatusActive)
	}
	var rows []model.AgentTeam
	if err := q.Order("id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *AgentTeamRepository) UpdateStatus(ctx context.Context, teamID uint64, status string) error {
	return r.db.WithContext(ctx).Model(&model.AgentTeam{}).
		Where("id = ?", teamID).
		Updates(map[string]any{"status": strings.ToLower(strings.TrimSpace(status))}).Error
}

func (r *AgentTeamRepository) UpdateByID(ctx context.Context, teamID uint64, updates map[string]any) error {
	if len(updates) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Model(&model.AgentTeam{}).
		Where("id = ?", teamID).
		Updates(updates).Error
}

func (r *AgentTeamRepository) DeleteByID(ctx context.Context, teamID uint64) error {
	return r.db.WithContext(ctx).Where("id = ?", teamID).Delete(&model.AgentTeam{}).Error
}

type AgentTeamMemberRepository struct {
	*baseRepo.BaseRepository[model.AgentTeamMember]
	db *gorm.DB
}

func NewAgentTeamMemberRepository(db *gorm.DB) *AgentTeamMemberRepository {
	if db == nil {
		panic("agent team member repository requires db")
	}
	return &AgentTeamMemberRepository{BaseRepository: baseRepo.NewBaseRepository[model.AgentTeamMember](db), db: db}
}

func (r *AgentTeamMemberRepository) Upsert(ctx context.Context, rec *model.AgentTeamMember) (*model.AgentTeamMember, error) {
	if rec == nil {
		return nil, gorm.ErrInvalidData
	}
	rec.Normalize()
	return r.BaseRepository.Upsert(ctx, rec, []clause.Column{{Name: "team_id"}, {Name: "child_agent_id"}})
}

func (r *AgentTeamMemberRepository) ListEnabledByTeam(ctx context.Context, teamID uint64) ([]model.AgentTeamMember, error) {
	var rows []model.AgentTeamMember
	if err := r.db.WithContext(ctx).Where("team_id = ? AND enabled = ?", teamID, true).Order("priority ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *AgentTeamMemberRepository) DeleteByTeamChild(ctx context.Context, teamID uint64, childAgentID uint64) error {
	return r.db.WithContext(ctx).Where("team_id = ? AND child_agent_id = ?", teamID, childAgentID).Delete(&model.AgentTeamMember{}).Error
}

func (r *AgentTeamMemberRepository) DeleteByTeam(ctx context.Context, teamID uint64) error {
	return r.db.WithContext(ctx).Where("team_id = ?", teamID).Delete(&model.AgentTeamMember{}).Error
}

type AgentHandoffTaskRepository struct {
	*baseRepo.BaseRepository[model.AgentHandoffTask]
	db *gorm.DB
}

func NewAgentHandoffTaskRepository(db *gorm.DB) *AgentHandoffTaskRepository {
	if db == nil {
		panic("agent handoff task repository requires db")
	}
	return &AgentHandoffTaskRepository{BaseRepository: baseRepo.NewBaseRepository[model.AgentHandoffTask](db), db: db}
}

func (r *AgentHandoffTaskRepository) UpsertByTaskID(ctx context.Context, rec *model.AgentHandoffTask) (*model.AgentHandoffTask, error) {
	if rec == nil {
		return nil, gorm.ErrInvalidData
	}
	rec.Normalize()
	return r.BaseRepository.Upsert(ctx, rec, []clause.Column{{Name: "task_id"}})
}

func (r *AgentHandoffTaskRepository) MarkRunning(ctx context.Context, taskID string, handoffTraceID string) error {
	now := time.Now()
	return r.db.WithContext(ctx).Model(&model.AgentHandoffTask{}).
		Where("task_id = ?", strings.TrimSpace(taskID)).
		Updates(map[string]any{
			"status":           model.TaskStatusRunning,
			"started_at":       &now,
			"handoff_trace_id": strings.TrimSpace(handoffTraceID),
		}).Error
}

func (r *AgentHandoffTaskRepository) MarkFinished(ctx context.Context, taskID, status, outputDigest, errorCode, errorSummary string) error {
	now := time.Now()
	updates := map[string]any{
		"status":        strings.ToLower(strings.TrimSpace(status)),
		"output_digest": strings.TrimSpace(outputDigest),
		"error_code":    strings.TrimSpace(errorCode),
		"error_summary": strings.TrimSpace(errorSummary),
		"ended_at":      &now,
	}
	return r.db.WithContext(ctx).Model(&model.AgentHandoffTask{}).
		Where("task_id = ?", strings.TrimSpace(taskID)).
		Updates(updates).Error
}

func (r *AgentHandoffTaskRepository) DeleteByTeam(ctx context.Context, teamID uint64) error {
	return r.db.WithContext(ctx).Where("team_id = ?", teamID).Delete(&model.AgentHandoffTask{}).Error
}

type AgentSharedContextRefRepository struct {
	*baseRepo.BaseRepository[model.AgentSharedContextRef]
	db *gorm.DB
}

func NewAgentSharedContextRefRepository(db *gorm.DB) *AgentSharedContextRefRepository {
	if db == nil {
		panic("agent shared context ref repository requires db")
	}
	return &AgentSharedContextRefRepository{BaseRepository: baseRepo.NewBaseRepository[model.AgentSharedContextRef](db), db: db}
}

func (r *AgentSharedContextRefRepository) GetByRefID(ctx context.Context, refID string) (*model.AgentSharedContextRef, error) {
	var rec model.AgentSharedContextRef
	err := r.db.WithContext(ctx).Where("context_ref_id = ?", strings.TrimSpace(refID)).Take(&rec).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrContextRefNotFound
	}
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

func (r *AgentSharedContextRefRepository) IsAgentVisible(rec *model.AgentSharedContextRef, agentID uint64) bool {
	if rec == nil || agentID == 0 {
		return false
	}
	needle := strconv.FormatUint(agentID, 10)
	for _, raw := range strings.Split(rec.VisibleToAgentIDs, ",") {
		if strings.TrimSpace(raw) == needle {
			return true
		}
	}
	return false
}

func (r *AgentSharedContextRefRepository) ValidateAccess(ctx context.Context, tenantUUID string, childAgentID uint64, refID string, now time.Time) error {
	rec, err := r.GetByRefID(ctx, refID)
	if err != nil {
		return err
	}
	if !strings.EqualFold(strings.TrimSpace(rec.TenantUUID), strings.TrimSpace(tenantUUID)) {
		return fmt.Errorf("context_ref tenant mismatch")
	}
	if rec.ExpiresAt != nil && now.After(*rec.ExpiresAt) {
		return fmt.Errorf("context_ref expired")
	}
	if !r.IsAgentVisible(rec, childAgentID) {
		return fmt.Errorf("context_ref not visible to child agent")
	}
	return nil
}
