package repository

import (
	"context"
	"strings"

	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	coreRepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type AgentAccessGrantRepository struct {
	*coreRepo.BaseRepository[dbmodel.AgentAccessGrant]
	db *gorm.DB
}

func NewAgentAccessGrantRepository(db *gorm.DB) *AgentAccessGrantRepository {
	return &AgentAccessGrantRepository{
		BaseRepository: coreRepo.NewBaseRepository[dbmodel.AgentAccessGrant](db),
		db:             db,
	}
}

func (r *AgentAccessGrantRepository) ListByAgent(ctx context.Context, env, tenantUUID string, agentUUID uuid.UUID, subjectType string) ([]dbmodel.AgentAccessGrant, error) {
	var rows []dbmodel.AgentAccessGrant
	q := r.db.WithContext(ctx).
		Where("env = ? AND tenant_uuid = ? AND agent_uuid = ?", strings.TrimSpace(env), strings.TrimSpace(tenantUUID), agentUUID)
	if strings.TrimSpace(subjectType) != "" {
		q = q.Where("subject_type = ?", strings.TrimSpace(subjectType))
	}
	err := q.Order("subject_type ASC, subject_uuid ASC").Find(&rows).Error
	return rows, err
}

func (r *AgentAccessGrantRepository) UpsertByAgent(ctx context.Context, env, tenantUUID string, agentUUID uuid.UUID, rows []dbmodel.AgentAccessGrant) error {
	env = strings.TrimSpace(env)
	tenantUUID = strings.TrimSpace(tenantUUID)
	if len(rows) == 0 {
		return nil
	}
	for i := range rows {
		rows[i].Env = env
		rows[i].TenantUUID = tenantUUID
		rows[i].AgentUUID = agentUUID
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "env"},
			{Name: "tenant_uuid"},
			{Name: "agent_uuid"},
			{Name: "subject_type"},
			{Name: "subject_uuid"},
		},
		DoUpdates: clause.AssignmentColumns([]string{
			"status",
			"source",
			"updated_by_user_uuid",
			"updated_at",
		}),
	}).Create(&rows).Error
}

func (r *AgentAccessGrantRepository) HasEnabledForSubjects(ctx context.Context, env, tenantUUID string, agentUUID uuid.UUID, subjectUUIDsByType map[string][]string) (bool, error) {
	pairs := make([][]string, 0, len(subjectUUIDsByType))
	args := make([]any, 0, len(subjectUUIDsByType)*2)
	for subjectType, subjectUUIDs := range subjectUUIDsByType {
		clean := make([]string, 0, len(subjectUUIDs))
		for _, subjectUUID := range subjectUUIDs {
			if trimmed := strings.TrimSpace(subjectUUID); trimmed != "" {
				clean = append(clean, trimmed)
			}
		}
		if len(clean) == 0 {
			continue
		}
		pairs = append(pairs, []string{"subject_type = ? AND subject_uuid IN ?"})
		args = append(args, strings.TrimSpace(subjectType), clean)
	}
	if len(pairs) == 0 {
		return false, nil
	}
	subjectWhere := make([]string, 0, len(pairs))
	for range pairs {
		subjectWhere = append(subjectWhere, "(subject_type = ? AND subject_uuid IN ?)")
	}
	var count int64
	err := r.db.WithContext(ctx).
		Model(&dbmodel.AgentAccessGrant{}).
		Where("env = ? AND tenant_uuid = ? AND agent_uuid = ? AND status = ?", strings.TrimSpace(env), strings.TrimSpace(tenantUUID), agentUUID, dbmodel.AgentAccessGrantStatusEnabled).
		Where(strings.Join(subjectWhere, " OR "), args...).
		Count(&count).Error
	return count > 0, err
}
