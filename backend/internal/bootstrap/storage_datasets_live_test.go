package bootstrap

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	sqlpkg "database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	stsv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/auth/sts/v1"
	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	budget "github.com/ArtisanCloud/PowerX/internal/infra/storage_budget"
	observer "github.com/ArtisanCloud/PowerX/internal/infra/storage_observer"
	archiveOps "github.com/ArtisanCloud/PowerX/internal/service/archive_ops"
	capservice "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	caprouter "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/router"
	taskqueue "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/task_queue"
	setting "github.com/ArtisanCloud/PowerX/internal/service/setting"
	gov "github.com/ArtisanCloud/PowerX/internal/service/storage_governance"
	authgrpc "github.com/ArtisanCloud/PowerX/internal/transport/grpc/auth"
	storagehttp "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/storage"
	caphttp "github.com/ArtisanCloud/PowerX/internal/transport/http/openapi/capability_registry"
	"github.com/ArtisanCloud/PowerX/pkg/auth"
	"github.com/ArtisanCloud/PowerX/pkg/auth/middleware"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/database"
	core "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	cm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	em "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	im "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	sm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	tm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	capRepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/capability_registry"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	pm "github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gorm.io/datatypes"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestStorageDatasetsNativeSTSAndHTTP(t *testing.T) {
	t.Run("managed", func(t *testing.T) { exerciseNativeDatasetLifecycle(t, "") })
	if external := os.Getenv("POWERX_STORAGE_DATASET_TEST_EXTERNAL_DSN"); external != "" {
		t.Run("independent-postgres", func(t *testing.T) { exerciseNativeDatasetLifecycle(t, external) })
	}
}

func exerciseNativeDatasetLifecycle(t *testing.T, external string) {
	dsn := os.Getenv("POWERX_STORAGE_CONTROL_TEST_DSN")
	if dsn == "" {
		t.Skip("requires owned temporary PostgreSQL")
	}
	parsed, err := pgx.ParseConfig(dsn)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(parsed.Host, "/tmp/powerx-control-owned-"), "refuse non-fixture PostgreSQL")
	require.Equal(t, "storage_control_test", parsed.User)
	require.Equal(t, "postgres", parsed.Database)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), DisableForeignKeyConstraintWhenMigrating: true})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	sourceDB := db
	if external != "" {
		private, err := pgx.ParseConfig(external)
		require.NoError(t, err)
		require.Equal(t, parsed.Host, private.Host)
		require.Equal(t, parsed.User, private.User)
		require.Equal(t, parsed.Database, private.Database)
		require.NotEqual(t, parsed.Port, private.Port)
		sourceDB, err = gorm.Open(postgres.Open(external), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		require.NoError(t, err)
		source, err := sourceDB.DB()
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, source.Close()) })
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")
	previous := core.PowerXSchema
	core.PowerXSchema = "dataset_core_" + suffix
	t.Cleanup(func() { core.PowerXSchema = previous })
	pluginSchema := "dataset_plugin_" + suffix
	pluginRole := "dataset_reader_" + suffix
	require.NoError(t, db.Exec("CREATE SCHEMA "+core.PowerXSchema).Error)
	t.Cleanup(func() { require.NoError(t, db.Exec("DROP SCHEMA "+core.PowerXSchema+" CASCADE").Error) })
	require.NoError(t, sourceDB.Exec("CREATE SCHEMA "+pluginSchema).Error)
	t.Cleanup(func() { require.NoError(t, sourceDB.Exec("DROP SCHEMA "+pluginSchema+" CASCADE").Error) })
	require.NoError(t, database.MigrateStorageGovernanceModels(db))
	require.NoError(t, db.AutoMigrate(&em.TaskHistory{}, &tm.Tenant{}, &sm.PluginInstanceConfig{}, &sm.PluginDrainJob{}, &im.User{}, &im.Member{}, &cm.HealthProbeResult{},
		&cm.CapabilityRecord{}, &cm.CapabilityRegistration{}, &cm.AdapterEndpoint{}, &cm.RoutingPolicy{}, &cm.FallbackPlan{}, &cm.InvocationTrace{}))
	require.NoError(t, sourceDB.Exec("CREATE ROLE "+pluginRole).Error)
	// Role and privileges are fixture-only. Revoke residual dependent grants
	// before dropping the role, and the parent instance is destroyed afterwards.
	t.Cleanup(func() {
		require.NoError(t, sourceDB.Exec("DROP OWNED BY "+pluginRole).Error)
		require.NoError(t, sourceDB.Exec("DROP ROLE "+pluginRole).Error)
	})
	tables := datasetFixtureTables{reports: pluginSchema + ".fixture_reports",
		assertions: pluginSchema + ".fixture_assertions", refs: pluginSchema + ".fixture_references"}
	require.NoError(t, sourceDB.Exec("CREATE TABLE "+tables.reports+" (id integer, tenant_uuid uuid NOT NULL, finished_at timestamptz NOT NULL, case_key integer, expected_assertions integer, format_version integer)").Error)
	require.NoError(t, sourceDB.Exec("CREATE INDEX ON "+tables.reports+" (tenant_uuid, finished_at, id)").Error)
	require.NoError(t, sourceDB.Exec("CREATE INDEX ON "+tables.reports+" (tenant_uuid, case_key, finished_at, id)").Error)
	require.NoError(t, sourceDB.Exec("CREATE TABLE "+tables.assertions+" (tenant_uuid uuid, report_id integer, assertion_id integer, PRIMARY KEY (tenant_uuid, report_id, assertion_id))").Error)
	require.NoError(t, sourceDB.Exec("CREATE TABLE "+tables.refs+" (tenant_uuid uuid, report_id integer, PRIMARY KEY (tenant_uuid, report_id))").Error)
	grantSource := func() {
		require.NoError(t, sourceDB.Exec("GRANT USAGE ON SCHEMA "+pluginSchema+" TO "+pluginRole).Error)
		require.NoError(t, sourceDB.Exec("GRANT SELECT ON "+tables.reports+", "+tables.assertions+", "+tables.refs+" TO "+pluginRole).Error)
	}
	grantSource()
	members := []string{"fixture_reports", "fixture_assertions", "fixture_references"}

	const pluginID = "com.powerx.plugins.storage-fixture"
	installation := pm.Plugin{ID: pluginID, Revision: uuid.NewString(), State: pm.StateEnabled, Version: "0.0.1",
		Paths: pm.InstalledPaths{Root: t.TempDir()}, DataLifecycle: map[string]pm.DatasetObservationSpec{}}
	dir := filepath.Join(installation.Paths.Root, "contracts", "storage-datasets")
	require.NoError(t, os.MkdirAll(dir, 0700))
	for _, key := range []string{"fixture_report", "fixture_alias"} {
		raw := []byte(fmt.Sprintf(`{"dataset_key":%q,"display_name_i18n":{"zh-CN":"合成执行报告","en":"Synthetic report"},"contract_version":1,"schema_contract_version":1,"source_binding_ref":"database","tenant_scope":"tenant","adapter_key":"fixture","adapter_version":1,"allowed_actions":["observe"]}`, key))
		if key == "fixture_report" {
			raw = bytes.Replace(raw, []byte(`["observe"]`), []byte(`["observe","preview","delete","archive"],"retention_bounds":{"minimum_days":7,"suggested_days":30,"maximum_days":3650}`), 1)
		}
		digest := sha256.Sum256(raw)
		require.NoError(t, os.WriteFile(filepath.Join(dir, key+".json"), raw, 0600))
		installation.DataLifecycle[key] = pm.DatasetObservationSpec{SHA256: hex.EncodeToString(digest[:]), Tables: members}
	}
	binding := uuid.New()
	var externalBindings map[string]gov.DatasetExternalSource
	if external != "" {
		const dsnEnv = "POWERX_STORAGE_DATASET_TEST_EXTERNAL_DSN"
		externalBindings, err = datasetExternalSources([]config.StoragePluginPostgresSource{{PluginID: pluginID, BindingUUID: binding.String(),
			ClusterID: "owned-independent-postgres", Schema: pluginSchema, WorkerRole: pluginRole, ObserverDSNEnv: dsnEnv}})
		require.NoError(t, err)
		// Use the actual deployment resolver and assert its connection is
		// distinct and default read-only before using it for catalog queries.
		readonly, closeObserver, err := externalBindings[pluginID].Open()
		require.NoError(t, err)
		var readOnlyMode string
		require.NoError(t, readonly.Raw("SHOW transaction_read_only").Scan(&readOnlyMode).Error)
		closeObserver()
		require.Equal(t, "on", readOnlyMode)
	}
	var queue *taskqueue.StreamDriver
	var datasetTasks *taskqueue.RuntimeService
	redisFault := &datasetRedisFault{}
	if socket := os.Getenv("POWERX_STORAGE_DATASET_TEST_REDIS_SOCKET"); socket != "" {
		require.Equal(t, filepath.Join(parsed.Host, "redis.sock"), socket)
		client := redis.NewClient(&redis.Options{Network: "unix", Addr: socket})
		t.Cleanup(func() { require.NoError(t, client.Close()) })
		require.NoError(t, client.Ping(ctx).Err())
		client.AddHook(redisFault)
		stream, err := event_bus.NewRedisStreamTaskDriver(client, 5*time.Second, "owned-datasets")
		require.NoError(t, err)
		queue = taskqueue.NewStreamDriverWithRetryDelay(db, stream, time.Millisecond)
		datasetTasks = taskqueue.NewRuntimeService(queue)
	}
	fixtureDir := t.TempDir()
	hostBudget, err := budget.New(budget.Config{Directory: filepath.Join(fixtureDir, "runtime"), FilesystemPath: fixtureDir,
		HostID: "fixture-host", FilesystemID: "fixture-fs", InstanceID: "owned-datasets", MinFreeBytes: 1 << 20, CriticalFreeBytes: 1})
	require.NoError(t, err)
	scope := gov.ControlScope{HostID: "fixture-host", FilesystemID: "fixture-fs", InstanceID: "owned-datasets"}
	control, err := gov.NewControlService(db, gov.ControlOptions{Scope: scope, Queue: queue})
	require.NoError(t, err)
	service, err := gov.NewService(gov.Options{
		Identity: gov.Identity{HostID: "fixture-host", FilesystemID: "fixture-fs", InstanceID: "owned-datasets", Environment: "test"}, FilesystemPath: fixtureDir,
		Collector: observer.Collector{}, DB: db, DatasetTasks: datasetTasks, DatasetExecutionEnabled: queue != nil, ControlWorkerEnabled: queue != nil, Control: control, Budget: hostBudget, DatabaseCollector: observer.PostgresCollector{DB: db, ClusterID: "owned"},
		DatasetSources: &gov.DatasetSources{ClusterID: "owned", External: externalBindings,
			Installed: func(_ context.Context, id string) (pm.Plugin, error) {
				if id != pluginID {
					return pm.Plugin{}, fmt.Errorf("fixture plugin unavailable")
				}
				return installation, nil
			},
			Binding: func(pm.Plugin) (uuid.UUID, string, string, error) { return binding, pluginSchema, pluginRole, nil },
		},
	})
	require.NoError(t, err)

	archiveTarget := ""
	if configPath := os.Getenv("POWERX_STORAGE_DATASET_TEST_S3_CONFIG"); configPath != "" && queue != nil {
		require.Equal(t, filepath.Join(parsed.Host, "s3.json"), configPath)
		archiveTarget = fixtureDatasetArchiveTarget(t, ctx, fixtureDir, configPath, service, control, queue, scope)
	}

	tenant := tm.Tenant{Key: "datasets-fixture", Name: "datasets fixture", Status: 1}
	require.NoError(t, db.Create(&tenant).Error)
	otherTenant := tm.Tenant{Key: "datasets-other", Name: "datasets other", Status: 1}
	require.NoError(t, db.Create(&otherTenant).Error)
	for i, item := range []struct{ businessCase, format int }{{1, 1}, {1, 1}, {2, 1}, {3, 1}, {4, 2}} {
		require.NoError(t, sourceDB.Exec("INSERT INTO "+tables.reports+" VALUES (?, ?, NOW() - INTERVAL '45 days', ?, 1, ?)", i+1, tenant.UUID, item.businessCase, item.format).Error)
		if i != 3 {
			require.NoError(t, sourceDB.Exec("INSERT INTO "+tables.assertions+" VALUES (?, ?, 1)", tenant.UUID, i+1).Error)
		}
	}
	require.NoError(t, sourceDB.Exec("INSERT INTO "+tables.reports+" VALUES (6, ?, NOW(), 1, 1, 1), (8, ?, NOW() - INTERVAL '60 days', 1, 0, 1)", tenant.UUID, otherTenant.UUID).Error)
	require.NoError(t, sourceDB.Exec("INSERT INTO "+tables.reports+" VALUES (7, ?, NOW() - INTERVAL '44 days', 1, 1, 1)", tenant.UUID).Error)
	require.NoError(t, sourceDB.Exec("INSERT INTO "+tables.assertions+" VALUES (?, 7, 1)", tenant.UUID).Error)
	require.NoError(t, sourceDB.Exec("INSERT INTO "+tables.refs+" VALUES (?, 2), (?, 1)", tenant.UUID, otherTenant.UUID).Error)
	capID := gov.DatasetRegisterCapability
	// The isolated HTTP test needs exactly this registry record; the production
	// capability-seed is exercised separately against an owned database.
	require.NoError(t, db.Create(&cm.CapabilityRecord{CapabilityID: capID, PluginID: "corex.platform", PluginVersion: "v1", Title: "Storage datasets", Status: "published"}).Error)
	policy := cm.RoutingPolicy{Strategy: "weighted_round_robin"}
	require.NoError(t, db.Create(&policy).Error)
	registration := cm.CapabilityRegistration{CapabilityID: capID, TenantUUID: tenant.UUID.String(), ContractRef: "v1", Status: "published", Version: 1, RoutingPolicyID: policy.UUID}
	require.NoError(t, db.Create(&registration).Error)
	require.NoError(t, db.Create(&cm.AdapterEndpoint{RegistrationID: registration.ID, CapabilityID: capID, TenantUUID: tenant.UUID.String(),
		AdapterID: "dataset-core-internal", TransportType: "core_internal", Endpoint: "core://storage-governance/datasets", Weight: 1,
		TimeoutMS: 5000, IsActive: true, Labels: datatypes.JSON(`{"method":"INVOKE"}`)}).Error)
	require.NoError(t, db.Create(&cm.CapabilityRecord{CapabilityID: capservice.GrantStatusCapabilityID, PluginID: "corex.platform", PluginVersion: "v1", Title: "Grant status", Status: "published"}).Error)
	require.NoError(t, db.Create(&cm.CapabilityRegistration{CapabilityID: capservice.GrantStatusCapabilityID, TenantUUID: tenant.UUID.String(), ContractRef: "v1", Status: "published", Version: 1, RoutingPolicyID: policy.UUID}).Error)
	for capability, endpoint := range map[string]string{gov.DatasetPreviewCapability: "core://storage-governance/datasets",
		taskqueue.ConsumeCapability: taskqueue.RuntimeEndpoint, taskqueue.EnqueueCapability: taskqueue.RuntimeEndpoint,
		gov.DatasetExecutionCapability: "core://storage-governance/datasets"} {
		require.NoError(t, db.Create(&cm.CapabilityRecord{CapabilityID: capability, PluginID: "corex.platform", PluginVersion: "v1", Title: capability, Status: "published"}).Error)
		for _, tid := range []string{tenant.UUID.String(), otherTenant.UUID.String()} {
			row := cm.CapabilityRegistration{CapabilityID: capability, TenantUUID: tid, ContractRef: "v1", Status: "published", Version: 1, RoutingPolicyID: policy.UUID}
			require.NoError(t, db.Create(&row).Error)
			require.NoError(t, db.Create(&cm.AdapterEndpoint{RegistrationID: row.ID, CapabilityID: capability, TenantUUID: tid,
				AdapterID: capability, TransportType: "core_internal", Endpoint: endpoint, Weight: 1,
				TimeoutMS: 10000, IsActive: true, Labels: datatypes.JSON(`{"method":"INVOKE"}`)}).Error)
		}
	}
	credentials := setting.NewPluginInstanceConfigService(&shared.Deps{DB: db})
	credentialOptions := &setting.ClientCredential{AllowedAudiences: []string{"powerx:api"}, AllowedScopes: []string{"access"}, AllowedCapabilities: []string{capID, capservice.GrantStatusCapabilityID, gov.DatasetPreviewCapability, taskqueue.ConsumeCapability, taskqueue.EnqueueCapability, gov.DatasetExecutionCapability}}
	cid, clientSecret, err := credentials.EnsureCredentials(ctx, tenant.UUID.String(), pluginID, credentialOptions)
	require.NoError(t, err)
	foreignID, foreignSecret, err := credentials.EnsureCredentials(ctx, tenant.UUID.String(), "com.powerx.plugins.other-fixture", credentialOptions)
	require.NoError(t, err)

	otherTenantID, otherTenantSecret, err := credentials.EnsureCredentials(ctx, otherTenant.UUID.String(), pluginID, credentialOptions)
	require.NoError(t, err)

	secret := make([]byte, 32)
	_, err = rand.Read(secret)
	require.NoError(t, err)
	secret = []byte(hex.EncodeToString(secret))
	previousJWT := auth.GetJWTSecret()
	auth.SetJWTSecret(secret)
	t.Cleanup(func() { auth.SetJWTSecret(previousJWT) })
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	ring := authgrpc.NewKeyRing()
	ring.AddHS("default", secret)
	stsServer := grpc.NewServer()
	stsv1.RegisterSTSServiceServer(stsServer, authgrpc.NewSTSServiceServerWithRing(&shared.Deps{DB: db}, ring))
	go stsServer.Serve(listener)
	t.Cleanup(func() { stsServer.Stop(); verifyDatasetFixturePortClosed(t, listener.Addr().String()) })
	conn, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, conn.Close()) })
	exchange := func(id, password string) string {
		out, err := stsv1.NewSTSServiceClient(conn).Exchange(ctx, &stsv1.ExchangeRequest{ClientId: id, ClientSecret: password, Audience: "powerx:api", Scope: "access", TtlSeconds: 300})
		require.NoError(t, err)
		require.EqualValues(t, 200, out.GetMeta().GetCode())
		return out.GetData().GetAccessToken()
	}
	sts, foreign := exchange(cid, clientSecret), exchange(foreignID, foreignSecret)
	root, err := auth.GenerateAccessJWT(reqctx.CoreXClaims{UserID: 1, IsRoot: true, TenantUUID: tenant.UUID.String(), MemberUUID: uuid.NewString()}, "powerx-auth", []string{"user"}, time.Minute, secret)
	require.NoError(t, err)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	authMW := middleware.APIKeyOrJwtMiddleware(db, secret, "powerx-auth", []string{"user"}, []string{"access"}, nil,
		middleware.WithExtraTokenChecks(middleware.TokenCheck{Issuer: "powerx-sts", Audiences: []string{"powerx:api"}}))
	group := engine.Group("/custom/v2", authMW)
	catalog := capservice.NewRegistryService(capservice.RegistryServiceOptions{DB: db})
	traces := capRepo.NewInvocationTraceRepository(db)
	invocation := capservice.NewInvocationService(capservice.InvocationServiceOptions{Catalog: catalog, Router: caprouter.NewService(caprouter.ServiceOptions{DB: db}),
		Audit:       capservice.NewAuditService(capservice.AuditServiceOptions{TraceRepo: traces}),
		CoreInvoker: capservice.NewCoreCapabilityMux(&gov.DatasetRegisterInvoker{Service: func() *gov.Service { return service }}, taskqueue.NewRuntimeInvoker(db, queue))})
	deps := &shared.Deps{DB: db, StorageGovernance: service, CapabilityCatalogSvc: catalog, CapabilityInvocationSvc: invocation}
	caphttp.RegisterTenantRoutes(group, deps)
	storagehttp.RegisterAPIRoutes(nil, group, deps)
	server := httptest.NewServer(engine)
	t.Cleanup(func() { server.Close(); verifyDatasetFixturePortClosed(t, strings.TrimPrefix(server.URL, "http://")) })
	cwd, err := os.Getwd()
	require.NoError(t, err)
	t.Logf("owned dataset fixture pid=%d cwd=%s cmd=go test ./internal/bootstrap -run ^TestStorageDatasetsNativeSTSAndHTTP$ HTTP=%s STS=%s", os.Getpid(), cwd, server.URL, listener.Addr())
	call := func(method, path, token string, body any, want int) []byte {
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		req, err := http.NewRequestWithContext(ctx, method, server.URL+"/custom/v2"+path, bytes.NewReader(raw))
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Trace-ID", uuid.NewString())
		out, err := server.Client().Do(req)
		require.NoError(t, err)
		defer out.Body.Close()
		data, err := io.ReadAll(out.Body)
		require.NoError(t, err)
		require.Equal(t, want, out.StatusCode, "%s", data)
		return data
	}
	op := func(key string) map[string]any {
		return map[string]any{"contract_version": "powerx.storage-dataset-host/v1", "operation": "register", "input": map[string]any{
			"request_uuid": uuid.NewString(), "dataset_key": key, "definition_version": 1, "definition_digest": installation.DataLifecycle[key].SHA256, "source_binding_ref": "database",
		}}
	}
	register := func(token string, in map[string]any, want int) []byte {
		return call("POST", "/tenant/invocations", token, map[string]any{"capability_id": capID, "preferred_protocol": "core_internal", "trace_id": uuid.NewString(), "payload": map[string]any{"body": in}}, want)
	}
	status := call("POST", "/tenant/capabilities:grant-status", sts, map[string]any{"capability_ids": []string{capID}}, 200)
	require.Contains(t, string(status), `"status":"granted"`)
	first := op("fixture_report")
	require.Contains(t, string(register(sts, first, 200)), `"registration_state":"accepted"`)
	require.Contains(t, string(register(sts, first, 200)), `"acceptance_revision":1`)
	require.Contains(t, string(register(sts, op("fixture_alias"), 200)), `"registration_state":"accepted"`)
	register(foreign, op("fixture_report"), 403)
	unsafe := op("fixture_report")
	unsafe["input"].(map[string]any)["tenant_uuid"] = uuid.NewString()
	register(sts, unsafe, 400)
	const base = "/admin/ops/storage/datasets"
	call("GET", base, sts, nil, 403)
	call("GET", base+"?schema=public", root, nil, 400)
	call("GET", base+"/00000000-0000-0000-0000-000000000000", root, nil, 400)
	var page struct {
		Data gov.DatasetPage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(call("GET", base+"?owner_kind=plugin", root, nil, 200), &page))
	require.Len(t, page.Data.Items, 2)
	require.NotNil(t, page.Data.PagePhysicalBytes)
	require.Positive(t, *page.Data.PagePhysicalBytes)
	require.Equal(t, *page.Data.Items[0].Observation.TotalBytes, *page.Data.PagePhysicalBytes)
	previewPath := ""
	if queue != nil {
		var fixture gov.DatasetView
		for _, item := range page.Data.Items {
			if item.DatasetKey == "fixture_report" {
				fixture = item
			}
		}
		require.Contains(t, fixture.AvailableActions, "preview")
		previewPath = exerciseStorageDatasetPreview(t, ctx, sourceDB, call, server, redisFault, tables, pluginRole, pluginID, base+"/"+fixture.DatasetUUID.String(),
			tenant.UUID.String(), sts, exchange(otherTenantID, otherTenantSecret), root, fixture.AcceptanceRevision)
		exerciseDatasetDeletion(t, ctx, sourceDB, call, tables, base+"/"+fixture.DatasetUUID.String(),
			tenant.UUID.String(), sts, exchange(otherTenantID, otherTenantSecret), root, fixture.AcceptanceRevision, previewPath)
		if archiveTarget != "" {
			exerciseDatasetArchive(t, ctx, db, sourceDB, call, tables, base+"/"+fixture.DatasetUUID.String(), tenant.UUID.String(),
				sts, root, fixture.AcceptanceRevision, archiveTarget)
		}
		exerciseDatasetReconciliation(t, ctx, sourceDB, call, tables, base+"/"+fixture.DatasetUUID.String(),
			tenant.UUID.String(), sts, exchange(otherTenantID, otherTenantSecret), root, fixture.AcceptanceRevision, func() {
				const executionControl = "/admin/ops/storage/execution-control"
				call("POST", executionControl+"/changes", root, gov.ExecutionChange{
					RequestID: uuid.NewString(), Action: "pause", Confirm: true, Reason: "Isolated dataset reconciliation",
				}, 200)
				pending, err := queue.Dequeue(ctx, "platform:storage-control", "core.storage.control."+scope.ID(),
					"owned-dataset-config", "test", 1, time.Second)
				require.NoError(t, err)
				require.Len(t, pending, 1)
				require.NoError(t, control.ApplyDelivery(ctx, pending[0]))
				require.Contains(t, string(call("GET", executionControl, root, nil, 200)), `"mode":"paused"`)
			})
	}
	check := base + "/" + page.Data.Items[0].DatasetUUID.String()
	require.Contains(t, string(call("GET", check, root, nil, 200)), `"source_state":"ready"`)
	changedManifest := installation.DataLifecycle["fixture_report"]
	changedManifest.Tables = []string{"other_fixture_table"}
	installation.DataLifecycle["fixture_report"] = changedManifest
	register(sts, op("fixture_report"), 409)
	changedManifest.Tables = members
	installation.DataLifecycle["fixture_report"] = changedManifest
	system := call("GET", base+"?owner_kind=system&available_action=open_existing_policy", root, nil, 200)
	require.Contains(t, string(system), "capability_registry_health_probe_results")
	legacy := call("GET", "/admin/ops/storage/database-tables/capability_registry_health_probe_results", root, nil, 200)
	require.Contains(t, string(legacy), "capability_registry_health_probe_results")
	require.NoError(t, sourceDB.Exec("DROP TABLE "+tables.reports).Error)
	require.NoError(t, sourceDB.Exec("CREATE TABLE "+tables.reports+" (id integer)").Error)
	grantSource()
	if previewPath != "" {
		require.Contains(t, string(call("GET", previewPath, root, nil, 200)), `"state":"invalidated"`)
	}
	require.Contains(t, string(call("GET", check, root, nil, 200)), `"source_state":"needs_revalidation"`)
	require.Contains(t, string(register(sts, op("fixture_report"), 200)), `"registration_state":"needs_revalidation"`)
	require.NoError(t, db.Model(&cm.CapabilityRegistration{}).Where("id = ?", registration.ID).Update("status", "disabled").Error)
	register(sts, op("fixture_report"), 403)
}

func verifyDatasetFixturePortClosed(t *testing.T, address string) {
	t.Helper()
	conn, err := net.DialTimeout("tcp", address, 100*time.Millisecond)
	if err == nil {
		conn.Close()
		t.Errorf("owned listener remained open: %s", address)
	}
}

type datasetFixtureTables struct{ reports, assertions, refs string }

type datasetRedisFault struct {
	offline atomic.Bool
	failed  atomic.Int64
}

func (*datasetRedisFault) DialHook(next redis.DialHook) redis.DialHook { return next }
func (*datasetRedisFault) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}
func (f *datasetRedisFault) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if f.offline.Load() {
			f.failed.Add(1)
			return fmt.Errorf("fixture redis transport unavailable")
		}
		return next(ctx, cmd)
	}
}

type datasetFixtureRoundTripper func(*http.Request) (*http.Response, error)

func (f datasetFixtureRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func exerciseStorageDatasetPreview(t *testing.T, ctx context.Context, db *gorm.DB,
	call func(string, string, string, any, int) []byte, server *httptest.Server, fault *datasetRedisFault, tables datasetFixtureTables, role, pluginID, path, tenant, sts, foreignTenant, root string, revision int64,
) string {
	t.Helper()
	sourceContents := func() []string {
		t.Helper()
		var contents []string
		for _, table := range []string{tables.reports, tables.assertions, tables.refs} {
			var rows string
			// The fixture owns these table names. Compare every row, including
			// the records for other tenants and the protection references.
			require.NoError(t, db.Raw("SELECT COALESCE(jsonb_agg(to_jsonb(r) ORDER BY to_jsonb(r)), '[]'::jsonb)::text FROM "+table+" r").Row().Scan(&rows))
			contents = append(contents, rows)
		}
		return contents
	}
	initialSource := sourceContents()

	const contract = "powerx.storage-dataset-host/v1"
	invocation := func(capability, token string, body any, status int, result any) {
		response := call("POST", "/tenant/invocations", token, map[string]any{"capability_id": capability,
			"preferred_protocol": "core_internal", "payload": map[string]any{"body": body}}, status)
		if result != nil {
			var outer struct {
				Data struct {
					Result json.RawMessage `json:"result"`
				} `json:"data"`
			}
			require.NoError(t, json.Unmarshal(response, &outer))
			require.NoError(t, json.Unmarshal(outer.Data.Result, result), "%s", response)
		}
	}
	q := func(op string, fields map[string]any, capability string, status int, result any) {
		fields["contract_version"], fields["operation"], fields["tenant_key"] = taskqueue.RuntimeContract, op, tenant
		invocation(capability, sts, fields, status, result)
	}
	receive := func() taskqueue.RuntimeMessage {
		var result struct{ Messages []taskqueue.RuntimeMessage }
		q("dequeue", map[string]any{"subscriber_id": taskqueue.DatasetPreviewSubscriber, "worker_id": "fixture-worker",
			"max_items": 1, "wait_timeout_ms": 500}, taskqueue.ConsumeCapability, 200, &result)
		require.Len(t, result.Messages, 1)
		require.NotNil(t, result.Messages[0].Lease)
		require.NotEmpty(t, result.Messages[0].IdempotencyKey)
		return result.Messages[0]
	}
	request := uuid.NewString()
	notice, err := json.Marshal(map[string]string{"contract_version": contract, "request_uuid": request})
	require.NoError(t, err)
	// Enqueue with the real topic, payload and intended subscription, then
	// obtain a genuine ordinary lease. It cannot authorize the later Root task.
	q("enqueue", map[string]any{"message": taskqueue.RuntimeMessage{ID: uuid.NewString(), TenantKey: tenant,
		Topic: taskqueue.DatasetPreviewTopic, SubscriberID: taskqueue.DatasetPreviewSubscriber,
		PayloadBase64:  base64.StdEncoding.EncodeToString(notice),
		IdempotencyKey: uuid.NewString(), Metadata: map[string]string{"plugin_id": pluginID}}},
		taskqueue.EnqueueCapability, 200, nil)
	forged := receive()

	sseCtx, stop := context.WithTimeout(ctx, 15*time.Second)
	defer stop()
	listen, err := http.NewRequestWithContext(sseCtx, "GET", server.URL+"/custom/v2/admin/ops/storage/runtime/events", nil)
	require.NoError(t, err)
	listen.Header.Set("Authorization", "Bearer "+root)
	response, err := server.Client().Do(listen)
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, 200, response.StatusCode)
	events := bufio.NewReader(response.Body)
	read := func() string {
		t.Helper()
		var frame strings.Builder
		for {
			line, err := events.ReadString('\n')
			require.NoError(t, err)
			if line == "\n" {
				return frame.String()
			}
			frame.WriteString(line)
		}
	}
	require.Contains(t, read(), "event: snapshot")
	input := gov.DatasetPreviewInput{RequestUUID: request, TenantUUID: tenant, AcceptanceRevision: revision, RetentionDays: 30}
	// Fail the first publish after the receipt and task commit. A later
	// blocking receive must recover the same authoritative task.
	fault.offline.Store(true)
	call("POST", path+"/previews", root, input, 202)
	fault.offline.Store(false)
	require.Positive(t, fault.failed.Load())
	notify := func() {
		t.Helper()
		frame := read()
		require.Contains(t, frame, "event: change")
		require.Contains(t, frame, `"reason":"dataset_preview_changed"`)
	}
	notify()
	call("POST", path+"/previews", root, input, 200)
	workerBody := func(op string, msg taskqueue.RuntimeMessage, additional map[string]any) map[string]any {
		fields := map[string]any{"request_uuid": request, "receipt_uuid": msg.Lease.ReceiptUUID, "fence": msg.Lease.Fence}
		for key, value := range additional {
			fields[key] = value
		}
		return map[string]any{"contract_version": contract, "operation": op, "input": fields}
	}
	worker := func(op, token string, msg taskqueue.RuntimeMessage, additional map[string]any, want int, result any) {
		invocation(gov.DatasetPreviewCapability, token, workerBody(op, msg, additional), want, result)
	}
	worker("claim_preview", sts, forged, nil, 409, nil)
	worker("claim_preview", foreignTenant, forged, nil, 403, nil)
	q("ack", map[string]any{"subscriber_id": forged.SubscriberID, "message_id": forged.ID, "receipt_uuid": forged.Lease.ReceiptUUID,
		"outcome": "failed"}, taskqueue.ConsumeCapability, 200, nil)
	first := receive()
	worker("claim_preview", foreignTenant, first, nil, 403, nil)
	q("ack", map[string]any{"subscriber_id": first.SubscriberID, "message_id": first.ID, "receipt_uuid": first.Lease.ReceiptUUID,
		"outcome": "completed"}, taskqueue.ConsumeCapability, 409, nil)
	var grant struct {
		TenantUUID    string    `json:"tenant_uuid"`
		CutoffAt      time.Time `json:"cutoff_at"`
		MaxExamined   int       `json:"max_examined"`
		SchemaVersion int       `json:"schema_contract_version"`
	}
	worker("claim_preview", sts, first, nil, 200, &grant)
	notify()
	require.Equal(t, tenant, grant.TenantUUID)
	require.Equal(t, 1, grant.SchemaVersion)
	require.Equal(t, 2048, grant.MaxExamined)
	at := time.Now().UTC()
	q("nack", map[string]any{"subscriber_id": first.SubscriberID, "message_id": first.ID, "receipt_uuid": first.Lease.ReceiptUUID,
		"retry_at": at, "reason": "fixture_lease_retry"}, taskqueue.ConsumeCapability, 200, nil)
	next := receive()
	require.Greater(t, next.Lease.Fence, first.Lease.Fence)
	worker("claim_preview", sts, next, nil, 200, &grant)
	notify()

	result := gov.DatasetPreviewCounts{Coverage: "complete", ProtectedByReason: []gov.DatasetProtectionCount{}}
	sql, err := db.DB()
	require.NoError(t, err)
	tx, err := sql.BeginTx(ctx, &sqlpkg.TxOptions{ReadOnly: true, Isolation: sqlpkg.LevelRepeatableRead})
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })
	// The synthetic resolver only knows its own trusted table and role. Its
	// bounded query normalizes statement output to the declared protection reasons.
	_, err = tx.ExecContext(ctx, "SET LOCAL ROLE "+role)
	require.NoError(t, err)
	var mode string
	require.NoError(t, tx.QueryRowContext(ctx, "SHOW transaction_read_only").Scan(&mode))
	require.Equal(t, "on", mode)
	_, err = tx.ExecContext(ctx, "SET LOCAL statement_timeout = '1500ms'")
	require.NoError(t, err)
	query := fmt.Sprintf(`
SELECT CASE
  WHEN root.format_version <> 1 THEN 'unknown_eligibility'
  WHEN EXISTS (SELECT 1 FROM %s ref WHERE ref.tenant_uuid = root.tenant_uuid AND ref.report_id = root.id) THEN 'referenced'
  WHEN (SELECT count(*) FROM %s child WHERE child.tenant_uuid = root.tenant_uuid AND child.report_id = root.id) <> root.expected_assertions THEN 'incomplete_record'
  WHEN NOT EXISTS (SELECT 1 FROM %[3]s newer WHERE newer.tenant_uuid = root.tenant_uuid AND newer.case_key = root.case_key
                   AND (newer.finished_at, newer.id) > (root.finished_at, root.id)) THEN 'latest_effective'
  ELSE '' END
FROM %[3]s root
WHERE root.tenant_uuid = $1 AND root.finished_at < $2
ORDER BY root.finished_at, root.id LIMIT $3`, tables.refs, tables.assertions, tables.reports)
	rows, err := tx.QueryContext(ctx, query, grant.TenantUUID, grant.CutoffAt, grant.MaxExamined+1)
	require.NoError(t, err)
	for rows.Next() {
		var reason string
		require.NoError(t, rows.Scan(&reason))
		result.ExaminedCount++
		if reason == "" {
			result.CandidateCount++
		} else {
			result.ProtectedByReason = append(result.ProtectedByReason, gov.DatasetProtectionCount{Reason: reason, Count: 1})
		}
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())
	require.NoError(t, tx.Commit())
	require.Equal(t, 6, result.ExaminedCount)
	require.Equal(t, 2, result.CandidateCount)
	report := map[string]any{"acceptance_revision": revision, "outcome": "completed", "result": result}
	worker("report_preview", sts, first, report, 409, nil)
	tooMany := result
	tooMany.ExaminedCount = grant.MaxExamined + 1
	worker("report_preview", sts, next, map[string]any{"acceptance_revision": revision, "outcome": "completed", "result": tooMany}, 400, nil)
	// Drop the successful HTTP response after Core commits the original
	// report. Root can observe it via SSE before the worker retries.
	receiptPath := path + "/previews/" + request
	reportBody, err := json.Marshal(map[string]any{"capability_id": gov.DatasetPreviewCapability,
		"preferred_protocol": "core_internal", "payload": map[string]any{"body": workerBody("report_preview", next, report)}})
	require.NoError(t, err)
	dropped, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/custom/v2/tenant/invocations", bytes.NewReader(reportBody))
	require.NoError(t, err)
	dropped.Header.Set("Authorization", "Bearer "+sts)
	dropped.Header.Set("Content-Type", "application/json")
	dropResponse := errors.New("fixture lost preview report response")
	var upstreamStatus int
	dropClient := &http.Client{Transport: datasetFixtureRoundTripper(func(req *http.Request) (*http.Response, error) {
		resp, err := server.Client().Transport.RoundTrip(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		upstreamStatus = resp.StatusCode
		if _, err := io.Copy(io.Discard, resp.Body); err != nil {
			return nil, err
		}
		return nil, dropResponse
	})}
	_, err = dropClient.Do(dropped)
	require.ErrorIs(t, err, dropResponse)
	require.Equal(t, http.StatusOK, upstreamStatus)
	notify()
	originalReceipt := call("GET", receiptPath, root, nil, 200)
	require.Contains(t, string(originalReceipt), `"state":"completed"`)

	worker("report_preview", sts, next, report, 200, nil)
	require.JSONEq(t, string(originalReceipt), string(call("GET", receiptPath, root, nil, 200)))
	different := result
	different.CandidateCount = 0
	worker("report_preview", sts, next, map[string]any{"acceptance_revision": revision, "outcome": "completed", "result": different}, 409, nil)
	ack := map[string]any{"subscriber_id": next.SubscriberID, "message_id": next.ID, "receipt_uuid": next.Lease.ReceiptUUID, "outcome": "completed"}
	q("ack", ack, taskqueue.ConsumeCapability, 200, nil)
	q("ack", ack, taskqueue.ConsumeCapability, 200, nil)
	worker("report_preview", sts, next, report, 200, nil)
	receipt := call("GET", receiptPath, root, nil, 200)
	require.Equal(t, initialSource, sourceContents(), "preview must preserve every source table row")
	for _, expected := range []string{`"state":"completed"`, `"candidate_count":2`, "referenced", "latest_effective", "incomplete_record", "unknown_eligibility"} {
		require.Contains(t, string(receipt), expected)
	}
	return receiptPath
}

func exerciseDatasetDeletion(t *testing.T, ctx context.Context, db *gorm.DB,
	call func(string, string, string, any, int) []byte, tables datasetFixtureTables, path, tenant, sts, foreign, root string, acceptance int64, previewPath string,
) {
	t.Helper()
	const contract = "powerx.storage-dataset-execution/v1"
	body := func(capability, token string, payload any, status int, result any) {
		data := call("POST", "/tenant/invocations", token, map[string]any{"capability_id": capability,
			"preferred_protocol": "core_internal", "payload": map[string]any{"body": payload}}, status)
		if result == nil {
			return
		}
		var response struct {
			Data struct{ Result json.RawMessage } `json:"data"`
		}
		require.NoError(t, json.Unmarshal(data, &response))
		require.NoError(t, json.Unmarshal(response.Data.Result, result))
	}
	worker := func(op, token string, payload any, status int, result any) {
		body(gov.DatasetExecutionCapability, token, map[string]any{"contract_version": contract, "operation": op, "input": payload}, status, result)
	}
	policy := path + "/policies/" + tenant
	var input = map[string]any{"expected_revision": 0, "acceptance_revision": acceptance, "enabled": true, "action": "delete", "retention_days": 30}
	call("PUT", policy, sts, input, 403)
	require.Contains(t, string(call("GET", policy, root, nil, 200)), `"state":"paused"`)
	require.Contains(t, string(call("PUT", policy, root, input, 200)), `"state":"enabled"`)
	request := uuid.NewString()
	batchPath := path + "/runs/" + request
	previewID := previewPath[strings.LastIndexByte(previewPath, '/')+1:]
	call("POST", path+"/runs", root, gov.DatasetRunInput{RequestUUID: request, TenantUUID: tenant, PreviewUUID: previewID, PolicyRevision: 1}, 202)
	call("POST", path+"/runs", root, gov.DatasetRunInput{RequestUUID: request, TenantUUID: tenant, PreviewUUID: previewID, PolicyRevision: 1}, 200)
	call("POST", path+"/runs", root, gov.DatasetRunInput{RequestUUID: uuid.NewString(), TenantUUID: tenant, PreviewUUID: previewID, PolicyRevision: 1}, 409)

	var deliveries struct{ Messages []taskqueue.RuntimeMessage }
	body(taskqueue.ConsumeCapability, sts, map[string]any{
		"contract_version": taskqueue.RuntimeContract, "operation": "dequeue", "tenant_key": tenant,
		"subscriber_id": taskqueue.DatasetExecutionSubscriber, "worker_id": "fixture-delete", "max_items": 1, "wait_timeout_ms": 500,
	}, 200, &deliveries)
	require.Len(t, deliveries.Messages, 1)
	lease := deliveries.Messages[0].Lease
	require.NotNil(t, lease)
	claim := map[string]any{"request_uuid": request, "receipt_uuid": lease.ReceiptUUID, "fence": lease.Fence}
	var authorization struct {
		State string
		Grant struct {
			ID       string    `json:"grant_uuid"`
			Issued   time.Time `json:"issued_at"`
			Deadline time.Time `json:"deadline_at"`
			Cutoff   time.Time `json:"cutoff_at"`
		}
	}
	worker("claim_execution", foreign, claim, 403, nil)
	worker("claim_execution", sts, claim, 200, &authorization)
	require.Equal(t, "authorized", authorization.State)
	require.NotEmpty(t, authorization.Grant.ID)

	// The synthetic source uses the same transaction for its business guard,
	// deletion and durable receipt. A newly created reference must win.
	ledger := tables.reports + "_batch_receipts"
	require.NoError(t, db.Exec("CREATE TABLE "+ledger+" (batch_uuid uuid PRIMARY KEY, grant_uuid uuid NOT NULL, receipt_uuid uuid NOT NULL, affected integer NOT NULL, sealed_at timestamptz NOT NULL)").Error)
	reference := db.WithContext(ctx).Begin()
	require.NoError(t, reference.Error)
	defer reference.Rollback()
	require.NoError(t, reference.Exec("INSERT INTO "+tables.refs+" VALUES (?, 1)", tenant).Error)
	var matched int64
	stored := uuid.NewString()
	sealedAt := time.Time{}
	writeSource := func(tx *gorm.DB) error {
		if err := tx.Exec("SET LOCAL lock_timeout='200ms'").Error; err != nil {
			return err
		}
		if err := tx.Exec("LOCK TABLE " + tables.refs + " IN SHARE MODE").Error; err != nil {
			return err
		}
		// The test adapter limits this batch to two known eligible roots and
		// repeats the tenant, deadline and reference checks before deletion.
		result := tx.Exec("WITH removed AS (DELETE FROM "+tables.reports+" r WHERE r.tenant_uuid = ? AND r.id IN (1, 7) AND r.finished_at < ? AND clock_timestamp() < ? AND NOT EXISTS (SELECT 1 FROM "+tables.refs+" ref WHERE ref.tenant_uuid=r.tenant_uuid AND ref.report_id=r.id) RETURNING tenant_uuid, id) DELETE FROM "+tables.assertions+" a USING removed WHERE a.tenant_uuid = removed.tenant_uuid AND a.report_id = removed.id",
			tenant, authorization.Grant.Cutoff, authorization.Grant.Deadline)
		if result.Error != nil {
			return result.Error
		}
		matched = result.RowsAffected
		sealedAt = time.Now().UTC().Truncate(time.Microsecond)
		return tx.Exec("INSERT INTO "+ledger+" VALUES (?, ?, ?, ?, ?)", request, authorization.Grant.ID, stored, matched, sealedAt).Error
	}
	require.ErrorContains(t, db.WithContext(ctx).Transaction(writeSource), "lock timeout")
	require.NoError(t, reference.Commit().Error)
	aborted := errors.New("fixture failure before commit")
	require.ErrorIs(t, db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := writeSource(tx); err != nil {
			return err
		}
		return aborted
	}), aborted)
	var count int64
	require.NoError(t, db.Table(ledger).Where("batch_uuid = ?", request).Count(&count).Error)
	require.Zero(t, count)
	require.NoError(t, db.Table(tables.reports).Where("tenant_uuid = ? AND id = 7", tenant).Count(&count).Error)
	require.EqualValues(t, 1, count)
	originalGrant := authorization.Grant.ID
	worker("claim_execution", sts, claim, 200, &authorization)
	require.Equal(t, originalGrant, authorization.Grant.ID)
	require.NoError(t, db.WithContext(ctx).Transaction(writeSource))
	require.EqualValues(t, 1, matched)
	require.Contains(t, string(call("PUT", policy, root, map[string]any{"expected_revision": 1, "enabled": false}, 200)), `"state":"draining"`)
	call("POST", batchPath+"/cancellations", root, map[string]any{}, 409)
	var replay struct{ State string }
	worker("claim_execution", sts, claim, 200, &replay)
	require.Equal(t, "reconcile_only", replay.State)

	// Let the original TaskBus lease expire, then report the immutable source
	// result. The original execution authorization is never reissued.
	timer := time.NewTimer(time.Until(lease.ExpiresAt) + 100*time.Millisecond)
	select {
	case <-ctx.Done():
		timer.Stop()
		t.Fatal(ctx.Err())
	case <-timer.C:
	}
	require.Contains(t, string(call("GET", batchPath, root, nil, 200)), `"state":"needs_reconciliation"`)
	var storedProof struct {
		ReceiptUUID string
		GrantUUID   string
		Affected    int
		SealedAt    time.Time
	}
	require.NoError(t, db.Table(ledger).Where("batch_uuid = ?", request).Take(&storedProof).Error)
	raw, err := json.Marshal(storedProof)
	require.NoError(t, err)
	sum := sha256.Sum256(raw)
	report := map[string]any{"request_uuid": request, "grant_uuid": storedProof.GrantUUID, "source_receipt_uuid": storedProof.ReceiptUUID,
		"source_receipt_digest": hex.EncodeToString(sum[:]), "affected_count": storedProof.Affected, "outcome": "committed", "source_sealed_at": storedProof.SealedAt}
	worker("report_execution", foreign, report, 403, nil)
	worker("report_execution", sts, report, 200, nil)
	worker("report_execution", sts, report, 200, nil)
	report["source_receipt_uuid"] = uuid.NewString()
	worker("report_execution", sts, report, 409, nil)
	require.Contains(t, string(call("GET", policy, root, nil, 200)), `"state":"paused"`)
	require.Contains(t, string(call("GET", batchPath, root, nil, 200)), `"affected_count":1`)
	require.NoError(t, db.Table(tables.reports).Where("tenant_uuid = ?", tenant).Count(&count).Error)
	require.EqualValues(t, 6, count)
	require.NoError(t, db.Table(tables.reports).Where("tenant_uuid = ? AND id = 1", tenant).Count(&count).Error)
	require.EqualValues(t, 1, count, "late reference must protect the root")
	require.NoError(t, db.Table(tables.reports).Where("tenant_uuid = ? AND id = 7", tenant).Count(&count).Error)
	require.Zero(t, count)
}

func exerciseDatasetReconciliation(t *testing.T, ctx context.Context, db *gorm.DB,
	call func(string, string, string, any, int) []byte, tables datasetFixtureTables, path, tenant, sts, foreign, root string,
	acceptance int64, pause func(),
) {
	t.Helper()
	invoke := func(capability, token string, payload any, status int, result any) {
		response := call("POST", "/tenant/invocations", token, map[string]any{"capability_id": capability,
			"preferred_protocol": "core_internal", "payload": map[string]any{"body": payload}}, status)
		if result != nil {
			var envelope struct {
				Data struct{ Result json.RawMessage } `json:"data"`
			}
			require.NoError(t, json.Unmarshal(response, &envelope))
			require.NoError(t, json.Unmarshal(envelope.Data.Result, result))
		}
	}
	receive := func(subscriber string) *taskqueue.RuntimeLease {
		var deliveries struct{ Messages []taskqueue.RuntimeMessage }
		for attempt := 0; attempt < 3 && len(deliveries.Messages) == 0; attempt++ {
			invoke(taskqueue.ConsumeCapability, sts, map[string]any{
				"contract_version": taskqueue.RuntimeContract, "operation": "dequeue", "tenant_key": tenant,
				"subscriber_id": subscriber, "worker_id": "fixture-reconcile", "max_items": 1, "wait_timeout_ms": 500,
			}, 200, &deliveries)
		}
		require.Len(t, deliveries.Messages, 1)
		require.NotNil(t, deliveries.Messages[0].Lease)
		return deliveries.Messages[0].Lease
	}
	execution := func(op, token string, input any, status int, output any) {
		invoke(gov.DatasetExecutionCapability, token, map[string]any{
			"contract_version": "powerx.storage-dataset-execution/v1", "operation": op, "input": input,
		}, status, output)
	}

	previewID := uuid.NewString()
	call("POST", path+"/previews", root, gov.DatasetPreviewInput{
		RequestUUID: previewID, TenantUUID: tenant, RetentionDays: 30, AcceptanceRevision: acceptance,
	}, 202)
	preview := receive(taskqueue.DatasetPreviewSubscriber)
	previewInput := map[string]any{"request_uuid": previewID, "receipt_uuid": preview.ReceiptUUID, "fence": preview.Fence}
	previewOperation := map[string]any{"contract_version": "powerx.storage-dataset-host/v1", "operation": "claim_preview", "input": previewInput}
	invoke(gov.DatasetPreviewCapability, sts, previewOperation, 200, nil)
	previewOperation["operation"] = "report_preview"
	previewInput["acceptance_revision"], previewInput["outcome"] = acceptance, "completed"
	previewInput["result"] = gov.DatasetPreviewCounts{Coverage: "bounded", ExaminedCount: 1, CandidateCount: 1, ProtectedByReason: []gov.DatasetProtectionCount{}}
	invoke(gov.DatasetPreviewCapability, sts, previewOperation, 200, nil)

	policyPath := path + "/policies/" + tenant
	var policy struct{ Data gov.DatasetExecutionPolicy }
	require.NoError(t, json.Unmarshal(call("GET", policyPath, root, nil, 200), &policy))
	config := map[string]any{"expected_revision": policy.Data.Revision, "acceptance_revision": acceptance,
		"enabled": true, "action": "delete", "retention_days": 30}
	require.NoError(t, json.Unmarshal(call("PUT", policyPath, root, config, 200), &policy))
	input := gov.DatasetRunInput{RequestUUID: uuid.NewString(), TenantUUID: tenant, PreviewUUID: previewID, PolicyRevision: policy.Data.Revision}
	batchPath := path + "/runs/" + input.RequestUUID
	call("POST", path+"/runs", root, input, 202)
	ref := map[string]any{"request_uuid": input.RequestUUID}
	execution("reconcile_execution", sts, ref, 409, nil)
	lease := receive(taskqueue.DatasetExecutionSubscriber)
	claim := map[string]any{"request_uuid": input.RequestUUID, "receipt_uuid": lease.ReceiptUUID, "fence": lease.Fence}
	type claimResult struct {
		State        string
		Grant        json.RawMessage
		ExpiredGrant json.RawMessage `json:"expired_grant"`
	}
	var original, replay claimResult
	execution("claim_execution", sts, claim, 200, &original)
	require.Equal(t, "authorized", original.State)
	var grant struct {
		ID       string    `json:"grant_uuid"`
		Deadline time.Time `json:"deadline_at"`
	}
	require.NoError(t, json.Unmarshal(original.Grant, &grant))
	require.NotEmpty(t, grant.ID)
	execution("claim_execution", sts, claim, 200, &replay)
	require.JSONEq(t, string(original.Grant), string(replay.Grant))
	execution("reconcile_execution", sts, ref, 409, nil)

	ledger := tables.reports + "_batch_receipts"
	var count, originalRows int64
	require.NoError(t, db.Table(ledger).Where("batch_uuid = ?", input.RequestUUID).Count(&count).Error)
	require.Zero(t, count, "the worker never reserved the lost grant")
	require.NoError(t, db.Table(tables.reports).Where("tenant_uuid = ?", tenant).Count(&originalRows).Error)

	pause()
	replay = claimResult{}
	execution("claim_execution", sts, claim, 200, &replay)
	require.Equal(t, "reconcile_only", replay.State)
	require.Empty(t, replay.Grant)
	require.Empty(t, replay.ExpiredGrant)
	blocked := input
	blocked.RequestUUID = uuid.NewString()
	call("POST", path+"/runs", root, blocked, 423)

	timer := time.NewTimer(time.Until(lease.ExpiresAt) + 100*time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	case <-timer.C:
	}
	execution("claim_execution", sts, claim, 409, nil)
	execution("reconcile_execution", foreign, ref, 403, nil)
	execution("reconcile_execution", sts, claim, 400, nil)
	execution("reconcile_execution", sts, ref, 200, &replay)
	require.Equal(t, "reconcile_only", replay.State)
	require.Empty(t, replay.Grant)
	require.JSONEq(t, string(original.Grant), string(replay.ExpiredGrant))
	require.Contains(t, string(call("GET", batchPath, root, nil, 200)), `"state":"needs_reconciliation"`)
	require.NoError(t, json.Unmarshal(call("GET", policyPath, root, nil, 200), &policy))
	require.Equal(t, input.RequestUUID, policy.Data.ActiveRequest)
	require.NoError(t, db.Table(ledger).Where("batch_uuid = ?", input.RequestUUID).Count(&count).Error)
	require.Zero(t, count, "a Core lookup cannot settle a source outcome")

	// Seal an empty local tombstone after the original deadline. This fixture
	// never invokes the business adapter for expired authorization.
	receipt := uuid.NewString()
	var sealedAt time.Time
	require.NoError(t, db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return tx.Raw("INSERT INTO "+ledger+" SELECT ?, ?, ?, 0, clock_timestamp() WHERE clock_timestamp() >= ? RETURNING sealed_at",
			input.RequestUUID, grant.ID, receipt, grant.Deadline).Row().Scan(&sealedAt)
	}))
	require.False(t, sealedAt.Before(grant.Deadline))
	sum := sha256.Sum256([]byte(input.RequestUUID + ":" + grant.ID + ":" + receipt + ":" + sealedAt.UTC().Format(time.RFC3339Nano)))
	report := map[string]any{"request_uuid": input.RequestUUID, "grant_uuid": grant.ID, "source_receipt_uuid": receipt,
		"source_receipt_digest": hex.EncodeToString(sum[:]), "outcome": "committed", "affected_count": 0, "source_sealed_at": grant.Deadline}
	execution("report_execution", sts, report, 409, nil)
	report["outcome"] = "aborted"
	report["source_sealed_at"] = time.Now().Add(time.Hour)
	execution("report_execution", sts, report, 409, nil)
	delete(report, "source_sealed_at")
	report["source_commit_at"] = sealedAt
	execution("report_execution", sts, report, 400, nil)
	delete(report, "source_commit_at")
	report["source_sealed_at"] = sealedAt
	execution("report_execution", sts, report, 200, nil)
	execution("report_execution", sts, report, 200, nil)
	require.Contains(t, string(call("GET", batchPath, root, nil, 200)), `"state":"aborted"`)
	policy.Data = gov.DatasetExecutionPolicy{}
	require.NoError(t, json.Unmarshal(call("GET", policyPath, root, nil, 200), &policy))
	require.Empty(t, policy.Data.ActiveRequest)
	require.NoError(t, db.Table(tables.reports).Where("tenant_uuid = ?", tenant).Count(&count).Error)
	require.Equal(t, originalRows, count)
}

func fixtureDatasetArchiveTarget(t *testing.T, ctx context.Context, dir, configFile string, service *gov.Service,
	control *gov.ControlService, queue *taskqueue.StreamDriver, scope gov.ControlScope,
) string {
	t.Helper()
	var targetConfig struct {
		Endpoint string `json:"endpoint"`
		Bucket   string `json:"bucket"`
		Region   string `json:"region"`
		Access   string `json:"access_key"`
		Secret   string `json:"secret_key"`
	}
	configuration, err := os.ReadFile(configFile)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(configuration, &targetConfig))
	require.True(t, strings.HasPrefix(targetConfig.Endpoint, "http://127.0.0.1:"))
	require.Equal(t, "powerx-dataset-validation", targetConfig.Bucket)
	root := reqctx.WithUserID(reqctx.WithIsRoot(reqctx.WithClaims(ctx, &reqctx.CoreXClaims{UserID: 1, IsRoot: true, Scope: "access"}), true), 1)
	archives := archiveOps.NewService(ctx, archiveOps.Config{Directory: filepath.Join(dir, "archive"), Instance: "owned-datasets",
		S3Credentials: map[string]archiveOps.S3Credential{"owned": {AccessKey: targetConfig.Access, SecretKey: targetConfig.Secret}}}, archiveOps.Options{})
	t.Cleanup(archives.Close)
	target, err := archives.SaveTarget(root, archiveOps.Target{Driver: "s3", Name: "owned dataset archive", Endpoint: targetConfig.Endpoint,
		Bucket: targetConfig.Bucket, Region: targetConfig.Region, Prefix: "test/" + uuid.NewString(), CredentialRef: "owned", ForcePathStyle: true})
	require.NoError(t, err)
	require.NoError(t, service.BindArchiveControl(archives))
	current, err := archives.TargetConfiguration(root)
	require.NoError(t, err)
	_, err = archives.AdoptTargetConfiguration(root, archiveOps.TargetAdoption{RequestID: uuid.NewString(), LegacyDigest: current.LegacyDigest})
	require.NoError(t, err)
	pending, err := queue.Dequeue(ctx, "platform:storage-control", "core.storage.control."+scope.ID(), "owned-dataset-config", "test", 1, time.Second)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	require.NoError(t, control.ApplyDelivery(ctx, pending[0]))
	require.NoError(t, archives.TestTarget(root, target.ID))
	return target.ID
}

func exerciseDatasetArchive(t *testing.T, ctx context.Context, db, sourceDB *gorm.DB,
	call func(string, string, string, any, int) []byte, tables datasetFixtureTables, path, tenant, sts, root string, acceptance int64, target string,
) {
	t.Helper()
	invoke := func(capability, version, op string, input any, status int, result any) {
		fields, isQueue := input.(map[string]any)
		if isQueue && capability == taskqueue.ConsumeCapability {
			fields["contract_version"], fields["operation"], fields["tenant_key"] = version, op, tenant
		} else {
			fields = map[string]any{"contract_version": version, "operation": op, "input": input}
		}
		raw := call("POST", "/tenant/invocations", sts, map[string]any{"capability_id": capability,
			"preferred_protocol": "core_internal", "payload": map[string]any{"body": fields}}, status)
		if result != nil {
			var response struct {
				Data struct{ Result json.RawMessage } `json:"data"`
			}
			require.NoError(t, json.Unmarshal(raw, &response))
			require.NoError(t, json.Unmarshal(response.Data.Result, result))
		}
	}
	receive := func(subscriber string) taskqueue.RuntimeMessage {
		var list struct{ Messages []taskqueue.RuntimeMessage }
		for i := 0; i < 3 && len(list.Messages) == 0; i++ {
			invoke(taskqueue.ConsumeCapability, taskqueue.RuntimeContract, "dequeue", map[string]any{
				"subscriber_id": subscriber, "worker_id": "archive-fixture", "max_items": 1, "wait_timeout_ms": 500,
			}, 200, &list)
		}
		require.Len(t, list.Messages, 1)
		require.NotNil(t, list.Messages[0].Lease)
		return list.Messages[0]
	}
	ref := func(id string, msg taskqueue.RuntimeMessage) map[string]any {
		return map[string]any{"request_uuid": id, "receipt_uuid": msg.Lease.ReceiptUUID, "fence": msg.Lease.Fence}
	}
	require.NoError(t, sourceDB.Exec("INSERT INTO "+tables.reports+" VALUES (9, ?, NOW() - INTERVAL '44 days', 1, 1, 1)", tenant).Error)
	require.NoError(t, sourceDB.Exec("INSERT INTO "+tables.assertions+" VALUES (?, 9, 1)", tenant).Error)
	var sourceCount int64
	require.NoError(t, sourceDB.Table(tables.reports).Where("tenant_uuid = ?", tenant).Count(&sourceCount).Error)

	previewID := uuid.NewString()
	const previewContract = "powerx.storage-dataset-host/v1"
	call("POST", path+"/previews", root, gov.DatasetPreviewInput{RequestUUID: previewID, TenantUUID: tenant, RetentionDays: 30, AcceptanceRevision: acceptance}, 202)
	preview := receive(taskqueue.DatasetPreviewSubscriber)
	invoke(gov.DatasetPreviewCapability, previewContract, "claim_preview", ref(previewID, preview), 200, nil)
	review := ref(previewID, preview)
	review["acceptance_revision"], review["outcome"] = acceptance, "completed"
	review["result"] = gov.DatasetPreviewCounts{Coverage: "bounded", ExaminedCount: 1, CandidateCount: 1, ProtectedByReason: []gov.DatasetProtectionCount{}}
	invoke(gov.DatasetPreviewCapability, previewContract, "report_preview", review, 200, nil)

	policy := path + "/policies/" + tenant
	config := map[string]any{"expected_revision": 2, "acceptance_revision": acceptance, "enabled": true, "action": "archive",
		"archive_target_uuid": target, "retention_days": 30}
	require.Contains(t, string(call("PUT", policy, root, config, 200)), `"state":"enabled"`)
	request := uuid.NewString()
	call("POST", path+"/runs", root, gov.DatasetRunInput{RequestUUID: request, TenantUUID: tenant, PreviewUUID: previewID, PolicyRevision: 3}, 202)
	work := receive(taskqueue.DatasetExecutionSubscriber)
	const executionContract = "powerx.storage-dataset-execution/v1"
	var grant struct {
		Grant struct {
			ID          string    `json:"grant_uuid"`
			CutoffAt    time.Time `json:"cutoff_at"`
			DeadlineAt  time.Time `json:"deadline_at"`
			MaxAffected int       `json:"max_affected"`
		}
	}
	invoke(gov.DatasetExecutionCapability, executionContract, "claim_execution", ref(request, work), 200, &grant)
	var body []byte
	require.NoError(t, sourceDB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var raw string
		err := tx.Raw("SELECT row_to_json(r)::text FROM "+tables.reports+" r WHERE r.tenant_uuid = ? AND r.id = 9 AND r.finished_at < ? AND clock_timestamp() < ?",
			tenant, grant.Grant.CutoffAt, grant.Grant.DeadlineAt).Scan(&raw).Error
		body = []byte(raw + "\n")
		return err
	}, &sqlpkg.TxOptions{ReadOnly: true}))
	require.Contains(t, string(body), `"id":9`)
	sum := sha256.Sum256(body)
	content := map[string]any{"report": map[string]any{"request_uuid": request, "grant_uuid": grant.Grant.ID,
		"source_receipt_uuid": uuid.NewString(), "source_receipt_digest": hex.EncodeToString(sum[:]),
		"outcome": "committed", "affected_count": 1, "source_sealed_at": time.Now().UTC()}, "archive_base64": base64.StdEncoding.EncodeToString(body)}
	invoke(gov.DatasetExecutionCapability, executionContract, "archive_execution", content, 200, nil)
	invoke(gov.DatasetExecutionCapability, executionContract, "archive_execution", content, 200, nil)
	batchPath := path + "/runs/" + request
	require.Contains(t, string(call("GET", batchPath, root, nil, 200)), `"archive_available":true`)
	require.Equal(t, body, call("GET", batchPath+"/archive", root, nil, 200))
	call("GET", batchPath+"/archive", sts, nil, 403)

	var count int64
	require.NoError(t, sourceDB.Table(tables.reports).Where("tenant_uuid = ?", tenant).Count(&count).Error)
	require.Equal(t, sourceCount, count, "archive must keep source data")
	var traces []cm.InvocationTrace
	require.NoError(t, db.Where("capability_id = ?", gov.DatasetExecutionCapability).Find(&traces).Error)
	require.NotEmpty(t, traces)
	for _, trace := range traces {
		require.NotContains(t, string(trace.RequestPayload), "archive_base64")
		require.NotContains(t, string(trace.ResponsePayload), grant.Grant.ID)
	}
}
