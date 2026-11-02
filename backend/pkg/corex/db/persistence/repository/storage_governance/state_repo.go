package storagegovrepo

import (
	"context"
	"errors"
	"time"

	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/storage_governance"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrRevisionConflict = errors.New("storage policy revision conflict")

// StateRepository 限定平台域 tenant_id=0 和明确实例，不提供跨租户通配查询。
type StateRepository struct {
	*repository.BaseRepository[m.Policy]
}

func NewStateRepository(db *gorm.DB) *StateRepository {
	return &StateRepository{repository.NewBaseRepository[m.Policy](db)}
}

func (r *StateRepository) GetPolicy(ctx context.Context, instance string) (*m.Policy, error) {
	var row m.Policy
	err := r.DB.WithContext(ctx).Where("tenant_id = ? AND instance_id = ?", 0, instance).First(&row).Error
	return &row, err
}

// SavePolicy 更新与修改审计原子提交；失败不能留下成功 revision。
func (r *StateRepository) SavePolicy(ctx context.Context, row *m.Policy, expected int64) error {
	return r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if expected == 0 {
			result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(row)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return ErrRevisionConflict
			}
		} else {
			result := tx.Model(&m.Policy{}).Where("tenant_id = ? AND instance_id = ? AND revision = ?", 0, row.InstanceID, expected).
				Updates(map[string]any{"revision": row.Revision, "schema_version": row.SchemaVersion, "document": row.Document, "updated_by": row.UpdatedBy})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return ErrRevisionConflict
			}
		}
		if err := tx.Create(&m.PolicyRevision{TenantID: 0, InstanceID: row.InstanceID, Revision: row.Revision, Document: row.Document, ActorID: row.UpdatedBy}).Error; err != nil {
			return err
		}
		// 仅清理本模块旧草稿审计元数据，绝不调用业务清理器。
		return tx.Unscoped().Where("tenant_id = ? AND instance_id = ? AND revision <= ?", 0, row.InstanceID, row.Revision-100).Delete(&m.PolicyRevision{}).Error
	})
}

func (r *StateRepository) SaveSample(ctx context.Context, row *m.Sample) error {
	return r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(row).Error; err != nil {
			return err
		}
		// 五分钟去重 + 15 天上限，包含当天未完成窗口；只删除观测元数据。
		return tx.Unscoped().Where("tenant_id = ? AND instance_id = ? AND slot < ?", 0, row.InstanceID, row.Slot.Add(-15*24*time.Hour)).Delete(&m.Sample{}).Error
	})
}

func (r *StateRepository) Samples(ctx context.Context, instance string, since time.Time) ([]m.Sample, error) {
	var rows []m.Sample
	err := r.DB.WithContext(ctx).Where("tenant_id = ? AND instance_id = ? AND slot >= ?", 0, instance, since).Order("slot ASC").Limit(4321).Find(&rows).Error
	return rows, err
}
