package backup_ops

import (
	"context"
	"database/sql"

	recovery "github.com/ArtisanCloud/PowerX/internal/infra/storage_recovery"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/ops"
	"github.com/jackc/pgx/v5"
	"gorm.io/gorm"
)

// CheckRetentionRecoveryCoverage is a fixed maintenance check of the current
// database, including foreign installation scopes. It never imports or writes.
func CheckRetentionRecoveryCoverage(ctx context.Context, db *gorm.DB, schema string, records map[string]recovery.RetentionRecord) (int, error) {
	legacy, e := checkRetentionSQLCoverage(ctx, db, schema, records)
	if e != nil {
		return 0, e
	}
	resolutions, e := checkResolutionSQLCoverage(ctx, db, schema, records)
	return legacy + resolutions, e
}
func checkRetentionSQLCoverage(ctx context.Context, db *gorm.DB, schema string, records map[string]recovery.RetentionRecord) (int, error) {
	var present bool
	if e := db.WithContext(ctx).Raw("SELECT to_regclass(?) IS NOT NULL", pgx.Identifier{schema, coremodel.TableOpsBackupCleanupRetentions}.Sanitize()).Scan(&present).Error; e != nil {
		return 0, controlFailure(503, "BACKUP_HISTORY_SOURCE_UNAVAILABLE")
	}
	if !present {
		return 0, nil
	}
	count := 0
	e := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		rows, total, _, e := repo.NewBackupCleanupRetentionRepository(tx).HistorySnapshotSchema(ctx, tx, schema, "", recovery.MaxRetentionRecords+1)
		if e != nil {
			return controlFailure(503, "BACKUP_HISTORY_SOURCE_UNAVAILABLE")
		}
		if total > recovery.MaxRetentionRecords {
			return controlFailure(423, "BACKUP_HISTORY_SOURCE_LIMIT")
		}
		for _, row := range rows {
			coverage, _, _ := historyCoverage(row, records)
			if coverage != "covered" {
				return controlFailure(423, "BACKUP_HISTORY_COVERAGE_REQUIRED")
			}
		}
		count = len(rows)
		return nil
	}, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	return count, e
}
