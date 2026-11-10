package database

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/storage_governance"
	"gorm.io/gorm"
)

// StorageGovernanceSchemaError 区分缺失结构与检查失败，不包含连接信息或 SQL 原文。
type StorageGovernanceSchemaError struct {
	Unavailable bool
	Reason      string
}

func (e *StorageGovernanceSchemaError) ReasonCode() string {
	if e.Unavailable {
		return "STORAGE_GOVERNANCE_UNAVAILABLE"
	}
	return "STORAGE_CONTROL_SCHEMA_NOT_READY"
}
func (e *StorageGovernanceSchemaError) Error() string {
	return fmt.Sprintf("%s (%s)", e.ReasonCode(), e.Reason)
}

// CheckStorageGovernanceSchema 只读验证集中迁移的六张表；启动和升级均不隐式补表。
func CheckStorageGovernanceSchema(db *gorm.DB) error {
	if err := CheckStorageGovernanceControlSchema(db); err != nil {
		return err
	}
	for _, table := range []string{(&m.Policy{}).TableName(), (&m.PolicyRevision{}).TableName(), (&m.Sample{}).TableName(), (&m.CleanupRun{}).TableName()} {
		if err := storageTableExists(db, table); err != nil {
			return err
		}
	}
	return nil
}

// CheckStorageGovernanceControlSchema 为运行时控制仓储保留独立的结构检查边界。
func CheckStorageGovernanceControlSchema(db *gorm.DB) error {
	if db == nil || db.Config == nil || db.Dialector == nil {
		return &StorageGovernanceSchemaError{Unavailable: true, Reason: "database_unavailable"}
	}
	for _, check := range []struct {
		model  any
		table  string
		column string
	}{
		{&m.ControlConfiguration{}, (&m.ControlConfiguration{}).TableName(), "applied_revision"},
		{&m.ControlRevision{}, (&m.ControlRevision{}).TableName(), "request_digest"},
	} {
		if err := storageTableExists(db, check.table); err != nil {
			return err
		}
		columns, err := db.Migrator().ColumnTypes(check.table)
		if err != nil {
			return &StorageGovernanceSchemaError{Unavailable: true, Reason: "column_inspection_failed"}
		}
		found := false
		for _, column := range columns {
			found = found || column.Name() == check.column
		}
		if !found {
			return &StorageGovernanceSchemaError{Reason: "column_missing:" + check.column}
		}
	}
	for _, check := range []struct {
		model       any
		table, name string
		columns     []string
	}{
		{&m.ControlConfiguration{}, (&m.ControlConfiguration{}).TableName(), "uk_ops_control_scope_domain", []string{"tenant_id", "scope_id", "domain"}},
		{&m.ControlRevision{}, (&m.ControlRevision{}).TableName(), "uk_ops_control_revision", []string{"tenant_id", "scope_id", "domain", "revision"}},
		{&m.ControlRevision{}, (&m.ControlRevision{}).TableName(), "uk_ops_control_request", []string{"tenant_id", "scope_id", "request_id"}},
	} {
		valid := false
		if db.Dialector.Name() == "postgres" {
			// 当前驱动 GetIndexes 不限定 schema，也不保证列序；直接按目标关系及键序核对。
			keys, _ := json.Marshal(check.columns)
			err := db.Raw(`SELECT EXISTS (
				SELECT 1 FROM pg_index x JOIN pg_class i ON i.oid=x.indexrelid
				WHERE x.indrelid=to_regclass(?) AND i.relname=?
				AND x.indisunique AND x.indisvalid AND x.indisready
				AND x.indpred IS NULL AND x.indexprs IS NULL AND x.indnkeyatts=x.indnatts
				AND (SELECT to_jsonb(array_agg(a.attname::text ORDER BY k.ord))
				FROM unnest(x.indkey) WITH ORDINALITY k(num,ord)
				JOIN pg_attribute a ON a.attrelid=x.indrelid AND a.attnum=k.num)=CAST(? AS jsonb)
				)`, check.table, check.name, string(keys)).Scan(&valid).Error
			if err != nil {
				return &StorageGovernanceSchemaError{Unavailable: true, Reason: "index_inspection_failed"}
			}
		} else if db.Dialector.Name() == "sqlite" {
			// 固定版本的 SQLite 驱动未实现 GetIndexes，直接读取只读索引目录。
			table := check.table[strings.LastIndex(check.table, ".")+1:]
			err := db.Raw(`SELECT EXISTS(SELECT 1 FROM pragma_index_list(?,?) WHERE name=? AND "unique"=1 AND partial=0)`, table, coremodel.PowerXSchema, check.name).Scan(&valid).Error
			if err != nil {
				return &StorageGovernanceSchemaError{Unavailable: true, Reason: "index_inspection_failed"}
			}
			var columns []string
			if valid {
				err = db.Raw("SELECT name FROM pragma_index_info(?,?) ORDER BY seqno", check.name, coremodel.PowerXSchema).Scan(&columns).Error
				if err != nil {
					return &StorageGovernanceSchemaError{Unavailable: true, Reason: "index_inspection_failed"}
				}
			}
			valid = valid && slices.Equal(columns, check.columns)
		} else {
			indexes, err := db.Migrator().GetIndexes(check.table)
			if err != nil {
				return &StorageGovernanceSchemaError{Unavailable: true, Reason: "index_inspection_failed"}
			}
			for _, index := range indexes {
				unique, known := index.Unique()
				if index.Name() == check.name && known && unique && slices.Equal(index.Columns(), check.columns) {
					valid = true
				}
			}
		}
		if !valid {
			return &StorageGovernanceSchemaError{Reason: "unique_index_missing_or_invalid:" + check.name}
		}
	}
	return nil
}

func storageTableExists(db *gorm.DB, table string) error {
	var exists bool
	var err error
	switch db.Dialector.Name() {
	case "postgres":
		err = db.Raw("SELECT to_regclass(?) IS NOT NULL", table).Scan(&exists).Error
	case "sqlite":
		var quoted strings.Builder
		db.Dialector.QuoteTo(&quoted, coremodel.PowerXSchema)
		name := table[strings.LastIndex(table, ".")+1:]
		err = db.Raw("SELECT EXISTS(SELECT 1 FROM "+quoted.String()+".sqlite_master WHERE type='table' AND name=?)", name).Scan(&exists).Error
	default:
		return &StorageGovernanceSchemaError{Unavailable: true, Reason: "unsupported_database"}
	}
	if err != nil {
		return &StorageGovernanceSchemaError{Unavailable: true, Reason: "table_inspection_failed"}
	}
	if !exists {
		return &StorageGovernanceSchemaError{Reason: "table_missing:" + table}
	}
	return nil
}
