package eventfabric

import (
	"context"
	"errors"

	eventfabricmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	baserepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ReplayRepository 管理事件回放任务。
type ReplayRepository struct {
	base *baserepo.BaseRepository[eventfabricmodel.ReplayRequest]
	db   *gorm.DB
}

func NewReplayRepository(db *gorm.DB) *ReplayRepository {
	return &ReplayRepository{
		base: baserepo.NewBaseRepository[eventfabricmodel.ReplayRequest](db),
		db:   db,
	}
}

func (r *ReplayRepository) WithDB(db *gorm.DB) *ReplayRepository {
	if db == nil {
		return r
	}
	return NewReplayRepository(db)
}

func (r *ReplayRepository) Create(ctx context.Context, req *eventfabricmodel.ReplayRequest) (*eventfabricmodel.ReplayRequest, error) {
	return r.base.Create(ctx, req)
}

func (r *ReplayRepository) FindByUUID(ctx context.Context, id uuid.UUID) (*eventfabricmodel.ReplayRequest, error) {
	var record eventfabricmodel.ReplayRequest
	err := r.db.WithContext(ctx).Where("uuid = ?", id).Take(&record).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &record, nil
}

func (r *ReplayRepository) UpdateStatus(ctx context.Context, id uuid.UUID, updates map[string]interface{}) error {
	if len(updates) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Model(&eventfabricmodel.ReplayRequest{}).Where("uuid = ?", id).Updates(updates).Error
}

func (r *ReplayRepository) ListByStatus(ctx context.Context, tenantKey string, status string) ([]*eventfabricmodel.ReplayRequest, error) {
	query := r.db.WithContext(ctx).Where("tenant_key = ?", tenantKey)
	if status != "" {
		query = query.Where("status = ?", status)
	}
	var records []*eventfabricmodel.ReplayRequest
	if err := query.Order("submitted_at DESC").Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}
