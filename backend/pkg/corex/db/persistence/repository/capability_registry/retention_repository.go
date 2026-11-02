package capability_registry

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	tenants "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// RetentionCutoffs 是 Service 已确认的期限，不接受表名或任意过滤表达式。
type RetentionCutoffs struct {
	Success, Failure, Pending, Quarantine time.Time
	Limit                                 int
	AdoptLegacy                           bool
}
type RetentionBatch struct {
	Traces      int64 `json:"traces"`
	Events      int64 `json:"events"`
	Quarantined int64 `json:"quarantined"`
	Classified  int64 `json:"classified"`
}
type RetentionRepository struct{ db *gorm.DB }

func NewRetentionRepository(db *gorm.DB) *RetentionRepository { return &RetentionRepository{db: db} }

// Ready 只检查，不执行迁移；生产大表索引必须先在容量允许的维护窗口完成。
func (r *RetentionRepository) Ready(ctx context.Context) error {
	if r == nil || r.db == nil {
		return errors.New("database_unavailable")
	}
	db := r.db.WithContext(ctx)
	for _, v := range []struct {
		model   any
		columns []string
		indexes []string
	}{
		{&m.InvocationTrace{}, []string{"record_class", "completed_at"}, []string{"idx_capability_trace_retention", "idx_capability_trace_publication"}},
		{&m.CapabilityEventPublication{}, []string{"record_class", "terminal_at", "delivery_receipt"}, []string{"idx_capability_event_retention"}},
	} {
		for _, col := range v.columns {
			if !db.Migrator().HasColumn(v.model, col) {
				return errors.New("database_retention_migration_required")
			}
		}
		for _, idx := range v.indexes {
			if !db.Migrator().HasIndex(v.model, idx) {
				return errors.New("database_retention_index_required")
			}
		}
	}
	if !db.Migrator().HasTable(&tenants.Tenant{}) {
		return errors.New("tenant_registry_unavailable")
	}
	if db.Dialector.Name() == "postgres" {
		var n int64
		err := db.Raw(`SELECT count(*) FROM pg_constraint WHERE contype='f' AND confrelid IN (to_regclass(?),to_regclass(?))`, m.InvocationTrace{}.TableName(), m.CapabilityEventPublication{}.TableName()).Scan(&n).Error
		if err != nil {
			return err
		}
		if n != 0 {
			return errors.New("database_external_references_require_review")
		}
	}
	return ctx.Err()
}
func (r *RetentionRepository) RelationBytes(ctx context.Context) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("database_unavailable")
	}
	if r.db.Dialector.Name() != "postgres" {
		return 0, errors.New("physical_usage_unavailable")
	}
	var usage struct {
		Traces *int64
		Events *int64
	}
	err := r.db.WithContext(ctx).Raw(`SELECT pg_total_relation_size(to_regclass(?)) AS traces,pg_total_relation_size(to_regclass(?)) AS events`, m.InvocationTrace{}.TableName(), m.CapabilityEventPublication{}.TableName()).Scan(&usage).Error
	if err != nil {
		return 0, err
	}
	// to_regclass 在表缺失时返回 NULL；显式返回占用未知，避免依赖驱动的标量 NULL 扫描行为。
	if usage.Traces == nil || usage.Events == nil {
		return 0, errors.New("physical_usage_unavailable")
	}
	return *usage.Traces + *usage.Events, nil
}
func registered(db *gorm.DB, alias string) *gorm.DB {
	// 固定模型的实例级 Root 维护；只进入登记且未删除的租户，不推断未知租户归属。
	return db.Where("EXISTS (SELECT 1 FROM " + (&tenants.Tenant{}).TableName() + " AS rt WHERE CAST(rt.uuid AS TEXT) = " + alias + ".tenant_uuid AND rt.deleted_at IS NULL)")
}
func eventExpired(db *gorm.DB, alias string, c RetentionCutoffs) *gorm.DB {
	if alias != "" {
		alias += "."
	}
	alias = strings.TrimSpace(alias)
	return db.Where(alias+"record_class = ? AND "+alias+"terminal_at IS NOT NULL", m.InvocationNotificationClass).
		Where("(("+alias+"status IN ? AND "+alias+"terminal_at < ?) OR ("+alias+"status IN ? AND "+alias+"terminal_at < ?) OR ("+alias+"status = ? AND "+alias+"terminal_at < ?))", []string{"published", "dispatched"}, c.Success, []string{"failed", "unrouted"}, c.Failure, "quarantined", c.Quarantine)
}
func (r *RetentionRepository) Batch(ctx context.Context, c RetentionCutoffs, apply bool) (RetentionBatch, error) {
	out := RetentionBatch{}
	if c.Limit < 1 || c.Limit > 1000 {
		return out, errors.New("invalid_batch_limit")
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if tx.Dialector.Name() == "postgres" {
			if err := tx.Exec("SET LOCAL lock_timeout='1s'").Error; err != nil {
				return err
			}
			if err := tx.Exec("SET LOCAL statement_timeout='8s'").Error; err != nil {
				return err
			}
		}
		lock := clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}
		// 超时通知只允许已分类记录，或显式采用且有同租户终态链路与负载一致的旧通知。
		var pending []m.CapabilityEventPublication
		q := registered(tx.Model(&m.CapabilityEventPublication{}).Table(m.CapabilityEventPublication{}.TableName()+" AS pe"), "pe").Select("pe.uuid,pe.tenant_uuid,pe.trace_id,pe.topic,pe.payload,pe.status,pe.created_at,pe.record_class,pe.publish_time,pe.attempts").Where("pe.status IN ? AND pe.created_at < ?", []string{"pending", "unconfirmed"}, c.Pending)
		if c.AdoptLegacy {
			q = q.Where("(pe.record_class = ? OR (pe.record_class = ? AND pe.attempts = 0 AND pe.publish_time IS NULL))", m.InvocationNotificationClass, "legacy_unclassified")
		} else {
			q = q.Where("pe.record_class = ?", m.InvocationNotificationClass)
		}
		q = q.Where("pe.topic IN ?", []string{"_topic.integration.gateway.invocation.succeeded", "_topic.integration.gateway.invocation.failed"}).Where("EXISTS (SELECT 1 FROM "+m.InvocationTrace{}.TableName()+" AS proof WHERE proof.event_publication_id=pe.uuid AND proof.tenant_uuid=pe.tenant_uuid AND proof.trace_id=pe.trace_id AND proof.status IN ?)", []string{"completed", "success", "succeeded", "failed", "error", "denied"})
		jsonField := func(field string) string {
			if tx.Dialector.Name() == "postgres" {
				return "(pe.payload ->> '" + field + "')"
			}
			return "json_extract(pe.payload, '$." + field + "')"
		}
		// 在有界扫描前排除不一致通知，防止旧未知记录占住批次而使可核验记录永久饥饿。
		proofTable := m.InvocationTrace{}.TableName()
		q = q.Where("(SELECT count(*) FROM " + proofTable + " AS all_ref WHERE all_ref.event_publication_id=pe.uuid)=1").Where("EXISTS (SELECT 1 FROM " + proofTable + " AS matched WHERE matched.event_publication_id=pe.uuid AND CAST(matched.tenant_uuid AS TEXT)=" + jsonField("tenant_uuid") + " AND matched.trace_id=" + jsonField("trace_id") + " AND matched.capability_id=" + jsonField("capability_id") + " AND matched.plugin_id=" + jsonField("plugin_id") + " AND matched.status=" + jsonField("status") + ")")
		if apply {
			q = q.Clauses(lock)
		}
		if err := q.Order("pe.created_at, pe.id").Limit(c.Limit).Find(&pending).Error; err != nil {
			return err
		}
		eventIDs := make([]uuid.UUID, 0, len(pending))
		for _, e := range pending {
			eventIDs = append(eventIDs, e.UUID)
		}
		var linked []m.InvocationTrace
		lq := tx.Unscoped().Select("uuid,tenant_uuid,event_publication_id,trace_id,capability_id,plugin_id,status,record_class,updated_at").Where("event_publication_id IN ?", eventIDs)
		if apply {
			lq = lq.Clauses(lock)
		}
		if len(eventIDs) > 0 {
			if err := lq.Limit(c.Limit*20 + 1).Find(&linked).Error; err != nil {
				return err
			}
		}
		if len(linked) > c.Limit*20 {
			return errors.New("reference_limit_exceeded")
		}
		refs := map[uuid.UUID][]m.InvocationTrace{}
		for _, tr := range linked {
			if tr.EventPublicationID != nil {
				refs[*tr.EventPublicationID] = append(refs[*tr.EventPublicationID], tr)
			}
		}
		for _, e := range pending {
			traces := refs[e.UUID]
			if !verifiedNotification(e, traces) {
				continue
			}
			out.Quarantined++
			if !apply {
				continue
			}
			now := time.Now().UTC()
			if err := tx.Model(&m.CapabilityEventPublication{}).Where("tenant_uuid = ? AND uuid = ? AND status = ?", e.TenantUUID, e.UUID, e.Status).Updates(map[string]any{"record_class": m.InvocationNotificationClass, "status": "quarantined", "terminal_at": now, "last_error": "publication_unconfirmed_timeout_no_business_completion_claim"}).Error; err != nil {
				return err
			}
			for _, tr := range traces {
				if tr.RecordClass == "legacy_unclassified" && m.TerminalInvocation(tr.Status) {
					if err := tx.Model(&m.InvocationTrace{}).Where("tenant_uuid = ? AND uuid = ?", e.TenantUUID, tr.UUID).Updates(map[string]any{"record_class": m.InvocationAuditClass, "completed_at": tr.UpdatedAt}).Error; err != nil {
						return err
					}
					out.Classified++
				}
			}
		}
		// 先删过期终态审计；关联通知尚未到期、来源不明或仍待核实时保护链路。
		var traces []m.InvocationTrace
		tq := registered(tx.Model(&m.InvocationTrace{}).Table(m.InvocationTrace{}.TableName()+" AS tr"), "tr").Select("tr.uuid,tr.tenant_uuid,tr.event_publication_id,tr.completed_at,tr.status").Where("tr.record_class = ? AND tr.completed_at IS NOT NULL", m.InvocationAuditClass).
			Where("((tr.status IN ? AND tr.completed_at < ?) OR (tr.status IN ? AND tr.completed_at < ?))", []string{"completed", "success", "succeeded"}, c.Success, []string{"failed", "error", "denied"}, c.Failure)
		eligiblePublication := eventExpired(tx.Unscoped().Model(&m.CapabilityEventPublication{}).Table(m.CapabilityEventPublication{}.TableName()+" AS valid_pub").Select("1").Where("valid_pub.uuid=tr.event_publication_id AND valid_pub.tenant_uuid=tr.tenant_uuid"), "valid_pub", c)
		tq = tq.Where("tr.event_publication_id IS NULL OR EXISTS (?)", eligiblePublication)
		if apply {
			tq = tq.Clauses(lock)
		}
		if err := tq.Order("tr.completed_at, tr.id").Limit(c.Limit).Find(&traces).Error; err != nil {
			return err
		}
		linkedIDs := make([]uuid.UUID, 0, len(traces))
		for _, tr := range traces {
			if tr.EventPublicationID != nil {
				linkedIDs = append(linkedIDs, *tr.EventPublicationID)
			}
		}
		var eligible []m.CapabilityEventPublication
		eqReady := eventExpired(tx.Unscoped().Select("uuid,tenant_uuid").Where("uuid IN ?", linkedIDs), "", c)
		if apply {
			eqReady = eqReady.Clauses(lock)
		}
		if len(linkedIDs) > 0 {
			if err := eqReady.Find(&eligible).Error; err != nil {
				return err
			}
		}
		eligibleIDs := map[string]bool{}
		for _, e := range eligible {
			eligibleIDs[e.TenantUUID+e.UUID.String()] = true
		}
		for _, tr := range traces {
			if tr.EventPublicationID != nil && !eligibleIDs[tr.TenantUUID+tr.EventPublicationID.String()] {
				continue
			}
			out.Traces++
			if apply {
				if err := tx.Unscoped().Where("tenant_uuid = ? AND uuid = ?", tr.TenantUUID, tr.UUID).Delete(&m.InvocationTrace{}).Error; err != nil {
					return err
				}
			}
		}
		var events []m.CapabilityEventPublication
		eq := registered(tx.Model(&m.CapabilityEventPublication{}).Table(m.CapabilityEventPublication{}.TableName()+" AS ep"), "ep")
		eq = eventExpired(eq, "ep", c)
		eq = eq.Where("NOT EXISTS (SELECT 1 FROM " + m.InvocationTrace{}.TableName() + " AS ref WHERE ref.event_publication_id=ep.uuid)")
		if apply {
			eq = eq.Clauses(lock)
		}
		if err := eq.Select("ep.uuid,ep.tenant_uuid").Order("ep.terminal_at, ep.id").Limit(c.Limit).Find(&events).Error; err != nil {
			return err
		}
		for _, e := range events {
			out.Events++
			if apply {
				if err := tx.Unscoped().Where("tenant_uuid = ? AND uuid = ?", e.TenantUUID, e.UUID).Delete(&m.CapabilityEventPublication{}).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return RetentionBatch{}, err
	}
	return out, nil
}

func verifiedNotification(e m.CapabilityEventPublication, traces []m.InvocationTrace) bool {
	if len(traces) == 0 || len(traces) >= 20 || e.UUID == uuid.Nil {
		return false
	}
	if e.Topic != "_topic.integration.gateway.invocation.succeeded" && e.Topic != "_topic.integration.gateway.invocation.failed" {
		return false
	}
	var p struct {
		Tenant     string `json:"tenant_uuid"`
		Trace      string `json:"trace_id"`
		Capability string `json:"capability_id"`
		Plugin     string `json:"plugin_id"`
		Status     string `json:"status"`
	}
	if json.Unmarshal(e.Payload, &p) != nil || p.Tenant != e.TenantUUID || p.Trace != e.TraceID {
		return false
	}
	for _, tr := range traces {
		if tr.TenantUUID != e.TenantUUID || tr.TraceID != e.TraceID || !m.TerminalInvocation(tr.Status) || tr.CapabilityID != p.Capability || tr.PluginID != p.Plugin || tr.Status != p.Status {
			return false
		}
	}
	return true
}
