package agent

import (
	"context"
	"errors"
	"time"

	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/agent"
	base "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type SessionOwner struct {
	TenantUUID             uuid.UUID
	PluginID, ServiceActor string
}
type ServiceSessionRepository struct {
	*base.BaseRepository[m.ServiceSession]
	db *gorm.DB
}

func NewServiceSessionRepository(db *gorm.DB) *ServiceSessionRepository {
	return &ServiceSessionRepository{BaseRepository: base.NewBaseRepository[m.ServiceSession](db), db: db}
}
func (r *ServiceSessionRepository) scoped(ctx context.Context, owner SessionOwner) *gorm.DB {
	q := r.db.WithContext(ctx).Where("tenant_uuid = ? AND plugin_id = ? AND service_actor = ?", owner.TenantUUID, owner.PluginID, owner.ServiceActor)
	if owner.TenantUUID == uuid.Nil || owner.PluginID == "" || owner.ServiceActor == "" {
		q.AddError(errors.New("agent_session.owner_required"))
	}
	return q
}
func (r *ServiceSessionRepository) CreateSession(ctx context.Context, owner SessionOwner, session *m.ServiceSession) error {
	if owner.TenantUUID == uuid.Nil || owner.PluginID == "" || owner.ServiceActor == "" {
		return errors.New("agent_session.owner_required")
	}
	session.TenantUUID, session.PluginID, session.ServiceActor = owner.TenantUUID, owner.PluginID, owner.ServiceActor
	return r.db.WithContext(ctx).Create(session).Error
}
func (r *ServiceSessionRepository) GetSession(ctx context.Context, owner SessionOwner, id uuid.UUID) (*m.ServiceSession, error) {
	var session m.ServiceSession
	err := r.scoped(ctx, owner).Where("uuid = ? AND status <> ?", id, "deleted").First(&session).Error
	return &session, err
}
func (r *ServiceSessionRepository) ListSessions(ctx context.Context, owner SessionOwner, page, size int) ([]m.ServiceSession, int64, error) {
	var total int64
	var sessions []m.ServiceSession
	q := r.scoped(ctx, owner).Model(&m.ServiceSession{}).Where("status <> ?", "deleted")
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := q.Order("created_at DESC, uuid DESC").Offset((page - 1) * size).Limit(size).Find(&sessions).Error
	return sessions, total, err
}

// WithSession serializes writes and checks ownership before loading children.
func (r *ServiceSessionRepository) WithSession(ctx context.Context, owner SessionOwner, id uuid.UUID, fn func(*ServiceSessionRepository, *m.ServiceSession) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		repo := NewServiceSessionRepository(tx)
		var session m.ServiceSession
		if err := repo.scoped(ctx, owner).Clauses(clause.Locking{Strength: "UPDATE"}).Where("uuid = ?", id).First(&session).Error; err != nil {
			return err
		}
		return fn(repo, &session)
	})
}
func (r *ServiceSessionRepository) SaveSession(ctx context.Context, owner SessionOwner, session *m.ServiceSession) error {
	session.UpdatedAt = time.Now().UTC()
	return r.scoped(ctx, owner).Model(&m.ServiceSession{}).Where("uuid = ?", session.UUID).Updates(map[string]any{"title": session.Title, "status": session.Status, "revision": session.Revision, "updated_at": session.UpdatedAt, "pending_task": session.PendingTask, "pending_task_expires_at": session.PendingTaskExpiresAt}).Error
}

// ExecutionHistory is read under the parent session lock, before the selected
// message. Later messages can never change an already accepted invocation.
func (r *ServiceSessionRepository) ExecutionHistory(ctx context.Context, session *m.ServiceSession, before uint64, limit int) ([]m.ServiceMessage, error) {
	var rows []m.ServiceMessage
	err := r.db.WithContext(ctx).Where("tenant_uuid = ? AND session_uuid = ? AND sequence < ?", session.TenantUUID, session.UUID, before).Order("sequence ASC").Limit(limit).Find(&rows).Error
	return rows, err
}
func (r *ServiceSessionRepository) MessageByKey(ctx context.Context, session *m.ServiceSession, key string) (*m.ServiceMessage, error) {
	var message m.ServiceMessage
	err := r.db.WithContext(ctx).Where("tenant_uuid = ? AND session_uuid = ? AND idempotency_key = ?", session.TenantUUID, session.UUID, key).First(&message).Error
	return &message, err
}
func (r *ServiceSessionRepository) InsertMessage(ctx context.Context, session *m.ServiceSession, message *m.ServiceMessage) error {
	message.TenantUUID, message.SessionUUID, message.AgentUUID = session.TenantUUID, session.UUID, session.AgentUUID
	return r.db.WithContext(ctx).Create(message).Error
}
func (r *ServiceSessionRepository) ListMessages(ctx context.Context, owner SessionOwner, id uuid.UUID, page, size int) ([]m.ServiceMessage, int64, error) {
	if _, err := r.GetSession(ctx, owner, id); err != nil {
		return nil, 0, err
	}
	var messages []m.ServiceMessage
	var total int64
	q := r.db.WithContext(ctx).Model(&m.ServiceMessage{}).Where("tenant_uuid = ? AND session_uuid = ?", owner.TenantUUID, id)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := q.Order("sequence ASC").Offset((page - 1) * size).Limit(size).Find(&messages).Error
	return messages, total, err
}
