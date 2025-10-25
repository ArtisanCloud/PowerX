package audit

import (
	"reflect"
	"time"

	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	"gorm.io/gorm"
)

func RegisterAuditCallbacks(db *gorm.DB, svc Service) {
	// 防重复注册（被多次调用时只生效一次）
	if _, ok := db.InstanceGet("audit_callbacks_registered"); ok {
		return
	}
	db.InstanceSet("audit_callbacks_registered", true)

	ignoreFull := (&dbm.AuditEvent{}).TableName()          // e.g. public.audit_event
	ignoreShort := (&dbm.AuditEvent{}).GetTableName(false) // e.g. audit_event
	auditType := reflect.TypeOf(dbm.AuditEvent{})

	shouldSkip := func(tx *gorm.DB) bool {
		if tx == nil || tx.Statement == nil {
			return true
		}
		// A) 会话标志：repo 写审计表时会设置这个
		if _, ok := tx.Statement.Settings.Load("skip_audit"); ok {
			return true
		}
		// B) 表名兜底（不同命名策略/含 schema/带引号都可能）
		tbl := tx.Statement.Table
		if tbl == "" {
			return true
		}
		if tbl == ignoreFull || tbl == ignoreShort ||
			tbl == `"`+ignoreFull+`"` || tbl == `"`+ignoreShort+`"` {
			return true
		}
		// C) 模型类型兜底（最稳）
		if tx.Statement.Schema != nil && tx.Statement.Schema.ModelType == auditType {
			return true
		}
		return false
	}

	db.Callback().Create().After("gorm:after_create").Register("audit:create", func(tx *gorm.DB) {
		if shouldSkip(tx) {
			return
		}
		_ = svc.Emit(tx.Statement.Context, &dbm.AuditEvent{
			OccurredAt:   time.Now(),
			Source:       "db",
			Operation:    "CREATE",
			ResourceType: "db.table",
			ResourceID:   tx.Statement.Table,
			Outcome:      "SUCCESS",
			Severity:     "INFO",
		})
	})

	db.Callback().Update().After("gorm:after_update").Register("audit:update", func(tx *gorm.DB) {
		if shouldSkip(tx) {
			return
		}
		_ = svc.Emit(tx.Statement.Context, &dbm.AuditEvent{
			OccurredAt:   time.Now(),
			Source:       "db",
			Operation:    "UPDATE",
			ResourceType: "db.table",
			ResourceID:   tx.Statement.Table,
			Outcome:      "SUCCESS",
			Severity:     "INFO",
		})
	})

	db.Callback().Delete().After("gorm:after_delete").Register("audit:delete", func(tx *gorm.DB) {
		if shouldSkip(tx) {
			return
		}
		_ = svc.Emit(tx.Statement.Context, &dbm.AuditEvent{
			OccurredAt:   time.Now(),
			Source:       "db",
			Operation:    "DELETE",
			ResourceType: "db.table",
			ResourceID:   tx.Statement.Table,
			Outcome:      "SUCCESS",
			Severity:     "INFO",
		})
	})
}
