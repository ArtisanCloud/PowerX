// pkg/corex/db/persistence/repository/iam/tenant_repo.go
package iam

import (
	"context"
	"errors"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	repository "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
)

type TenantRepository struct {
	*repository.BaseRepository[dbm.Tenant]
	db *gorm.DB
}

func NewTenantRepository(db *gorm.DB) *TenantRepository {
	return &TenantRepository{
		BaseRepository: repository.NewBaseRepository[dbm.Tenant](db),
		db:             db,
	}
}

func (r *TenantRepository) GetByID(ctx context.Context, id uint64) (*dbm.Tenant, error) {
	var t dbm.Tenant
	if err := r.db.WithContext(ctx).First(&t, id).Error; err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *TenantRepository) GetByKey(ctx context.Context, key string) (*dbm.Tenant, error) {
	var t dbm.Tenant
	if err := r.db.WithContext(ctx).Where("key = ?", key).First(&t).Error; err != nil {
		return nil, err
	}
	return &t, nil
}

// EnsureByKey 并发安全地确保存在指定 key 的租户：
// - 存在：直接返回
// - 不存在：插入（ON CONFLICT DO NOTHING），随后再读一遍拿到 ID
func (r *TenantRepository) EnsureByKey(ctx context.Context, key, name string) (*dbm.Tenant, error) {
	// 先查
	if t, err := r.GetByKey(ctx, key); err == nil && t != nil {
		return t, nil
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	// 竞态安全地尝试创建
	toCreate := &dbm.Tenant{
		Key:    key,
		Name:   name,
		Status: 1,
	}
	if err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "key"}}, // 依赖 tenant.key 的唯一索引
			DoNothing: true,
		}).
		Create(toCreate).Error; err != nil {
		return nil, err
	}

	// 无论是我们创建还是别人抢先创建，这里再读一遍拿到最终记录（含 ID）
	return r.GetByKey(ctx, key)
}

// EnsureSystemTenant 语义化封装，复用 EnsureByKey
func (r *TenantRepository) EnsureSystemTenant(ctx context.Context) (*dbm.Tenant, error) {
	return r.EnsureByKey(ctx, dbm.SystemTenantKey, "System")
}

// MapNamesByIDs 批量查询租户名称
func (r *TenantRepository) MapNamesByIDs(ctx context.Context, ids []uint64) (map[uint64]string, error) {
	mm := make(map[uint64]string, len(ids))
	if len(ids) == 0 {
		return mm, nil
	}
	var ts []dbm.Tenant
	if err := r.DB.WithContext(ctx).
		Table(model.TableIAMTenant).
		Where("id IN ?", ids).
		Find(&ts).Error; err != nil {
		return mm, err
	}
	for _, t := range ts {
		mm[t.ID] = t.Name
	}
	return mm, nil
}
