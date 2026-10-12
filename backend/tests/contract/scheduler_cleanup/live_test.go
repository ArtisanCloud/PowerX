package schedulercleanup_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	commonv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/common/v1"
	stsv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/auth/sts/v1"
	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/apikeypermissions"
	svc "github.com/ArtisanCloud/PowerX/internal/service/runtime_scheduler"
	"github.com/ArtisanCloud/PowerX/internal/service/setting"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/database"
	iam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	gw "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/runtime_scheduler"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func sha(text string) string { sum := sha256.Sum256([]byte(text)); return hex.EncodeToString(sum[:]) }

type record struct {
	Name     string          `json:"name"`
	HTTP     int             `json:"http_status"`
	Trace    string          `json:"trace_id"`
	Response json.RawMessage `json:"response"`
}

func liveDB(t *testing.T) (*config.Config, *gorm.DB, string) {
	path := os.Getenv("POWERX_SCHEDULER_CONFIG")
	if path == "" {
		t.Skip("opt-in isolated PostgreSQL/API acceptance")
	}
	cfg, err := config.Load(path)
	require.NoError(t, err)
	db, err := database.Connect(cfg.Database)
	require.NoError(t, err)
	db = db.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)})
	sql, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sql.Close() })
	source, err := os.ReadFile("../../../cmd/database/seed/seed_dev_api_keys.go")
	require.NoError(t, err)
	match := regexp.MustCompile(`Key:\s*"(pxk_[^"]+)"`).FindSubmatch(source)
	require.Len(t, match, 2)
	return cfg, db, string(match[1])
}
func installOwnerGrants(t *testing.T, db *gorm.DB, key gw.IntegrationGatewayAPIKey, owner string, actions ...string) []gw.IntegrationGatewayAPIKeyPermission {
	t.Helper()
	var permissions []iam.Permission
	require.NoError(t, db.Where("module = ? AND status = ? AND allow_api_key = ?", "scheduler", iam.PermissionStatusActive, true).Find(&permissions).Error)
	grants := []gw.IntegrationGatewayAPIKeyPermission{}
	for _, action := range actions {
		scope := "_scope.scheduler.jobs.service_" + action
		var selected *iam.Permission
		for i, p := range permissions {
			parsed, ok := apikeypermissions.ResolvePermission(p)
			if ok && parsed.Scope == scope {
				selected = &permissions[i]
				break
			}
		}
		require.NotNil(t, selected, "capability-seed must materialize service grants")
		var meta map[string]any
		require.NoError(t, json.Unmarshal(selected.Meta, &meta))
		require.Equal(t, true, meta["api_key_explicit"])
		var existing gw.IntegrationGatewayAPIKeyPermission
		require.NoError(t, db.Where("api_key_uuid = ? AND scope = ? AND plugin_id = ?", key.UUID, scope, owner).Limit(1).Find(&existing).Error)
		if existing.UUID == uuid.Nil {
			existing = gw.IntegrationGatewayAPIKeyPermission{APIKeyUUID: key.UUID, Scope: scope, Action: action, ResourceType: "api", ResourcePattern: "scheduler_jobs", PluginID: owner, Effect: "allow"}
			require.NoError(t, db.Create(&existing).Error)
		}
		grants = append(grants, existing)
	}
	return grants
}
func TestLiveSchedulerCleanupHTTPAndSTS(t *testing.T) {
	cfg, db, apiKey := liveDB(t)
	base, grpcAddr := os.Getenv("POWERX_SCHEDULER_HTTP"), os.Getenv("POWERX_SCHEDULER_GRPC")
	if base == "" || grpcAddr == "" {
		t.Skip("HTTP and real STS endpoints required")
	}
	var key gw.IntegrationGatewayAPIKey
	require.NoError(t, db.Where("key_hash = ?", sha(apiKey)).First(&key).Error)
	tenant := key.TenantUUID
	owner := "com.powerx.plugins.scrm"
	installOwnerGrants(t, db, key, owner, "read", "delete")
	apiAuthorization := "ApiKey " + apiKey
	records := []record{}
	call := func(name, method, path, auth string, body any) (int, map[string]json.RawMessage) {
		var data []byte
		var err error
		if body != nil {
			data, err = json.Marshal(body)
			require.NoError(t, err)
		}
		request, err := http.NewRequest(method, base+path, bytes.NewReader(data))
		require.NoError(t, err)
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Trace-ID", uuid.NewString())
		if auth != "" {
			request.Header.Set("Authorization", auth)
		}
		response, err := (&http.Client{Timeout: 15 * time.Second}).Do(request)
		require.NoError(t, err)
		defer response.Body.Close()
		raw, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
		require.NoError(t, err)
		parsed := map[string]json.RawMessage{}
		require.NoError(t, json.Unmarshal(raw, &parsed), "%s: %s", name, raw)
		records = append(records, record{name, response.StatusCode, request.Header.Get("X-Trace-ID"), raw})
		return response.StatusCode, parsed
	}
	if out := os.Getenv("POWERX_SCHEDULER_EVIDENCE"); out != "" {
		defer func() {
			raw, err := json.MarshalIndent(map[string]any{"schema": "powerx.scheduler.cleanup-acceptance/v1", "http_endpoint": base, "records": records}, "", "  ")
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(out, raw, 0600))
		}()
	}
	status, response := call("development_host_grant_status", "POST", "/api/v1/tenant/capabilities:grant-status", apiAuthorization, map[string]any{"capability_ids": []string{svc.CleanupReadCapabilityID, svc.CleanupDeleteCapabilityID}})
	require.Equal(t, 200, status)
	require.NotContains(t, string(response["data"]), "not_granted")
	var rootUser iam.User
	require.NoError(t, db.Where("is_root = ? AND status = ?", true, 1).First(&rootUser).Error)
	claims := reqctx.CoreXClaims{Env: "dev", TenantUUID: tenant, UserID: rootUser.ID, UserUUID: rootUser.UUID.String(), IsRoot: true, Scope: "access", Platforms: []string{"admin"}, RegisteredClaims: jwt.RegisteredClaims{Issuer: cfg.Auth.Issuer, Audience: jwt.ClaimStrings{cfg.Auth.AudienceUser}, Subject: rootUser.UUID.String(), IssuedAt: jwt.NewNumericDate(time.Now()), ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute))}}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(cfg.Auth.JWTSecret))
	require.NoError(t, err)
	admin := "Bearer " + token
	credentials := setting.NewPluginInstanceConfigService(&shared.Deps{DB: db})
	clientOwner := "com.powerx.acceptance.scheduler." + strings.ReplaceAll(uuid.NewString(), "-", "")
	expiry := time.Now().Add(10 * time.Minute).Unix()
	cid, secret, err := credentials.EnsureCredentials(context.Background(), tenant, clientOwner, &setting.ClientCredential{AllowedAudiences: []string{"powerx:api"}, AllowedScopes: []string{"access"}, AllowedCapabilities: []string{svc.CleanupReadCapabilityID, svc.CleanupDeleteCapabilityID}, ExpiresAt: &expiry})
	require.NoError(t, err)
	defer credentials.DeleteCredentials(context.Background(), tenant, clientOwner, false)
	conn, err := grpc.NewClient(grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer conn.Close()
	exchanged, err := stsv1.NewSTSServiceClient(conn).Exchange(context.Background(), &stsv1.ExchangeRequest{Ctx: &commonv1.RequestContext{RequestId: uuid.NewString()}, ClientId: cid, ClientSecret: secret, Audience: "powerx:api", Scope: "access", TtlSeconds: 300})
	require.NoError(t, err)
	require.EqualValues(t, 200, exchanged.GetMeta().GetCode())
	sts := "Bearer " + exchanged.GetData().GetAccessToken()
	records = append(records, record{"real_sts_exchange", 200, exchanged.GetMeta().GetRequestId(), json.RawMessage(`{"token_redacted":true,"audience":"powerx:api"}`)})
	prefix := "core-cleanup-" + uuid.NewString()
	create := func(label, jobOwner string) models.SchedulerJob {
		status, response := call(label, "POST", "/api/v1/admin/scheduler/jobs", admin, map[string]any{"owner_type": "plugin", "owner_id": jobOwner, "name": prefix + "-" + label, "schedule_type": "interval", "schedule_expr": "24h", "payload": map[string]any{"acceptance": true}})
		require.Equal(t, 200, status, "%s", response)
		var wrapped struct {
			Job models.SchedulerJob `json:"job"`
		}
		require.NoError(t, json.Unmarshal(response["data"], &wrapped))
		require.EqualValues(t, 1, wrapped.Job.Revision)
		return wrapped.Job
	}
	job := create("create_api_key_fixture", owner)
	stsJob := create("create_sts_fixture", clientOwner)
	defer func() {
		db.Unscoped().Where("job_uuid IN ?", []uuid.UUID{job.UUID, stsJob.UUID}).Delete(&models.SchedulerJobRun{})
		db.Unscoped().Where("uuid IN ?", []uuid.UUID{job.UUID, stsJob.UUID}).Delete(&models.SchedulerJob{})
	}()
	path := "/api/v1/tenant/scheduler/jobs/" + job.UUID.String()
	deletePath := path + "?expected_revision=1"
	status, _ = call("owner_scoped_preview", "GET", "/api/v1/tenant/scheduler/jobs?owner_id="+owner, apiAuthorization, nil)
	require.Equal(t, 200, status)
	status, _ = call("missing_revision", "DELETE", path, apiAuthorization, nil)
	require.Equal(t, 400, status)
	status, _ = call("unknown_delete_query", "DELETE", deletePath+"&force=true", apiAuthorization, nil)
	require.Equal(t, 400, status)
	status, _ = call("api_key_wrong_owner", "DELETE", "/api/v1/tenant/scheduler/jobs/"+stsJob.UUID.String()+"?expected_revision=1", apiAuthorization, nil)
	require.Equal(t, 403, status)
	status, _ = call("sts_wrong_owner", "DELETE", deletePath, sts, nil)
	require.Equal(t, 403, status)
	status, _ = call("sts_admin_plane_denied", "DELETE", "/api/v1/admin/scheduler/jobs/"+stsJob.UUID.String()+"?expected_revision=1", sts, nil)
	require.Equal(t, 403, status)
	status, _ = call("pause_changes_revision", "POST", "/api/v1/admin/scheduler/jobs/"+job.UUID.String()+"/pause", admin, nil)
	require.Equal(t, 200, status)
	status, _ = call("stale_preview_conflict", "DELETE", deletePath, apiAuthorization, nil)
	require.Equal(t, 409, status)
	status, _ = call("resume_changes_revision", "POST", "/api/v1/admin/scheduler/jobs/"+job.UUID.String()+"/resume", admin, nil)
	require.Equal(t, 200, status)
	status, response = call("manual_trigger_before_delete", "POST", "/api/v1/admin/scheduler/jobs/"+job.UUID.String()+"/trigger", admin, nil)
	require.Equal(t, 200, status)
	var triggered struct {
		Job models.SchedulerJob `json:"job"`
	}
	require.NoError(t, json.Unmarshal(response["data"], &triggered))
	status, response = call("api_key_delete", "DELETE", path+"?expected_revision="+jsonNumber(t, triggered.Job.Revision), apiAuthorization, nil)
	require.Equal(t, 200, status, "%s", response)
	var deletion svc.DeleteJobResult
	require.NoError(t, json.Unmarshal(response["data"], &deletion))
	require.True(t, deletion.Deleted)
	require.False(t, deletion.AlreadyDeleted)
	status, response = call("idempotent_repeated_delete", "DELETE", deletePath, apiAuthorization, nil)
	require.Equal(t, 200, status)
	require.Contains(t, string(response["data"]), `"already_deleted":true`)
	status, _ = call("default_get_excludes_deleted", "GET", path, apiAuthorization, nil)
	require.Equal(t, 404, status)
	status, _ = call("tombstone_authorized_read", "GET", path+"?include_deleted=true", apiAuthorization, nil)
	require.Equal(t, 200, status)
	status, response = call("retained_execution_history", "GET", path+"/runs", apiAuthorization, nil)
	require.Equal(t, 200, status)
	require.Contains(t, string(response["data"]), "trace")
	status, _ = call("deleted_trigger_rejected", "POST", "/api/v1/admin/scheduler/jobs/"+job.UUID.String()+"/trigger", admin, nil)
	require.Equal(t, 404, status)
	status, _ = call("deleted_resume_rejected", "POST", "/api/v1/admin/scheduler/jobs/"+job.UUID.String()+"/resume", admin, nil)
	require.Equal(t, 404, status)
	status, response = call("typed_sts_delete", "POST", "/api/v1/tenant/invocations", sts, map[string]any{"capability_id": svc.CleanupDeleteCapabilityID, "preferred_protocol": "core_internal", "payload": map[string]any{"body": map[string]any{"operation": "delete_job", "job_uuid": stsJob.UUID.String(), "expected_revision": 1}}})
	require.Equal(t, 200, status, "%s", response)
	status, _ = call("free_proxy_rejected", "POST", "/api/v1/tenant/invocations", apiAuthorization, map[string]any{"capability_id": svc.CleanupDeleteCapabilityID, "preferred_protocol": "core_internal", "payload": map[string]any{"method": "DELETE", "endpoint": "/admin/root", "body": map[string]any{"operation": "delete_job", "job_uuid": job.UUID.String(), "expected_revision": 1}}})
	require.Equal(t, 400, status)
	// Read-only temporary key requires both the IAM identity and Gateway grant.
	rawKey := "pxk_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	readKey := gw.IntegrationGatewayAPIKey{TenantUUID: tenant, ProfileID: key.ProfileID, Name: prefix, KeyPrefix: rawKey[:12], KeyHash: sha(rawKey), Status: "active"}
	require.NoError(t, db.Create(&readKey).Error)
	defer db.Unscoped().Delete(&readKey)
	identity := iam.APIKey{TenantUUID: tenant, ProfileID: key.ProfileID, KeyHash: sha(rawKey)}
	require.NoError(t, db.Create(&identity).Error)
	defer db.Unscoped().Delete(&identity)
	grants := installOwnerGrants(t, db, readKey, owner, "read")
	defer db.Unscoped().Where("api_key_uuid = ?", readKey.UUID).Delete(&gw.IntegrationGatewayAPIKeyPermission{})
	status, _ = call("read_only_key_history", "GET", path+"/runs", "ApiKey "+rawKey, nil)
	require.Equal(t, 200, status)
	status, _ = call("read_only_key_delete_denied", "DELETE", deletePath, "ApiKey "+rawKey, nil)
	require.Equal(t, 403, status)
	require.NoError(t, db.Delete(&grants[0]).Error)
	status, _ = call("live_read_grant_revocation", "GET", path+"/runs", "ApiKey "+rawKey, nil)
	require.Equal(t, 403, status)
	// Foreign tenant rows must never be read/deleted by a root of this tenant.
	foreign := models.SchedulerJob{TenantUUID: uuid.NewString(), OwnerType: "plugin", OwnerID: owner, Name: prefix, ScheduleType: "interval", ScheduleExpr: "24h", Status: "active", PayloadJSON: []byte(`{}`)}
	require.NoError(t, db.Create(&foreign).Error)
	defer db.Unscoped().Delete(&foreign)
	status, _ = call("cross_tenant_delete", "DELETE", "/api/v1/admin/scheduler/jobs/"+foreign.UUID.String()+"?expected_revision=1", admin, nil)
	require.Equal(t, 404, status)
	// Same name, new UUID after deletion; old history remains addressable.
	recreated := create("create_api_key_fixture", owner)
	require.NotEqual(t, job.UUID, recreated.UUID)
	defer db.Unscoped().Delete(&recreated)
	adminJob := create("create_admin_fixture", owner)
	defer db.Unscoped().Delete(&adminJob)
	status, _ = call("admin_jwt_delete", "DELETE", "/api/v1/admin/scheduler/jobs/"+adminJob.UUID.String()+"?expected_revision=1", admin, nil)
	require.Equal(t, 200, status)
	deniedUser := iam.User{Email: prefix + "@example.invalid", Status: 1}
	require.NoError(t, db.Create(&deniedUser).Error)
	defer db.Unscoped().Delete(&deniedUser)
	deniedMember := iam.Member{TenantUUID: tenant, UserUUID: deniedUser.UUID.String(), UserID: deniedUser.ID, Username: prefix, Status: 1}
	require.NoError(t, db.Create(&deniedMember).Error)
	defer db.Unscoped().Delete(&deniedMember)
	deniedClaims := claims
	deniedClaims.IsRoot = false
	deniedClaims.UserID = deniedUser.ID
	deniedClaims.UserUUID = deniedUser.UUID.String()
	deniedClaims.MemberID = deniedMember.ID
	deniedClaims.MemberUUID = deniedMember.UUID.String()
	deniedToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, deniedClaims).SignedString([]byte(cfg.Auth.JWTSecret))
	require.NoError(t, err)
	status, _ = call("admin_missing_delete_rbac", "DELETE", "/api/v1/admin/scheduler/jobs/"+recreated.UUID.String()+"?expected_revision=1", "Bearer "+deniedToken, nil)
	require.Equal(t, 403, status)
	status, _ = call("new_default_list", "GET", "/api/v1/tenant/scheduler/jobs?owner_id="+owner, apiAuthorization, nil)
	require.Equal(t, 200, status)
	t.Logf("HTTP, real STS, scoped API Key, revisions, history, deletion and name reuse passed: %d records", len(records))
}
func jsonNumber(t *testing.T, value uint64) string {
	t.Helper()
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	return string(raw)
}

type blockingBus struct {
	entered chan struct{}
	release chan struct{}
	calls   atomic.Int64
}

func (b *blockingBus) Publish(string, any, context.Context) {
	b.calls.Add(1)
	close(b.entered)
	<-b.release
}
func (*blockingBus) Subscribe(string, event_bus.Handler) func() { return func() {} }
func (*blockingBus) Close() error                               { return nil }
func TestLivePostgresPublicationFenceAcrossConnections(t *testing.T) {
	cfg, db, _ := liveDB(t)
	db2, err := database.Connect(cfg.Database)
	require.NoError(t, err)
	sql2, err := db2.DB()
	require.NoError(t, err)
	defer sql2.Close()
	tenant := "6b5d0240-9920-46da-b707-88200e0f51ea"
	claims := &reqctx.CoreXClaims{TenantUUID: tenant, IsRoot: true, UserID: 1}
	ctx := reqctx.WithTenantUUID(reqctx.WithClaims(context.Background(), claims), tenant)
	bus := &blockingBus{entered: make(chan struct{}), release: make(chan struct{})}
	first := svc.NewService(svc.Options{DB: db, EventBus: bus})
	second := svc.NewService(svc.Options{DB: db2})
	job, err := first.CreateJob(ctx, svc.JobSpec{OwnerType: "plugin", OwnerID: "com.powerx.plugins.scrm", Name: "core-fence-" + uuid.NewString(), ScheduleType: "interval", ScheduleExpr: "24h", Payload: map[string]any{"acceptance": true}}, "root", "create")
	require.NoError(t, err)
	defer db.Unscoped().Delete(job)
	defer db.Unscoped().Where("job_uuid = ?", job.UUID).Delete(&models.SchedulerJobRun{})
	triggered := make(chan error, 1)
	go func() { _, err := first.TriggerJob(ctx, job.UUID.String(), "root", "fenced-trigger"); triggered <- err }()
	select {
	case <-bus.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("publication did not enter")
	}
	deleted := make(chan error, 1)
	go func() {
		_, err := second.DeleteJob(ctx, svc.DeleteJobInput{JobUUID: job.UUID.String(), ExpectedRevision: 2})
		deleted <- err
	}()
	select {
	case err := <-deleted:
		t.Fatalf("Postgres fence failed: delete returned during publication: %v", err)
	case <-time.After(80 * time.Millisecond):
	}
	close(bus.release)
	require.NoError(t, <-triggered)
	require.NoError(t, <-deleted)
	_, err = second.TriggerJob(ctx, job.UUID.String(), "root", "late")
	require.Error(t, err)
	require.EqualValues(t, 1, bus.calls.Load())
	t.Log("independent PostgreSQL connections: deletion waits for publication, then future trigger is rejected")
}
