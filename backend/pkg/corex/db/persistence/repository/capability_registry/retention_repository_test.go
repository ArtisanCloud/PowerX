package capability_registry

import (
	"context"
	"encoding/json"
	core "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	tenants "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func retentionDB(t *testing.T) *gorm.DB {
	t.Helper()
	old := core.PowerXSchema
	core.PowerXSchema = "main"
	t.Cleanup(func() { core.PowerXSchema = old })
	var driver gorm.Dialector = sqlite.Open(filepath.Join(t.TempDir(), "retention.db"))
	if file := os.Getenv("POWERX_STORAGE_VALIDATION_CONFIG"); file != "" {
		var cfg struct {
			DSN string `json:"pg_dsn"`
		}
		raw, e := os.ReadFile(file)
		require.NoError(t, e)
		require.NoError(t, json.Unmarshal(raw, &cfg))
		require.True(t, strings.Contains(cfg.DSN, "port=19432"))
		driver = postgres.Open(cfg.DSN)
		core.PowerXSchema = "validation_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	}
	db, err := gorm.Open(driver, &gorm.Config{})
	require.NoError(t, err)
	sqlDB, _ := db.DB()
	t.Cleanup(func() { sqlDB.Close() })
	if db.Dialector.Name() == "postgres" {
		schema := core.PowerXSchema
		require.NoError(t, db.Exec("CREATE SCHEMA "+schema).Error)
		t.Cleanup(func() { _ = db.Exec("DROP SCHEMA " + schema + " CASCADE").Error })
	}
	require.NoError(t, db.AutoMigrate(&m.InvocationTrace{}, &m.CapabilityEventPublication{}, &tenants.Tenant{}))
	return db
}
func TestRetentionPreservesPendingUnknownForeignReferencesAndRecent(t *testing.T) {
	db := retentionDB(t)
	repo := NewRetentionRepository(db)
	require.NoError(t, repo.Ready(context.Background()))
	tenant := uuid.NewString()
	registeredTenant := &tenants.Tenant{Key: "test", Name: "test"}
	registeredTenant.UUID = uuid.MustParse(tenant)
	require.NoError(t, db.Create(registeredTenant).Error)
	now := time.Now().UTC()
	old := now.AddDate(0, 0, -100)
	makePair := func(status, class, tenantID string, when time.Time) (m.InvocationTrace, m.CapabilityEventPublication) {
		event := m.CapabilityEventPublication{TenantUUID: tenantID, Topic: "_topic.integration.gateway.invocation.succeeded", TraceID: uuid.NewString(), RecordClass: class, Status: status, TerminalAt: &when, Payload: []byte(`{}`)}
		require.NoError(t, db.Create(&event).Error)
		trace := m.InvocationTrace{TenantUUID: tenantID, TraceID: event.TraceID, RecordClass: m.InvocationAuditClass, Status: "completed", CompletedAt: &old, PluginID: "test", CapabilityID: "test", EventPublicationID: &event.UUID}
		require.NoError(t, db.Create(&trace).Error)
		return trace, event
	}
	removedTrace, removedEvent := makePair("published", m.InvocationNotificationClass, tenant, old)
	pendingTrace, _ := makePair("pending", m.InvocationNotificationClass, tenant, now)
	recentTrace, _ := makePair("published", m.InvocationNotificationClass, tenant, now)
	unknownTrace, _ := makePair("published", "legacy_unclassified", tenant, old)
	foreignTrace, _ := makePair("published", m.InvocationNotificationClass, uuid.NewString(), old)
	_, referenced := makePair("published", m.InvocationNotificationClass, tenant, old)
	extra := m.InvocationTrace{TenantUUID: tenant, TraceID: "unknown-reference", RecordClass: "legacy_unclassified", Status: "pending", EventPublicationID: &referenced.UUID}
	require.NoError(t, db.Create(&extra).Error)
	c := RetentionCutoffs{Success: now.AddDate(0, 0, -30), Failure: now.AddDate(0, 0, -90), Quarantine: now.AddDate(0, 0, -30), Pending: now.Add(-24 * time.Hour), Limit: 200}
	preview, err := repo.Batch(context.Background(), c, false)
	require.NoError(t, err)
	require.EqualValues(t, 2, preview.Traces)
	require.Zero(t, preview.Events) // 预览不假设先删 trace 已发生。
	result, err := repo.Batch(context.Background(), c, true)
	require.NoError(t, err)
	require.EqualValues(t, 2, result.Traces)
	require.EqualValues(t, 1, result.Events)
	var n int64
	require.NoError(t, db.Unscoped().Model(&m.InvocationTrace{}).Where("uuid IN ?", []uuid.UUID{pendingTrace.UUID, recentTrace.UUID, unknownTrace.UUID, foreignTrace.UUID}).Count(&n).Error)
	require.EqualValues(t, 4, n)
	require.NoError(t, db.Unscoped().Model(&m.InvocationTrace{}).Where("uuid = ?", removedTrace.UUID).Count(&n).Error)
	require.Zero(t, n)
	require.NoError(t, db.Unscoped().Model(&m.CapabilityEventPublication{}).Where("uuid = ?", removedEvent.UUID).Count(&n).Error)
	require.Zero(t, n)
	require.NoError(t, db.Unscoped().Model(&m.CapabilityEventPublication{}).Where("uuid = ?", referenced.UUID).Count(&n).Error)
	require.EqualValues(t, 1, n)
}
func TestLegacyPendingRequiresExplicitAdoptionAndPayloadProof(t *testing.T) {
	db := retentionDB(t)
	repo := NewRetentionRepository(db)
	tenant := &tenants.Tenant{Key: "test", Name: "test"}
	require.NoError(t, db.Create(tenant).Error)
	old := time.Now().AddDate(0, 0, -100)
	tenantID := tenant.UUID.String()
	payload, _ := json.Marshal(map[string]string{"tenant_uuid": tenantID, "trace_id": "old-trace", "plugin_id": "test", "capability_id": "test", "status": "completed"})
	event := m.CapabilityEventPublication{TenantUUID: tenantID, TraceID: "old-trace", Topic: "_topic.integration.gateway.invocation.succeeded", Payload: payload, Status: "pending"}
	event.CreatedAt = old
	require.NoError(t, db.Create(&event).Error)
	trace := m.InvocationTrace{TenantUUID: tenantID, TraceID: event.TraceID, PluginID: "test", CapabilityID: "test", Status: "completed", EventPublicationID: &event.UUID}
	trace.UpdatedAt = old
	require.NoError(t, db.Create(&trace).Error)
	c := RetentionCutoffs{Success: time.Now(), Failure: time.Now(), Quarantine: time.Now().AddDate(0, 0, -30), Pending: time.Now().Add(-24 * time.Hour), Limit: 10}
	result, err := repo.Batch(context.Background(), c, true)
	require.NoError(t, err)
	require.Zero(t, result.Quarantined)
	malformedPayload, _ := json.Marshal(map[string]string{"tenant_uuid": tenantID, "trace_id": "unverified-old", "capability_id": "other", "plugin_id": "test", "status": "completed"})
	unknown := m.CapabilityEventPublication{TenantUUID: tenantID, TraceID: "unverified-old", Topic: event.Topic, Payload: malformedPayload, Status: "pending"}
	unknown.CreatedAt = old.Add(-24 * time.Hour)
	require.NoError(t, db.Create(&unknown).Error)
	unknownTrace := m.InvocationTrace{TenantUUID: tenantID, TraceID: unknown.TraceID, PluginID: "test", CapabilityID: "test", Status: "completed", EventPublicationID: &unknown.UUID}
	require.NoError(t, db.Create(&unknownTrace).Error)
	c.Limit = 1 // 不一致的更老记录不能占住全部批次，让已核验通知永远无法处置。
	c.AdoptLegacy = true
	result, err = repo.Batch(context.Background(), c, true)
	require.NoError(t, err)
	require.EqualValues(t, 1, result.Quarantined)
	require.Zero(t, result.Events)
	require.Zero(t, result.Traces)
	require.NoError(t, db.First(&event, "uuid = ?", event.UUID).Error)
	require.Equal(t, "quarantined", event.Status)
	require.Empty(t, event.DeliveryReceipt)
	result, err = repo.Batch(context.Background(), c, true)
	require.NoError(t, err)
	require.Zero(t, result.Traces, "new quarantine must retain the linked trace throughout its grace period")
	require.Zero(t, result.Events)
	c.Quarantine = time.Now().Add(time.Hour) // 模拟宽限期已经结束，而非修改运行中的数据库时钟。
	result, err = repo.Batch(context.Background(), c, true)
	require.NoError(t, err)
	require.EqualValues(t, 1, result.Traces)
	require.EqualValues(t, 1, result.Events)
	var remaining int64
	require.NoError(t, db.Model(&m.CapabilityEventPublication{}).Where("uuid = ?", unknown.UUID).Count(&remaining).Error)
	require.EqualValues(t, 1, remaining, "unverified legacy notification remains protected even after the grace cutoff")
}

func TestRetentionUsesTerminalTimeAndSeparateFailureWindow(t *testing.T) {
	db := retentionDB(t)
	tenant := &tenants.Tenant{Key: "active", Name: "Active"}
	deleted := &tenants.Tenant{Key: "deleted", Name: "Deleted"}
	require.NoError(t, db.Create(tenant).Error)
	require.NoError(t, db.Create(deleted).Error)
	require.NoError(t, db.Delete(deleted).Error)
	now := time.Now().UTC()
	old, middle, recent := now.AddDate(0, 0, -100), now.AddDate(0, 0, -40), now.AddDate(0, 0, -1)
	var protected []uuid.UUID
	for _, tc := range []struct {
		status, class, tenant string
		terminal              *time.Time
		remove                bool
	}{
		{"completed", m.InvocationAuditClass, tenant.UUID.String(), &middle, true},
		{"failed", m.InvocationAuditClass, tenant.UUID.String(), &old, true},
		{"failed", m.InvocationAuditClass, tenant.UUID.String(), &middle, false},
		{"completed", m.InvocationAuditClass, tenant.UUID.String(), &recent, false},
		{"completed", m.InvocationAuditClass, tenant.UUID.String(), nil, false},
		{"pending", m.InvocationAuditClass, tenant.UUID.String(), &old, false},
		{"completed", "legacy_unclassified", tenant.UUID.String(), &old, false},
		{"completed", m.InvocationAuditClass, deleted.UUID.String(), &old, false},
	} {
		row := m.InvocationTrace{TraceID: uuid.NewString(), TenantUUID: tc.tenant, PluginID: "test", CapabilityID: "test", Status: tc.status, RecordClass: tc.class, CompletedAt: tc.terminal}
		row.CreatedAt = old // 创建很久但刚结束，仍必须按终态时间保留。
		require.NoError(t, db.Create(&row).Error)
		if !tc.remove {
			protected = append(protected, row.UUID)
		}
	}
	c := RetentionCutoffs{Success: now.AddDate(0, 0, -30), Failure: now.AddDate(0, 0, -90), Pending: now.Add(-24 * time.Hour), Quarantine: now.AddDate(0, 0, -30), Limit: 200}
	out, err := NewRetentionRepository(db).Batch(context.Background(), c, true)
	require.NoError(t, err)
	require.EqualValues(t, 2, out.Traces)
	var n int64
	require.NoError(t, db.Unscoped().Model(&m.InvocationTrace{}).Where("uuid IN ?", protected).Count(&n).Error)
	require.EqualValues(t, len(protected), n)
}
