package ops

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"

	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// LockControlCatalog 覆盖未配置行的跨安装接管及唯一当前计划竞争；调用方按 control→catalog 取锁。
func (r *BackupPolicyRepository) LockControlCatalog(ctx context.Context, tx *gorm.DB) error {
	if tx == nil || tx.Dialector.Name() != "postgres" {
		return errors.New("backup control requires PostgreSQL transaction")
	}
	sum := sha256.Sum256([]byte("powerx.backup.control/v1\x00" + (&model.BackupPolicy{}).TableName()))
	return tx.WithContext(ctx).Exec("SELECT pg_advisory_xact_lock(?)", int64(binary.BigEndian.Uint64(sum[:8]))).Error
}
func (r *BackupPolicyRepository) ControlRows(ctx context.Context, db *gorm.DB) ([]model.BackupPolicy, error) {
	rows := []model.BackupPolicy{}
	e := db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Order("uuid ASC").Limit(201).Find(&rows).Error
	return rows, e
}

// WriteControlRows 只更新既有平台计划表，不建立第二份可编辑配置。调用方已持有目录锁并核对完整摘要。
func (r *BackupPolicyRepository) WriteControlRows(ctx context.Context, tx *gorm.DB, scope, actor string, rows []model.BackupPolicy) error {
	if e := tx.WithContext(ctx).Model(&model.BackupPolicy{}).Where("control_scope_id = ? OR control_scope_id = ?", scope, "").Update("is_current", false).Error; e != nil {
		return e
	}
	for _, row := range rows {
		if row.ID == 0 {
			row.CreatedBy = actor
			row.UpdatedBy = actor
			row.ControlScopeID = scope
			if e := tx.WithContext(ctx).Create(&row).Error; e != nil {
				return e
			}
			continue
		}
		values := map[string]any{"name": row.Name, "backup_type": row.BackupType, "schedule": row.Schedule, "interval_hours": row.IntervalHours, "retention_days": row.RetentionDays, "retention_mode": row.RetentionMode, "retention_count": row.RetentionCount, "timezone": row.Timezone, "drill_enabled": row.DrillEnabled, "drill_interval_days": row.DrillIntervalDays, "target_ref": row.TargetRef, "storage_target": row.StorageTarget, "enabled": row.Enabled, "is_current": row.IsCurrent, "control_scope_id": scope, "execution_fingerprint": row.ExecutionFingerprint, "control_revision": row.ControlRevision, "updated_by": actor}
		result := tx.WithContext(ctx).Model(&model.BackupPolicy{}).Where("id = ? AND (control_scope_id = ? OR control_scope_id = ?)", row.ID, scope, "").Updates(values)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errors.New("backup policy control row changed")
		}
	}
	return nil
}
