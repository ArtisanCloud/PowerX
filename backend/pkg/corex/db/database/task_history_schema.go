package database

import (
	"fmt"
	"slices"
	"strings"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	eventmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	"gorm.io/gorm"
)

const taskHistoryIdempotencyIndex = "uidx_event_task_histories_idempotency"

// TaskHistorySchemaError 表示部署的任务历史结构未达到运行要求，不包含 SQL 或业务内容。
type TaskHistorySchemaError struct{ Reason string }

func (e *TaskHistorySchemaError) Error() string {
	return fmt.Sprintf("TASK_HISTORY_SCHEMA_NOT_READY (%s): 请先执行正式数据库迁移/升级；dev/test 可用 database migrate -scope task-history-idempotency", e.Reason)
}

// CheckTaskHistorySchema 只读检查启动所需的幂等列与精确唯一约束；不执行自动修复。
func CheckTaskHistorySchema(db *gorm.DB) error {
	fail := func(reason string) error { return &TaskHistorySchemaError{Reason: reason} }
	if db == nil {
		return fail("database_unavailable")
	}
	model := &eventmodel.TaskHistory{}
	if !db.Migrator().HasTable(model) {
		return fail("history_table_missing")
	}
	columns, err := db.Migrator().ColumnTypes(model)
	if err != nil {
		return fail("column_inspection_failed")
	}
	found := false
	for _, column := range columns {
		if column.Name() != "idempotency_key" {
			continue
		}
		found = true
		nullable, knownNullable := column.Nullable()
		length, knownLength := column.Length()
		if !knownNullable || !nullable || !knownLength || length != 255 || !strings.EqualFold(column.DatabaseTypeName(), "varchar") {
			return fail("idempotency_column_invalid")
		}
	}
	if !found {
		return fail("idempotency_column_missing")
	}
	indexes, err := db.Migrator().GetIndexes(model)
	if err != nil {
		return fail("index_inspection_failed")
	}
	found = false
	for _, index := range indexes {
		if index.Name() != taskHistoryIdempotencyIndex {
			continue
		}
		found = true
		unique, known := index.Unique()
		if !known || !unique || !slices.Equal(index.Columns(), []string{"tenant_key", "subscriber_id", "idempotency_key"}) {
			return fail("idempotency_index_invalid")
		}
	}
	if !found {
		return fail("idempotency_index_missing")
	}
	if db.Dialector.Name() == "postgres" {
		var state struct {
			Valid, Ready, Full, Plain bool
			Definition                string
		}
		result := db.Raw(`SELECT i.indisvalid AS valid, i.indisready AS ready,
			i.indpred IS NULL AS full, i.indexprs IS NULL AS plain,
			pg_get_indexdef(i.indexrelid) AS definition
			FROM pg_index i JOIN pg_class idx ON idx.oid=i.indexrelid
			JOIN pg_class tbl ON tbl.oid=i.indrelid JOIN pg_namespace ns ON ns.oid=tbl.relnamespace
			WHERE ns.nspname=? AND tbl.relname=? AND idx.relname=?`,
			coremodel.PowerXSchema, coremodel.TableEventTaskHistories, taskHistoryIdempotencyIndex).Scan(&state)
		if result.Error != nil {
			return fail("index_inspection_failed")
		}
		if result.RowsAffected != 1 || !state.Valid || !state.Ready || !state.Full || !state.Plain || strings.Contains(state.Definition, "NULLS NOT DISTINCT") {
			return fail("idempotency_index_invalid")
		}
	}
	return nil
}
