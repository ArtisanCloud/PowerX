package servicechannel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	cap "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	eventaudit "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/audit"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/database"
	core "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	cm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	iam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	gw "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	setting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/migrator"
	"gorm.io/gorm/schema"
)

const tenant = "6b5d0240-9920-46da-b707-88200e0f51ea"
const producer = "com.powerx.plugins.scrm"
const consumer = "com.powerx.plugins.ai"

type fixture struct {
	db    *gorm.DB
	svc   *Service
	topic *m.TopicDefinition
	now   time.Time
}

// SQLite 不识别生产模型的 PostgreSQL timestamp with time zone 类型。
// 仅在隔离测试中保留相同的字段语义，改用 SQLite 可扫描的 datetime。
type timeDialect struct{ sqlite.Dialector }

func (d timeDialect) Migrator(db *gorm.DB) gorm.Migrator {
	return sqlite.Migrator{Migrator: migrator.Migrator{Config: migrator.Config{DB: db, Dialector: d, CreateIndexAfterCreateTable: true}}}
}

func (d timeDialect) DataTypeOf(f *schema.Field) string {
	if f.GORMDataType == schema.Time {
		return "datetime"
	}
	return d.Dialector.DataTypeOf(f)
}

func setup(t *testing.T) *fixture {
	t.Helper()
	old := core.PowerXSchema
	core.PowerXSchema = "main"
	t.Cleanup(func() { core.PowerXSchema = old })
	var dialector gorm.Dialector = timeDialect{sqlite.Dialector{DSN: fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())}}
	if dsn := os.Getenv("POWERX_EVENTFABRIC_TEST_DSN"); dsn != "" {
		core.PowerXSchema = "eventfabric_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
		dialector = postgres.Open(dsn)
	}
	db, err := gorm.Open(dialector, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	if db.Dialector.Name() == "postgres" {
		var databaseName string
		require.NoError(t, db.Raw("select current_database()").Scan(&databaseName).Error)
		require.True(t, strings.HasPrefix(databaseName, "powerx_eventfabric_test_"), "refuse non-test database")
		schemaName := core.PowerXSchema
		require.NoError(t, db.Exec(`CREATE SCHEMA "`+schemaName+`"`).Error)
		t.Cleanup(func() { require.NoError(t, db.Exec(`DROP SCHEMA "`+schemaName+`" CASCADE`).Error) })
	}
	require.NoError(t, db.AutoMigrate(&cm.CapabilityRecord{}, &cm.CapabilityRegistration{}, &setting.PluginInstanceConfig{}, &m.TopicDefinition{}, &m.AclBinding{}, &m.EventEnvelope{}, &m.DeliveryAttempt{}, &m.DlqMessage{}))
	f := &fixture{db: db, svc: NewService(db), now: time.Now().UTC()}
	f.svc.usageGuard = nil // 排空服务有独立回归；本套只依赖事件及授权表。
	f.svc.clock = func() time.Time { return f.now }
	for _, id := range []string{PublishCapability, ConsumeCapability} {
		require.NoError(t, db.Create(&cm.CapabilityRecord{CapabilityID: id, PluginID: "corex.platform", PluginVersion: "test", Title: id, Status: "published"}).Error)
		require.NoError(t, db.Create(&cm.CapabilityRegistration{CapabilityID: id, TenantUUID: tenant, Status: "published", Version: 1, ContractRef: Contract, RoutingPolicyID: uuid.New()}).Error)
	}
	for _, plugin := range []string{producer, consumer} {
		raw, _ := json.Marshal(map[string]any{"client_id": plugin, "allowed_capabilities": []string{PublishCapability, ConsumeCapability}})
		require.NoError(t, db.Create(&setting.PluginInstanceConfig{TenantUUID: tenant, PluginID: plugin, Key: "auth.credentials", Enabled: true, ValueJSON: datatypes.JSON(raw)}).Error)
	}
	f.topic = &m.TopicDefinition{TenantKey: tenant, Namespace: "_topic.scrm", Name: "changed", Lifecycle: "active", PayloadFormat: "json", MaxRetry: 2, AckTimeoutSec: 1}
	require.NoError(t, db.Create(f.topic).Error)
	f.bind(t, producer, "publish")
	f.bind(t, consumer, "subscribe")
	return f
}
func (f *fixture) bind(t *testing.T, plugin, action string) {
	t.Helper()
	require.NoError(t, f.db.Create(&m.AclBinding{TenantKey: tenant, TopicUUID: f.topic.UUID, PrincipalType: "plugin", PrincipalID: plugin, Action: action, Status: 1}).Error)
}
func ctxFor(plugin string) context.Context {
	ctx := reqctx.WithTenantUUID(context.Background(), tenant)
	ctx = reqctx.WithClaims(ctx, &reqctx.CoreXClaims{TenantUUID: tenant, PluginID: plugin, RegisteredClaims: jwt.RegisteredClaims{Issuer: "powerx-sts", Subject: "client:" + plugin, Audience: jwt.ClaimStrings{"powerx:api"}, ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}})
	return reqctx.WithSTSIssuanceStatus(ctx, 0)
}
func (f *fixture) pub(id string) Request {
	return Request{ContractVersion: Contract, Operation: "publish", TenantUUID: tenant, PluginID: producer, Event: &Event{EventID: id, IdempotencyKey: "key-" + id, Topic: f.topic.FullTopic, Version: "v1", Payload: json.RawMessage(`{"endpoint_uuid":"lab"}`)}}
}
func (f *fixture) sub() Request {
	return Request{ContractVersion: Contract, Operation: "subscribe", TenantUUID: tenant, PluginID: consumer, SubscriberID: consumer, Topics: []string{f.topic.FullTopic}, BatchSize: 10, SupportedVersions: []string{"v1"}, CompatibilityMode: "strict"}
}
func settle(in Request, id, operation string) Request {
	in.Operation = operation
	in.DeliveryID = id
	return in
}
func code(t *testing.T, want string, err error) {
	t.Helper()
	require.Error(t, err)
	require.Equal(t, want, dto.CodeOf(err), "%v", err)
}

func TestPublishDurableFanoutIdempotencyAndAck(t *testing.T) {
	f := setup(t)
	in := f.pub("event-one")
	result, err := f.svc.Publish(ctxFor(producer), in)
	require.NoError(t, err)
	require.Equal(t, true, result["accepted"])
	_, err = f.svc.Publish(ctxFor(producer), in)
	require.NoError(t, err)
	var count int64
	require.NoError(t, f.db.Model(&m.EventEnvelope{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
	items, err := f.svc.Poll(ctxFor(consumer), f.sub())
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "event-one", items[0].EventID)
	require.NotEmpty(t, items[0].TraceID)
	require.Equal(t, "sts:"+producer+":client:"+producer, items[0].PublishedBy)
	_, err = f.svc.Settle(ctxFor(consumer), settle(f.sub(), items[0].DeliveryID, "ack"))
	require.NoError(t, err)
	_, err = f.svc.Settle(ctxFor(consumer), settle(f.sub(), items[0].DeliveryID, "ack"))
	require.NoError(t, err)
	items, err = f.svc.Poll(ctxFor(consumer), f.sub())
	require.NoError(t, err)
	require.Empty(t, items)
	var envelope m.EventEnvelope
	require.NoError(t, f.db.First(&envelope).Error)
	require.Equal(t, "succeeded", envelope.Status)
	in.Event.Payload = json.RawMessage(`{"changed":true}`)
	_, err = f.svc.Publish(ctxFor(producer), in)
	code(t, "EVENT_FABRIC_IDEMPOTENCY_CONFLICT", err)
}

func TestAuthenticatedTenantOwnerAndReservedIdentity(t *testing.T) {
	f := setup(t)
	_, err := f.svc.Publish(context.Background(), f.pub("anonymous"))
	code(t, "EVENT_FABRIC_SERVICE_REQUIRED", err)
	admin := reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), tenant), &reqctx.CoreXClaims{TenantUUID: tenant, UserID: 1, IsRoot: true})
	_, err = f.svc.Publish(admin, f.pub("admin"))
	code(t, "EVENT_FABRIC_SERVICE_REQUIRED", err)
	for _, tc := range []struct {
		name, want string
		change     func(*Request)
	}{
		{"tenant", "EVENT_FABRIC_TENANT_FORBIDDEN", func(r *Request) { r.TenantUUID = uuid.NewString() }},
		{"owner", "EVENT_FABRIC_OWNER_FORBIDDEN", func(r *Request) { r.PluginID = consumer }},
		{"contract", "EVENT_FABRIC_CONTRACT_REQUIRED", func(r *Request) { r.ContractVersion = "" }},
		{"principal", "EVENT_FABRIC_RESERVED_ATTRIBUTE", func(r *Request) { r.Event.Attributes = map[string]string{"principal_id": producer} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := f.pub(tc.name)
			tc.change(&in)
			_, e := f.svc.Publish(ctxFor(producer), in)
			code(t, tc.want, e)
		})
	}
	_, err = f.svc.Publish(reqctx.WithSTSIssuanceStatus(ctxFor(producer), 401), f.pub("proof"))
	code(t, "EVENT_FABRIC_SERVICE_REQUIRED", err)
	in := f.sub()
	in.SubscriberID = producer
	_, err = f.svc.Poll(ctxFor(consumer), in)
	code(t, "EVENT_FABRIC_SUBSCRIBER_FORBIDDEN", err)
}

func TestRevocationBeforeDeliveryAndDuringStream(t *testing.T) {
	f := setup(t)
	for _, id := range []string{"first", "second"} {
		_, err := f.svc.Publish(ctxFor(producer), f.pub(id))
		require.NoError(t, err)
	}
	sent := 0
	err := f.svc.Stream(ctxFor(consumer), f.sub(), func(kind string, _ any) error {
		if kind == "delivery" {
			sent++
			require.NoError(t, f.db.Model(&m.AclBinding{}).Where("principal_id = ? AND action = ?", consumer, "subscribe").Update("status", 0).Error)
		}
		return nil
	})
	code(t, "EVENT_FABRIC_TOPIC_FORBIDDEN", err)
	require.Equal(t, 1, sent)
	_, err = f.svc.Poll(ctxFor(consumer), f.sub())
	code(t, "EVENT_FABRIC_TOPIC_FORBIDDEN", err)
	require.NoError(t, f.db.Model(&m.AclBinding{}).Where("principal_id = ?", consumer).Update("status", 1).Error)
	require.NoError(t, f.db.Model(&cm.CapabilityRegistration{}).Where("capability_id = ?", ConsumeCapability).Update("status", "disabled").Error)
	_, err = f.svc.Poll(ctxFor(consumer), f.sub())
	code(t, "EVENT_FABRIC_GRANT_REQUIRED", err)
}

func TestDisconnectRestartRedeliveryAndStaleReceipt(t *testing.T) {
	f := setup(t)
	_, err := f.svc.Publish(ctxFor(producer), f.pub("restart"))
	require.NoError(t, err)
	items, err := f.svc.Poll(ctxFor(consumer), f.sub())
	require.NoError(t, err)
	require.Len(t, items, 1)
	old := items[0]
	items, err = f.svc.Poll(ctxFor(consumer), f.sub())
	require.NoError(t, err)
	require.Empty(t, items)
	f.now = f.now.Add(2 * time.Second)
	_, err = f.svc.Settle(ctxFor(consumer), settle(f.sub(), old.DeliveryID, "ack"))
	code(t, "EVENT_FABRIC_RECEIPT_STALE", err)
	f.svc = NewService(f.db)
	f.svc.clock = func() time.Time { return f.now }
	items, err = f.svc.Poll(ctxFor(consumer), f.sub())
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, old.EventID, items[0].EventID)
	require.NotEqual(t, old.DeliveryID, items[0].DeliveryID)
	require.Equal(t, 2, items[0].Attempt)
	_, err = f.svc.Settle(ctxFor(consumer), settle(f.sub(), old.DeliveryID, "ack"))
	code(t, "EVENT_FABRIC_RECEIPT_STALE", err)
	foreign := f.sub()
	foreign.SubscriberID = producer
	foreign.PluginID = producer
	_, err = f.svc.Settle(ctxFor(producer), settle(foreign, items[0].DeliveryID, "ack"))
	code(t, "EVENT_FABRIC_DELIVERY_FORBIDDEN", err)
	_, err = f.svc.Settle(ctxFor(consumer), settle(f.sub(), items[0].DeliveryID, "ack"))
	require.NoError(t, err)
}

func TestNackTimeoutExhaustionAndNoDuplicateDeadLetter(t *testing.T) {
	f := setup(t)
	_, err := f.svc.Publish(ctxFor(producer), f.pub("nack"))
	require.NoError(t, err)
	items, err := f.svc.Poll(ctxFor(consumer), f.sub())
	require.NoError(t, err)
	require.Len(t, items, 1)
	_, err = f.svc.Settle(ctxFor(consumer), settle(f.sub(), items[0].DeliveryID, "nack"))
	require.NoError(t, err)
	_, err = f.svc.Settle(ctxFor(consumer), settle(f.sub(), items[0].DeliveryID, "nack"))
	code(t, "EVENT_FABRIC_RECEIPT_STALE", err)
	f.now = f.now.Add(2 * time.Second)
	items, err = f.svc.Poll(ctxFor(consumer), f.sub())
	require.NoError(t, err)
	require.Len(t, items, 1)
	f.now = f.now.Add(2 * time.Second)
	items, err = f.svc.Poll(ctxFor(consumer), f.sub())
	require.NoError(t, err)
	require.Empty(t, items)
	items, err = f.svc.Poll(ctxFor(consumer), f.sub())
	require.NoError(t, err)
	require.Empty(t, items)
	var count int64
	require.NoError(t, f.db.Model(&m.DlqMessage{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
}

func TestPublishRollbackAndDependencyFailure(t *testing.T) {
	f := setup(t)
	require.NoError(t, f.db.Callback().Create().Before("gorm:create").Register("test.fail_attempt", func(db *gorm.DB) {
		if db.Statement.Table == core.TableEventDeliveryAttempts {
			db.AddError(errors.New("injected"))
		}
	}))
	_, err := f.svc.Publish(ctxFor(producer), f.pub("rollback"))
	code(t, "EVENT_FABRIC_UNAVAILABLE", err)
	var count int64
	require.NoError(t, f.db.Model(&m.EventEnvelope{}).Count(&count).Error)
	require.Zero(t, count)
	require.NoError(t, f.db.Callback().Create().Remove("test.fail_attempt"))
	require.NoError(t, f.db.Migrator().DropTable(&m.AclBinding{}))
	_, err = f.svc.Publish(ctxFor(producer), f.pub("acl-down"))
	code(t, "EVENT_FABRIC_AUTHORIZATION_UNAVAILABLE", err)
}

func TestAPIKeyRequiresExactGrantAndPluginOwner(t *testing.T) {
	f := setup(t)
	require.NoError(t, f.db.AutoMigrate(&gw.IntegrationGatewayAPIKey{}, &gw.IntegrationGatewayAPIKeyPermission{}, &iam.Permission{}))
	key := &gw.IntegrationGatewayAPIKey{TenantUUID: tenant, Name: "isolated-event-test", KeyPrefix: "test", KeyHash: "test-only-hash", Status: "active", PluginOwnerPolicy: datatypes.JSON(`{"version":1,"mode":"key","plugin_ids":["` + producer + `"]}`)}
	key.UUID = uuid.New()
	require.NoError(t, f.db.Create(key).Error)
	permission := &iam.Permission{Module: "event_fabric", Resource: "service_publish", Action: "publish", Status: iam.PermissionStatusActive, Effect: "allow", AllowAPIKey: true, Meta: datatypes.JSON(`{"capability_id":"` + PublishCapability + `","api_key_explicit":true,"api_key":{"scope":"_scope.event_fabric.service.publish","action":"publish","resource_type":"capability","resource_pattern":"event_fabric_service_publish","effect":"allow"}}`)}
	require.NoError(t, f.db.Create(permission).Error)
	ctx := reqctx.WithAuthenticatedAPIKeyHash(reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), tenant), &reqctx.CoreXClaims{TenantUUID: tenant, Platforms: []string{"api_key"}}), key.KeyHash)
	_, err := f.svc.Publish(ctx, f.pub("key"))
	code(t, "EVENT_FABRIC_GRANT_REQUIRED", err)
	grant := &gw.IntegrationGatewayAPIKeyPermission{APIKeyUUID: key.UUID, Scope: "_scope.event_fabric.service.publish", Action: "publish", ResourceType: "capability", ResourcePattern: "event_fabric_service_publish", Effect: "allow"}
	require.NoError(t, f.db.Create(grant).Error)
	_, err = f.svc.Publish(ctx, f.pub("key"))
	require.NoError(t, err)
	access, err := cap.NewGrantStatusService(f.db).CurrentAccess(ctx)
	require.NoError(t, err)
	require.NoError(t, access.Require(PublishCapability))
	require.Error(t, access.Require(ConsumeCapability))
	var credential setting.PluginInstanceConfig
	require.NoError(t, f.db.Where("tenant_uuid = ? AND plugin_id = ?", tenant, producer).First(&credential).Error)
	var value map[string]any
	require.NoError(t, json.Unmarshal(credential.ValueJSON, &value))
	value["config"] = map[string]any{"runtime": map[string]any{"event_bridge": map[string]any{"tenant_uuid": tenant, "topic_bindings": map[string]string{"scrm.changed": f.topic.FullTopic}}}}
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	require.NoError(t, f.db.Model(&credential).Update("value_json", raw).Error)
	resolve := Request{ContractVersion: Contract, Operation: "resolve_topics", TenantUUID: tenant, PluginID: producer, TopicKeys: []string{"scrm.changed"}}
	resolved, err := f.svc.ResolveTopics(ctx, resolve, PublishCapability)
	require.NoError(t, err)
	require.Equal(t, map[string]string{"scrm.changed": f.topic.FullTopic}, resolved["topic_bindings"])
	require.NoError(t, f.db.Delete(grant).Error)
	_, err = f.svc.Publish(ctx, f.pub("key-revoked"))
	code(t, "EVENT_FABRIC_GRANT_REQUIRED", err)
	_, err = f.svc.ResolveTopics(ctx, resolve, PublishCapability)
	code(t, "EVENT_FABRIC_GRANT_REQUIRED", err)
}

func TestConcurrentClaimsOneActiveReceipt(t *testing.T) {
	if os.Getenv("POWERX_EVENTFABRIC_TEST_DSN") == "" {
		t.Skip("row-lock concurrency is verified on isolated PostgreSQL")
	}
	f := setup(t)
	_, err := f.svc.Publish(ctxFor(producer), f.pub("concurrent"))
	require.NoError(t, err)
	start := make(chan struct{})
	results := make(chan []Message, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for n := 0; n < 2; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			rows, e := f.svc.Poll(ctxFor(consumer), f.sub())
			results <- rows
			errs <- e
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	close(errs)
	for e := range errs {
		require.NoError(t, e)
	}
	count := 0
	for rows := range results {
		count += len(rows)
	}
	require.Equal(t, 1, count)
}

func TestInvokerRejectsRawProxyAndRecoversAfterSendFailure(t *testing.T) {
	f := setup(t)
	i := &Invoker{Service: f.svc}
	raw, _ := json.Marshal(f.pub("stream"))
	var body map[string]any
	require.NoError(t, json.Unmarshal(raw, &body))
	in := cap.CoreCapabilityInvokeInput{CapabilityID: PublishCapability, TenantUUID: tenant, Endpoint: Endpoint, Method: "INVOKE", Body: body, Payload: map[string]any{"body": body}}
	bad := in
	bad.Endpoint = "http://example.invalid"
	_, err := i.InvokeCoreCapability(ctxFor(producer), bad)
	code(t, "EVENT_FABRIC_INVALID_ARGUMENT", err)
	bad = in
	bad.Body = map[string]any{"operation": "publish", "headers": map[string]string{"principal_id": producer}}
	_, err = i.InvokeCoreCapability(ctxFor(producer), bad)
	code(t, "EVENT_FABRIC_INVALID_ARGUMENT", err)
	_, err = i.InvokeCoreCapability(ctxFor(producer), in)
	require.NoError(t, err)
	sent := 0
	err = f.svc.Stream(ctxFor(consumer), f.sub(), func(kind string, _ any) error {
		if kind == "delivery" {
			sent++
			return errors.New("disconnected")
		}
		return nil
	})
	require.Error(t, err)
	require.Equal(t, 1, sent)
	f.now = f.now.Add(2 * time.Second)
	rows, err := f.svc.Poll(ctxFor(consumer), f.sub())
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, 2, rows[0].Attempt)
}

type auditCapture struct{ rows []eventaudit.Record }

func (a *auditCapture) Write(_ context.Context, r eventaudit.Record) error {
	a.rows = append(a.rows, r)
	return nil
}

func TestLiveCredentialRevocationAndSanitizedAudit(t *testing.T) {
	f := setup(t)
	audit := &auditCapture{}
	f.svc.audit = audit
	result, err := f.svc.Publish(ctxFor(producer), f.pub("audit"))
	require.NoError(t, err)
	require.Len(t, audit.rows, 1)
	require.Equal(t, result["trace_id"], audit.rows[0].TraceID)
	require.Equal(t, tenant, audit.rows[0].TenantID)
	require.NotEmpty(t, audit.rows[0].PrincipalID)
	raw, _ := json.Marshal(map[string]any{"client_id": producer, "allowed_capabilities": []string{ConsumeCapability}})
	require.NoError(t, f.db.Model(&setting.PluginInstanceConfig{}).Where("plugin_id = ?", producer).Update("value_json", datatypes.JSON(raw)).Error)
	_, err = f.svc.Publish(ctxFor(producer), f.pub("revoked"))
	code(t, "EVENT_FABRIC_GRANT_REQUIRED", err)
	require.Equal(t, "FAILED", audit.rows[1].Status)
	require.Equal(t, "EVENT_FABRIC_GRANT_REQUIRED", audit.rows[1].ErrorMessage)
	serialized, _ := json.Marshal(audit.rows)
	require.NotContains(t, string(serialized), "endpoint_uuid")
	require.NotContains(t, string(serialized), "allowed_capabilities")
	require.NotContains(t, string(serialized), "Bearer")
}

func TestNackRollbackLeavesOriginalReceiptValid(t *testing.T) {
	f := setup(t)
	_, err := f.svc.Publish(ctxFor(producer), f.pub("nack-rollback"))
	require.NoError(t, err)
	rows, err := f.svc.Poll(ctxFor(consumer), f.sub())
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.NoError(t, f.db.Callback().Create().Before("gorm:create").Register("test.reject_next", func(db *gorm.DB) {
		if db.Statement.Table == core.TableEventDeliveryAttempts {
			db.AddError(errors.New("injected next attempt failure"))
		}
	}))
	_, err = f.svc.Settle(ctxFor(consumer), settle(f.sub(), rows[0].DeliveryID, "nack"))
	code(t, "EVENT_FABRIC_UNAVAILABLE", err)
	require.NoError(t, f.db.Callback().Create().Remove("test.reject_next"))
	_, err = f.svc.Settle(ctxFor(consumer), settle(f.sub(), rows[0].DeliveryID, "ack"))
	require.NoError(t, err)
}

func TestSamePluginDifferentCredentialCannotSettleReceipt(t *testing.T) {
	f := setup(t)
	require.NoError(t, f.db.AutoMigrate(&gw.IntegrationGatewayAPIKey{}, &gw.IntegrationGatewayAPIKeyPermission{}, &iam.Permission{}))
	key := &gw.IntegrationGatewayAPIKey{TenantUUID: tenant, Name: "same-owner-other-subject", KeyPrefix: "test", KeyHash: "test-other-credential", Status: "active", PluginOwnerPolicy: datatypes.JSON(`{"version":1,"mode":"key","plugin_ids":["` + consumer + `"]}`)}
	key.UUID = uuid.New()
	require.NoError(t, f.db.Create(key).Error)
	require.NoError(t, f.db.Create(&iam.Permission{Module: "event_fabric", Resource: "service_consume", Action: "consume", Status: iam.PermissionStatusActive, Effect: "allow", AllowAPIKey: true, Meta: datatypes.JSON(`{"capability_id":"` + ConsumeCapability + `","api_key_explicit":true,"api_key":{"scope":"_scope.event_fabric.service.consume","action":"consume","resource_type":"capability","resource_pattern":"event_fabric_service_consume","effect":"allow"}}`)}).Error)
	require.NoError(t, f.db.Create(&gw.IntegrationGatewayAPIKeyPermission{APIKeyUUID: key.UUID, Scope: "_scope.event_fabric.service.consume", Action: "consume", ResourceType: "capability", ResourcePattern: "event_fabric_service_consume", Effect: "allow"}).Error)
	other := reqctx.WithAuthenticatedAPIKeyHash(reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), tenant), &reqctx.CoreXClaims{TenantUUID: tenant, Platforms: []string{"api_key"}}), key.KeyHash)
	_, err := f.svc.Publish(ctxFor(producer), f.pub("subject-owner"))
	require.NoError(t, err)
	rows, err := f.svc.Poll(ctxFor(consumer), f.sub())
	require.NoError(t, err)
	require.Len(t, rows, 1)
	_, err = f.svc.Settle(other, settle(f.sub(), rows[0].DeliveryID, "ack"))
	code(t, "EVENT_FABRIC_DELIVERY_FORBIDDEN", err)
	f.now = f.now.Add(2 * time.Second)
	newRows, err := f.svc.Poll(other, f.sub())
	require.NoError(t, err)
	require.Len(t, newRows, 1)
	_, err = f.svc.Settle(ctxFor(consumer), settle(f.sub(), newRows[0].DeliveryID, "ack"))
	code(t, "EVENT_FABRIC_DELIVERY_FORBIDDEN", err)
	_, err = f.svc.Settle(other, settle(f.sub(), newRows[0].DeliveryID, "ack"))
	require.NoError(t, err)
}

func TestSchemaPreflightRequiresFormalMigration(t *testing.T) {
	f := setup(t)
	require.NoError(t, f.db.Migrator().DropColumn(&m.DeliveryAttempt{}, "ClaimedBy"))
	svc := NewService(f.db)
	svc.usageGuard = nil
	_, err := svc.Publish(ctxFor(producer), f.pub("unmigrated"))
	code(t, "EVENT_FABRIC_SCHEMA_NOT_READY", err)
	require.NoError(t, database.MigrateEventServiceDeliveryIdentity(f.db))
	require.NoError(t, database.MigrateEventServiceDeliveryIdentity(f.db))
	svc = NewService(f.db)
	svc.usageGuard = nil
	_, err = svc.Publish(ctxFor(producer), f.pub("migrated"))
	require.NoError(t, err)
}
