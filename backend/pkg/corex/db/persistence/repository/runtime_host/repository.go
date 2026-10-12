package runtime_host

import (
	"context"
	"errors"

	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/runtime_host"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrConflict = errors.New("runtime_host.conflict")

type Repository struct {
	*repository.BaseRepository[m.Task]
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{repository.NewBaseRepository[m.Task](db)}
}
func (r *Repository) WithTx(ctx context.Context, fn func(*gorm.DB) error) error {
	if r == nil || r.DB == nil {
		return errors.New("runtime_host.database_unavailable")
	}
	return r.DB.WithContext(ctx).Transaction(fn)
}
func (r *Repository) Audit(ctx context.Context, tx *gorm.DB, tenant, subject uuid.UUID, task *uuid.UUID, action, outcome, key string) error {
	if tx == nil {
		tx = r.DB
	}
	if tx == nil {
		return errors.New("runtime_host.database_unavailable")
	}
	return tx.WithContext(ctx).Create(&m.Operation{TenantUUID: tenant, CallerSubjectUUID: subject, TaskUUID: task, Action: action, Outcome: outcome, KeyDigest: key, RequestID: RequestID(ctx)}).Error
}

type requestKey struct{}

func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestKey{}, id)
}
func RequestID(ctx context.Context) string {
	value, _ := ctx.Value(requestKey{}).(string)
	return value
}
func (r *Repository) Subject(ctx context.Context, tenant uuid.UUID, subject string) (uuid.UUID, error) {
	if r == nil || r.DB == nil {
		return uuid.Nil, errors.New("runtime_host.database_unavailable")
	}
	row := m.Subject{TenantUUID: tenant, CredentialSubject: subject}
	err := r.DB.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "tenant_uuid"}, {Name: "credential_subject"}}, DoNothing: true}).Create(&row).Error
	if err != nil {
		return uuid.Nil, err
	}
	err = r.DB.WithContext(ctx).Where("tenant_uuid = ? AND credential_subject = ?", tenant, subject).First(&row).Error
	return row.UUID, err
}
func (r *Repository) CreateTask(ctx context.Context, tx *gorm.DB, row *m.Task) (*m.Task, bool, error) {
	result := tx.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "tenant_uuid"}, {Name: "caller_subject_uuid"}, {Name: "idempotency_key"}}, DoNothing: true}).Create(row)
	if result.Error != nil {
		return nil, false, result.Error
	}
	if result.RowsAffected == 1 {
		return row, true, nil
	}
	var current m.Task
	err := tx.WithContext(ctx).Where("tenant_uuid = ? AND caller_subject_uuid = ? AND idempotency_key = ?", row.TenantUUID, row.CallerSubjectUUID, row.IdempotencyKey).First(&current).Error
	return &current, false, err
}
func (r *Repository) GetTask(ctx context.Context, tx *gorm.DB, tenant, subject, id uuid.UUID) (*m.Task, error) {
	if tx == nil {
		tx = r.DB
	}
	if tx == nil {
		return nil, errors.New("runtime_host.database_unavailable")
	}
	var row m.Task
	err := tx.WithContext(ctx).Where("tenant_uuid = ? AND caller_subject_uuid = ? AND uuid = ?", tenant, subject, id).First(&row).Error
	return &row, err
}

// UpdateTask 在与读取相同的事务中使用原版本及归属作 CAS，不包含任何可写归属字段。
func (r *Repository) UpdateTask(ctx context.Context, tx *gorm.DB, row *m.Task, expected int64) error {
	result := tx.WithContext(ctx).Model(&m.Task{}).Where("tenant_uuid = ? AND caller_subject_uuid = ? AND uuid = ? AND revision = ?", row.TenantUUID, row.CallerSubjectUUID, row.UUID, expected).Updates(map[string]any{
		"state": row.State, "progress": row.Progress, "revision": row.Revision, "message_key": row.MessageKey, "result": row.Result, "completed_at": row.CompletedAt, "updated_at": row.UpdatedAt,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrConflict
	}
	return nil
}
