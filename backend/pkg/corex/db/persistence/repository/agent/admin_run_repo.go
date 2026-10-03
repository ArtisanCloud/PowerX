package agent

import (
	"context"
	"errors"
	"time"

	chatmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/agent"
	iam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// AdminRunRepository 所有交互查询显式带 tenant/env；扫描仅由受信任恢复器调用。
type AdminRunRepository struct{ db *gorm.DB }

func NewAdminRunRepository(db *gorm.DB) *AdminRunRepository { return &AdminRunRepository{db: db} }
func (r *AdminRunRepository) Get(ctx context.Context, tenant uuid.UUID, env string, id uuid.UUID) (*m.AdminRunAdmission, error) {
	var row m.AdminRunAdmission
	err := r.db.WithContext(ctx).Where("tenant_uuid = ? AND env = ? AND uuid = ?", tenant, env, id).First(&row).Error
	return &row, err
}
func (r *AdminRunRepository) ByKey(ctx context.Context, tenant uuid.UUID, env string, session uuid.UUID, key string) (*m.AdminRunAdmission, error) {
	var row m.AdminRunAdmission
	err := r.db.WithContext(ctx).Where("tenant_uuid = ? AND env = ? AND session_uuid = ? AND idempotency_key = ?", tenant, env, session, key).First(&row).Error
	return &row, err
}
func (r *AdminRunRepository) WithSession(ctx context.Context, tenant uuid.UUID, env string, id uuid.UUID, fn func(*AdminRunRepository, *chatmodel.AgentChatSession) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var session chatmodel.AgentChatSession
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_uuid = ? AND env = ? AND uuid = ?", tenant, env, id).First(&session).Error; err != nil {
			return err
		}
		return fn(NewAdminRunRepository(tx), &session)
	})
}
func (r *AdminRunRepository) Message(ctx context.Context, tenant uuid.UUID, env string, id uuid.UUID) (*chatmodel.AgentChatMessage, error) {
	var row chatmodel.AgentChatMessage
	err := r.db.WithContext(ctx).Where("tenant_uuid = ? AND env = ? AND uuid = ?", tenant, env, id).First(&row).Error
	return &row, err
}
func (r *AdminRunRepository) Actor(ctx context.Context, tenant uuid.UUID, user uuid.UUID, member *uuid.UUID) (*iam.User, *iam.Member, error) {
	var actor iam.User
	if err := r.db.WithContext(ctx).Where("uuid = ? AND status = ?", user, iam.UserStatusActive).First(&actor).Error; err != nil {
		return nil, nil, err
	}
	if member == nil {
		if actor.IsRoot {
			return &actor, nil, nil
		}
		return nil, nil, gorm.ErrRecordNotFound
	}
	var membership iam.Member
	if err := r.db.WithContext(ctx).Where("uuid = ? AND tenant_uuid = ? AND user_id = ? AND status = 1", *member, tenant, actor.ID).First(&membership).Error; err != nil {
		return nil, nil, err
	}
	return &actor, &membership, nil
}
func (r *AdminRunRepository) Active(ctx context.Context, tenant uuid.UUID, env string, session uuid.UUID) (bool, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&m.AdminRunAdmission{}).Where("tenant_uuid = ? AND env = ? AND session_uuid = ? AND finished_at IS NULL", tenant, env, session).Count(&n).Error
	return n > 0, err
}
func (r *AdminRunRepository) Insert(ctx context.Context, row *m.AdminRunAdmission) error {
	return r.db.WithContext(ctx).Create(row).Error
}
func (r *AdminRunRepository) MarkAdmitted(ctx context.Context, row *m.AdminRunAdmission) error {
	result := r.db.WithContext(ctx).Model(&m.AdminRunAdmission{}).Where("tenant_uuid = ? AND env = ? AND uuid = ? AND admission_state = ?", row.TenantUUID, row.Env, row.UUID, "pending_create").Update("admission_state", "admitted")
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
func (r *AdminRunRepository) ListUnfinished(ctx context.Context, env string, after uint64, limit int, includeArchives ...bool) ([]m.AdminRunAdmission, error) {
	if limit < 1 || limit > 200 {
		return nil, gorm.ErrInvalidData
	}
	var rows []m.AdminRunAdmission
	q := r.db.WithContext(ctx).Where("env = ? AND id > ? AND admission_state IN ?", env, after, []string{"pending_create", "admitted"})
	if len(includeArchives) > 0 && includeArchives[0] {
		q = q.Where("finished_at IS NULL OR archived_at IS NULL")
	} else {
		q = q.Where("finished_at IS NULL")
	}
	err := q.Order("id ASC").Limit(limit).Find(&rows).Error
	return rows, err
}

// Finish 仅在 WithSession 事务锁内调用；稳定消息 UUID 防止事务重试重复插入。
func (r *AdminRunRepository) Finish(ctx context.Context, row *m.AdminRunAdmission, session *chatmodel.AgentChatSession, message *chatmodel.AgentChatMessage, at time.Time, status string) error {
	if message != nil && session.Status != "deleted" {
		if err := r.db.WithContext(ctx).Create(message).Error; err != nil {
			return err
		}
		row.AssistantMessageUUID = &message.UUID
		if err := r.db.WithContext(ctx).Model(&chatmodel.AgentChatSession{}).Where("tenant_uuid = ? AND env = ? AND id = ?", row.TenantUUID, row.Env, session.ID).Updates(map[string]any{"latest_at": at, "updated_at": at}).Error; err != nil {
			return err
		}
	}
	result := r.db.WithContext(ctx).Model(&m.AdminRunAdmission{}).Where("tenant_uuid = ? AND env = ? AND uuid = ? AND finished_at IS NULL", row.TenantUUID, row.Env, row.UUID).Updates(map[string]any{"status": status, "finished_at": at, "assistant_message_uuid": row.AssistantMessageUUID})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("admin run already finalized")
	}
	return nil
}

func (r *AdminRunRepository) Session(ctx context.Context, tenant uuid.UUID, env string, id uuid.UUID) (*chatmodel.AgentChatSession, error) {
	var row chatmodel.AgentChatSession
	err := r.db.WithContext(ctx).Where("tenant_uuid = ? AND env = ? AND uuid = ?", tenant, env, id).First(&row).Error
	return &row, err
}
