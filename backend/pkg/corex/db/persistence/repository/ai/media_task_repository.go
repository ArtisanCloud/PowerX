package ai

import (
	"context"
	"errors"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ai"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrIdempotencyConflict = errors.New("ai.media.idempotency_conflict")

type MediaTaskRepository struct {
	*repository.BaseRepository[m.MediaTask]
}

func NewMediaTaskRepository(db *gorm.DB) *MediaTaskRepository {
	return &MediaTaskRepository{repository.NewBaseRepository[m.MediaTask](db)}
}

func (r *MediaTaskRepository) FindRequest(ctx context.Context, tenant, subject, capability, key string) (*m.MediaTask, error) {
	var row m.MediaTask
	err := r.DB.WithContext(ctx).Where("tenant_uuid = ? AND subject = ? AND capability = ? AND idempotency_key = ?", tenant, subject, capability, key).First(&row).Error
	return &row, err
}
func (r *MediaTaskRepository) WithTx(ctx context.Context, fn func(*gorm.DB) error) error {
	return r.DB.WithContext(ctx).Transaction(fn)
}
func (r *MediaTaskRepository) Create(ctx context.Context, tx *gorm.DB, in *m.MediaTask) (*m.MediaTask, bool, error) {
	result := tx.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "tenant_uuid"}, {Name: "subject"}, {Name: "capability"}, {Name: "idempotency_key"}}, DoNothing: true}).Create(in)
	if result.Error != nil {
		return nil, false, result.Error
	}
	if result.RowsAffected == 1 {
		return in, true, nil
	}
	var current m.MediaTask
	err := tx.WithContext(ctx).Where("tenant_uuid = ? AND subject = ? AND capability = ? AND idempotency_key = ?", in.TenantUUID, in.Subject, in.Capability, in.IdempotencyKey).First(&current).Error
	if err == nil && current.RequestDigest != in.RequestDigest {
		return nil, false, ErrIdempotencyConflict
	}
	return &current, false, err
}
func (r *MediaTaskRepository) Get(ctx context.Context, tenant, id, subject string) (*m.MediaTask, error) {
	var out m.MediaTask
	q := r.DB.WithContext(ctx).Where("tenant_uuid = ? AND uuid = ?", tenant, id)
	if subject != "" {
		q = q.Where("subject = ?", subject)
	}
	err := q.First(&out).Error
	return &out, err
}
func (r *MediaTaskRepository) Update(ctx context.Context, tx *gorm.DB, tenant, id string, apply func(*m.MediaTask) error) error {
	if tx == nil {
		return r.WithTx(ctx, func(tx *gorm.DB) error { return r.Update(ctx, tx, tenant, id, apply) })
	}
	var row m.MediaTask
	if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_uuid = ? AND uuid = ?", tenant, id).First(&row).Error; err != nil {
		return err
	}
	if err := apply(&row); err != nil {
		return err
	}
	row.Revision++
	return tx.WithContext(ctx).Save(&row).Error
}
