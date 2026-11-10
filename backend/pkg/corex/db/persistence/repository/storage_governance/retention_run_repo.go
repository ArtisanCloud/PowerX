package storagegovrepo

import (
	"context"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/storage_governance"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"gorm.io/gorm"
	"time"
)

// RetentionRunRepository 复用治理账本；固定 tenant_id=0、域命名空间及显式 scope，事务由 Service 传入。
type RetentionRunRepository struct {
	*repository.BaseRepository[m.CleanupRun]
	instance string
}

func NewRetentionRunRepository(db *gorm.DB, scope string) *RetentionRunRepository {
	return &RetentionRunRepository{repository.NewBaseRepository[m.CleanupRun](db), "db-retention:" + scope}
}
func (r *RetentionRunRepository) Get(ctx context.Context, tx *gorm.DB, id string) (*m.CleanupRun, error) {
	var out m.CleanupRun
	e := tx.WithContext(ctx).Where("tenant_id = ? AND instance_id = ? AND run_id = ?", 0, r.instance, id).First(&out).Error
	return &out, e
}
func (r *RetentionRunRepository) Create(ctx context.Context, tx *gorm.DB, row *m.CleanupRun) error {
	row.TenantID = 0
	row.InstanceID = r.instance
	return tx.WithContext(ctx).Create(row).Error
}
func (r *RetentionRunRepository) List(ctx context.Context) ([]m.CleanupRun, error) {
	var out []m.CleanupRun
	e := r.DB.WithContext(ctx).Where("tenant_id = ? AND instance_id = ? AND state = ?", 0, r.instance, "completed").Order("id DESC").Limit(100).Find(&out).Error
	return out, e
}

// Trim 每次最多回收 500 条过期预览／30 天终态回执，硬删仅为该域的治理元数据。
func (r *RetentionRunRepository) Trim(ctx context.Context, tx *gorm.DB, now time.Time) error {
	var ids []uint64
	if e := tx.WithContext(ctx).Model(&m.CleanupRun{}).Where("tenant_id = ? AND instance_id = ? AND ((state IN ? AND expires_at < ?) OR (state = ? AND created_at < ?))", 0, r.instance, []string{"preview", "consumed"}, now, "completed", now.AddDate(0, 0, -30)).Order("id").Limit(500).Pluck("id", &ids).Error; e != nil {
		return e
	}
	if len(ids) > 0 {
		if e := tx.Unscoped().Where("tenant_id = ? AND instance_id = ? AND id IN ?", 0, r.instance, ids).Delete(&m.CleanupRun{}).Error; e != nil {
			return e
		}
	}
	var n int64
	if e := tx.Model(&m.CleanupRun{}).Where("tenant_id = ? AND instance_id = ?", 0, r.instance).Count(&n).Error; e != nil {
		return e
	}
	if n >= 200000 {
		return ErrRevisionConflict
	}
	return nil
}

// Ready 只验证既有集中迁移，不在运行时补建表或索引。
func (r *RetentionRunRepository) Ready(ctx context.Context) bool {
	db := r.DB.WithContext(ctx)
	return db.Migrator().HasTable(&m.CleanupRun{}) && db.Migrator().HasIndex((&m.CleanupRun{}).TableName(), "uk_storage_cleanup_run") && db.Migrator().HasColumn(&m.CleanupRun{}, "expires_at")
}

// Consume 仅消费同域、同操作者且尚未消费的预览；与清理和回执共用事务。
func (r *RetentionRunRepository) Consume(ctx context.Context, tx *gorm.DB, id string, actor uint64) error {
	out := tx.WithContext(ctx).Model(&m.CleanupRun{}).Where("tenant_id = ? AND instance_id = ? AND run_id = ? AND actor_id = ? AND state = ?", 0, r.instance, id, actor, "preview").Update("state", "consumed")
	if out.Error != nil {
		return out.Error
	}
	if out.RowsAffected != 1 {
		return ErrRevisionConflict
	}
	return nil
}

// NewSystemRetentionRunRepository 按固定适配器隔离治理账本，不能跨表消费预览或回执。
func NewSystemRetentionRunRepository(db *gorm.DB, scope, id string) *RetentionRunRepository {
	if !SystemRetentionSupported(id) {
		return nil
	}
	return &RetentionRunRepository{repository.NewBaseRepository[m.CleanupRun](db), "sys-retention:" + scope + ":" + id}
}
