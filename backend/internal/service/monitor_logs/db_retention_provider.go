package monitorlogs

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	logcfg "github.com/ArtisanCloud/PowerX/pkg/utils/logger/config"
	"gorm.io/gorm"
)

var identRegexp = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

type DBRetentionProvider struct {
	db       *gorm.DB
	batch    int
	maxRows  int
	tableCfg []logcfg.RetentionDBTable
}

func NewDBRetentionProvider(db *gorm.DB, batchSize, maxDeleteRows int, tables []logcfg.RetentionDBTable) *DBRetentionProvider {
	if batchSize <= 0 {
		batchSize = 5000
	}
	if maxDeleteRows <= 0 {
		maxDeleteRows = 200000
	}
	return &DBRetentionProvider{
		db:       db,
		batch:    batchSize,
		maxRows:  maxDeleteRows,
		tableCfg: append([]logcfg.RetentionDBTable{}, tables...),
	}
}

func (p *DBRetentionProvider) Cleanup(ctx context.Context, defaultRetentionDays int) (int64, []string) {
	if p == nil || p.db == nil || len(p.tableCfg) == 0 {
		return 0, nil
	}
	var totalDeleted int64
	errs := make([]string, 0, 4)
	for i := range p.tableCfg {
		cfg := p.tableCfg[i]
		name := strings.TrimSpace(cfg.Name)
		col := strings.TrimSpace(cfg.TimeColumn)
		if name == "" || col == "" {
			continue
		}
		if !isSafeTableName(name) || !isSafeIdent(col) {
			errs = append(errs, fmt.Sprintf("skip unsafe table config: %s.%s", name, col))
			continue
		}
		if !p.db.Migrator().HasTable(name) {
			continue
		}
		retentionDays := cfg.RetentionDays
		if retentionDays <= 0 {
			retentionDays = defaultRetentionDays
		}
		if retentionDays <= 0 {
			continue
		}
		cutoff := time.Now().Add(-time.Duration(retentionDays) * 24 * time.Hour)
		deleted, err := p.cleanupTable(ctx, name, col, cutoff)
		totalDeleted += deleted
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", name, err))
		}
	}
	return totalDeleted, errs
}

func (p *DBRetentionProvider) cleanupTable(ctx context.Context, table, timeColumn string, cutoff time.Time) (int64, error) {
	quotedTable := quoteCompositeName(table)
	quotedCol := quoteIdent(timeColumn)
	sql := fmt.Sprintf(`
DELETE FROM %s
WHERE ctid IN (
  SELECT ctid FROM %s
  WHERE %s < @cutoff
  LIMIT @limit
)`, quotedTable, quotedTable, quotedCol)

	var total int64
	remaining := p.maxRows
	for remaining > 0 {
		limit := p.batch
		if remaining < limit {
			limit = remaining
		}
		res := p.db.WithContext(ctx).Exec(sql, map[string]any{
			"cutoff": cutoff,
			"limit":  limit,
		})
		if res.Error != nil {
			return total, res.Error
		}
		if res.RowsAffected <= 0 {
			break
		}
		total += res.RowsAffected
		remaining -= int(res.RowsAffected)
	}
	return total, nil
}

func isSafeTableName(name string) bool {
	parts := strings.Split(name, ".")
	for i := range parts {
		if !isSafeIdent(parts[i]) {
			return false
		}
	}
	return true
}

func isSafeIdent(name string) bool {
	return identRegexp.MatchString(strings.TrimSpace(name))
}

func quoteCompositeName(name string) string {
	parts := strings.Split(name, ".")
	quoted := make([]string, 0, len(parts))
	for i := range parts {
		quoted = append(quoted, quoteIdent(parts[i]))
	}
	return strings.Join(quoted, ".")
}

func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(strings.TrimSpace(name), `"`, `""`) + `"`
}
