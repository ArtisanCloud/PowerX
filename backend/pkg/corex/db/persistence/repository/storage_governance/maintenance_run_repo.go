package storagegovrepo

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"time"

	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/storage_governance"
	"gorm.io/gorm"
)

// MaintenanceRunRepository 隔离当前安装的维护账本，不复用记录删除回执。
type MaintenanceRunRepository struct {
	db        *gorm.DB
	namespace string
}

func NewMaintenanceRunRepository(db *gorm.DB, scope string) *MaintenanceRunRepository {
	return &MaintenanceRunRepository{db, "db-maintenance:" + scope}
}
func (r *MaintenanceRunRepository) Lock(ctx context.Context, tx *gorm.DB) error {
	if tx.Dialector.Name() != "postgres" {
		return nil
	}
	sum := sha256.Sum256([]byte(r.namespace))
	return tx.WithContext(ctx).Exec("SELECT pg_advisory_xact_lock(?)", int64(binary.BigEndian.Uint64(sum[:8]))).Error
}
func (r *MaintenanceRunRepository) Get(ctx context.Context, tx *gorm.DB, id string) (*m.CleanupRun, error) {
	return NewCleanupRepository(tx).GetRun(ctx, r.namespace, id)
}
func (r *MaintenanceRunRepository) List(ctx context.Context) ([]m.CleanupRun, error) {
	var rows []m.CleanupRun
	e := r.db.WithContext(ctx).Where("tenant_id = ? AND instance_id = ? AND state NOT IN ?", 0, r.namespace, []string{"preview", "consumed"}).Order("id DESC").Limit(100).Find(&rows).Error
	return rows, e
}
func (r *MaintenanceRunRepository) Active(ctx context.Context, tx *gorm.DB) (*m.CleanupRun, error) {
	var row m.CleanupRun
	e := tx.WithContext(ctx).Where("tenant_id = ? AND instance_id = ? AND state IN ?", 0, r.namespace, []string{"queued", "running", "cancel_requested", "unconfirmed"}).Order("id ASC").First(&row).Error
	return &row, e
}
func (r *MaintenanceRunRepository) Create(ctx context.Context, tx *gorm.DB, row *m.CleanupRun) error {
	row.InstanceID = r.namespace
	row.TenantID = 0
	return tx.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 仅回收 30 天前的已知终态和失效检查；运行中及结果不明的意图永不自动清除。
		if e := tx.Unscoped().Where("tenant_id = ? AND instance_id = ? AND state IN ? AND updated_at < ?", 0, r.namespace, []string{"preview", "consumed", "completed", "failed", "cancelled", "reviewed"}, time.Now().UTC().Add(-30*24*time.Hour)).Delete(&m.CleanupRun{}).Error; e != nil {
			return e
		}
		return NewCleanupRepository(tx).CreateRun(ctx, row)
	})
}
func (r *MaintenanceRunRepository) Update(ctx context.Context, tx *gorm.DB, row *m.CleanupRun, expected int64) error {
	row.InstanceID = r.namespace
	return NewCleanupRepository(tx).UpdateRun(ctx, row, expected)
}
