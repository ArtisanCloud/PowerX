package agent

import (
	"context"
	"time"

	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/agent"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ListDurableLocators 仅供受信任的 Core 恢复扫描器跨租户读取低频锚点。
func (r *ServiceSessionRepository) ListDurableLocators(ctx context.Context, env string, after uint64, limit int, includeArchives ...bool) ([]m.ServiceInvocation, error) {
	if limit < 1 || limit > 200 {
		return nil, gorm.ErrInvalidData
	}
	var rows []m.ServiceInvocation
	q := r.db.WithContext(ctx).Where("run_env = ? AND id > ? AND admission_state IN ?", env, after, []string{"pending_create", "admitted"})
	if len(includeArchives) > 0 && includeArchives[0] {
		if len(includeArchives) > 1 && includeArchives[1] {
			q = q.Where("finished_at IS NULL OR archived_at IS NULL OR hot_expires_at IS NULL")
		} else {
			q = q.Where("finished_at IS NULL OR archived_at IS NULL")
		}
	} else {
		q = q.Where("finished_at IS NULL")
	}
	err := q.Order("id ASC").Limit(limit).Find(&rows).Error
	return rows, err
}

func (r *ServiceSessionRepository) DurableInvocation(ctx context.Context, tenant, id uuid.UUID) (*m.ServiceInvocation, error) {
	var row m.ServiceInvocation
	err := r.db.WithContext(ctx).Where("tenant_uuid = ? AND uuid = ? AND admission_state = ?", tenant, id, "admitted").First(&row).Error
	return &row, err
}

// FinishDurableInvocation 必须在父会话事务锁内调用。
func (r *ServiceSessionRepository) FinishDurableInvocation(ctx context.Context, owner SessionOwner, run *m.ServiceInvocation) error {
	result := r.scoped(ctx, owner).Model(&m.ServiceInvocation{}).Where("session_uuid = ? AND uuid = ? AND admission_state = ? AND finished_at IS NULL", run.SessionUUID, run.UUID, "admitted").Updates(map[string]any{
		"status": run.Status, "reason_code": run.ReasonCode, "output": run.Output, "response_envelope": run.ResponseEnvelope, "finished_at": run.FinishedAt,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *ServiceSessionRepository) Message(ctx context.Context, session *m.ServiceSession, id uuid.UUID) (*m.ServiceMessage, error) {
	var message m.ServiceMessage
	err := r.db.WithContext(ctx).Where("tenant_uuid = ? AND session_uuid = ? AND uuid = ?", session.TenantUUID, session.UUID, id).First(&message).Error
	return &message, err
}
func (r *ServiceSessionRepository) InvocationByKey(ctx context.Context, session *m.ServiceSession, key string) (*m.ServiceInvocation, error) {
	var run m.ServiceInvocation
	err := r.db.WithContext(ctx).Where("tenant_uuid = ? AND session_uuid = ? AND idempotency_key = ?", session.TenantUUID, session.UUID, key).First(&run).Error
	return &run, err
}
func (r *ServiceSessionRepository) ActiveInvocations(ctx context.Context, session *m.ServiceSession) (int64, error) {
	var total int64
	err := r.db.WithContext(ctx).Model(&m.ServiceInvocation{}).Where("tenant_uuid = ? AND session_uuid = ? AND status IN ?", session.TenantUUID, session.UUID, []string{"accepted", "planning", "running", "cancelling"}).Count(&total).Error
	return total, err
}

// ExpireInvocations must run under the parent session lock. A process restart
// must not leave an orphaned execution blocking the session indefinitely.
// Expiration records failure; it never retries an execution automatically.
func (r *ServiceSessionRepository) ExpireInvocations(ctx context.Context, session *m.ServiceSession, now time.Time) error {
	return r.db.WithContext(ctx).Model(&m.ServiceInvocation{}).
		Where("tenant_uuid = ? AND session_uuid = ? AND status IN ? AND deadline_at <= ?", session.TenantUUID, session.UUID, []string{"accepted", "planning", "running", "cancelling"}, now).
		Where("admission_state IS NULL OR admission_state = ?", "").
		Updates(map[string]any{"status": "failed", "reason_code": "AGENT_SESSION_EXECUTION_EXPIRED", "output": "", "finished_at": now.UTC()}).Error
}
func (r *ServiceSessionRepository) InsertInvocation(ctx context.Context, owner SessionOwner, session *m.ServiceSession, run *m.ServiceInvocation) error {
	run.TenantUUID, run.SessionUUID, run.AgentUUID = owner.TenantUUID, session.UUID, session.AgentUUID
	run.PluginID, run.ServiceActor = owner.PluginID, owner.ServiceActor
	return r.db.WithContext(ctx).Create(run).Error
}
func (r *ServiceSessionRepository) MarkInvocationAdmitted(ctx context.Context, owner SessionOwner, sessionID, id uuid.UUID) error {
	result := r.scoped(ctx, owner).Model(&m.ServiceInvocation{}).
		Where("session_uuid = ? AND uuid = ? AND admission_state = ?", sessionID, id, "pending_create").
		Update("admission_state", "admitted")
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
func (r *ServiceSessionRepository) Invocation(ctx context.Context, owner SessionOwner, sessionID, id uuid.UUID) (*m.ServiceInvocation, error) {
	var run m.ServiceInvocation
	err := r.scoped(ctx, owner).Where("session_uuid = ? AND uuid = ?", sessionID, id).First(&run).Error
	return &run, err
}
func (r *ServiceSessionRepository) CancelInvocation(ctx context.Context, owner SessionOwner, sessionID, id uuid.UUID) error {
	return r.scoped(ctx, owner).Model(&m.ServiceInvocation{}).Where("session_uuid = ? AND uuid = ? AND status = ?", sessionID, id, "running").Updates(map[string]any{"status": "cancelling", "cancel_requested_at": time.Now().UTC()}).Error
}
func (r *ServiceSessionRepository) FinishInvocation(ctx context.Context, owner SessionOwner, run *m.ServiceInvocation, status, reason, output string) error {
	return r.scoped(ctx, owner).Model(&m.ServiceInvocation{}).Where("session_uuid = ? AND uuid = ? AND status IN ?", run.SessionUUID, run.UUID, []string{"running", "cancelling"}).Updates(map[string]any{"status": status, "reason_code": reason, "output": output, "finished_at": time.Now().UTC()}).Error
}
