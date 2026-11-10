package backup_ops

import (
	"context"
	"database/sql"
	recovery "github.com/ArtisanCloud/PowerX/internal/infra/storage_recovery"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	"github.com/jackc/pgx/v5"
	"gorm.io/gorm"
	"reflect"
)

// 固定来源覆盖检查，不补建、不改写 SQL 或独立结果。
func checkResolutionSQLCoverage(ctx context.Context, db *gorm.DB, schema string, records map[string]recovery.RetentionRecord) (int, error) {
	table := pgx.Identifier{schema, model.TableOpsBackupCleanupResolutions}.Sanitize()
	var present bool
	if e := db.WithContext(ctx).Raw("SELECT to_regclass(?) IS NOT NULL", table).Scan(&present).Error; e != nil {
		return 0, controlFailure(503, "BACKUP_RESOLUTION_HISTORY_UNAVAILABLE")
	}
	if !present {
		return 0, nil
	}
	count := 0
	e := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		rows := []m.BackupCleanupResolution{}
		if e := tx.WithContext(ctx).Unscoped().Table(table).Order("request_id ASC").Limit(recovery.MaxRetentionRecords + 1).Find(&rows).Error; e != nil {
			return controlFailure(503, "BACKUP_RESOLUTION_HISTORY_UNAVAILABLE")
		}
		if len(rows) > recovery.MaxRetentionRecords {
			return controlFailure(423, "BACKUP_RESOLUTION_HISTORY_LIMIT")
		}
		for _, row := range rows {
			r, ok := records[row.RequestID.String()]
			if !ok || !resolutionMatches(r, row) || r.State == "checking" && row.State != "checking" || r.State != "checking" && row.State != "checking" && !reflect.DeepEqual(r, resolutionRecord(row)) {
				return controlFailure(423, "BACKUP_RESOLUTION_HISTORY_COVERAGE_REQUIRED")
			}
		}
		count = len(rows)
		return nil
	}, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	return count, e
}
