package backup

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	budget "github.com/ArtisanCloud/PowerX/internal/infra/storage_budget"
	archive "github.com/ArtisanCloud/PowerX/internal/service/archive_ops"
	taskqueue "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/task_queue"
	gov "github.com/ArtisanCloud/PowerX/internal/service/storage_governance"
	"github.com/ArtisanCloud/PowerX/pkg/auth"
	authmw "github.com/ArtisanCloud/PowerX/pkg/auth/middleware"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/database"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	mevent "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type targetS3Object struct {
	key, version string
	body         []byte
	at           time.Time
}
type targetVersionedS3 struct {
	mu           sync.Mutex
	objects      []targetS3Object
	counter      int
	corrupt      bool
	public       bool
	publicPolicy string
}

func (m *targetVersionedS3) serve(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()
	q := r.URL.Query()
	key := strings.TrimPrefix(r.URL.Path, "/archives/")
	errOut := func(code string, status int) {
		w.WriteHeader(status)
		_, _ = fmt.Fprintf(w, "<Error><Code>%s</Code><Message>test</Message></Error>", code)
	}
	if !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 ") {
		errOut("AccessDenied", 403)
		return
	}
	if r.Method == "GET" && q.Has("versioning") {
		_, _ = io.WriteString(w, "<VersioningConfiguration><Status>Enabled</Status></VersioningConfiguration>")
		return
	}
	if r.Method == "GET" && q.Has("policy") {
		if m.publicPolicy != "" {
			_, _ = io.WriteString(w, m.publicPolicy)
			return
		}
		if m.public {
			_, _ = io.WriteString(w, `{"Statement":[{"Effect":"Allow","Principal":"*"}]}`)
		} else {
			errOut("NoSuchBucketPolicy", 404)
		}
		return
	}
	if r.Method == "GET" && q.Has("acl") {
		_, _ = io.WriteString(w, "<AccessControlPolicy><AccessControlList/></AccessControlPolicy>")
		return
	}
	if r.Method == "GET" && q.Has("lifecycle") {
		errOut("NoSuchLifecycleConfiguration", 404)
		return
	}
	if r.Method == "GET" && q.Has("versions") {
		type row struct {
			Key     string `xml:"Key"`
			Version string `xml:"VersionId"`
			Latest  bool   `xml:"IsLatest"`
			Size    int    `xml:"Size"`
			At      string `xml:"LastModified"`
		}
		var out struct {
			XMLName   xml.Name `xml:"ListVersionsResult"`
			Rows      []row    `xml:"Version"`
			Truncated bool     `xml:"IsTruncated"`
		}
		latest := map[string]string{}
		for _, o := range m.objects {
			latest[o.key] = o.version
		}
		for _, o := range m.objects {
			if strings.HasPrefix(o.key, q.Get("prefix")) {
				out.Rows = append(out.Rows, row{o.key, o.version, latest[o.key] == o.version, len(o.body), o.at.Format(time.RFC3339)})
			}
		}
		_ = xml.NewEncoder(w).Encode(out)
		return
	}
	if r.Method == "PUT" {
		if r.Header.Get("If-None-Match") == "*" {
			for _, o := range m.objects {
				if o.key == key {
					errOut("PreconditionFailed", 412)
					return
				}
			}
		}
		raw, _ := io.ReadAll(r.Body)
		m.counter++
		version := strconv.Itoa(m.counter)
		m.objects = append(m.objects, targetS3Object{key, version, raw, time.Now().UTC()})
		w.Header().Set("x-amz-version-id", version)
		return
	}
	if r.Method == "GET" {
		for i := len(m.objects) - 1; i >= 0; i-- {
			o := m.objects[i]
			if o.key == key && (q.Get("versionId") == "" || q.Get("versionId") == o.version) {
				data := o.body
				if m.corrupt && strings.Contains(key, "/objects/") {
					data = bytes.Repeat([]byte("!"), len(data))
				}
				w.Header().Set("x-amz-version-id", o.version)
				w.Header().Set("Content-Length", strconv.Itoa(len(data)))
				_, _ = w.Write(data)
				return
			}
		}
		errOut("NoSuchVersion", 404)
		return
	}
	if r.Method == "DELETE" {
		for i, o := range m.objects {
			if o.key == key && o.version == q.Get("versionId") {
				m.objects = append(m.objects[:i], m.objects[i+1:]...)
				w.WriteHeader(204)
				return
			}
		}
		errOut("NoSuchVersion", 404)
		return
	}
	errOut("InvalidRequest", 400)
}

type targetCollector struct{}

func (targetCollector) Collect(context.Context, string, []gov.Resource) gov.Snapshot {
	return gov.Snapshot{}
}

// 真正 HTTP/JWT/PostgreSQL/TaskBus/SSE；S3 使用隔离版本协议仿真，不连接客户目标。
func TestArchiveTargetNativeHTTPAndSSE(t *testing.T) { runArchiveControlNativeHTTP(t, false) }
func TestArchivePolicyNativeHTTPAndSSE(t *testing.T) { runArchiveControlNativeHTTP(t, true) }
func runArchiveControlNativeHTTP(t *testing.T, policyMode bool) {
	dsn := os.Getenv("POWERX_STORAGE_CONTROL_TEST_DSN")
	if dsn == "" {
		t.Skip("owned temporary PostgreSQL only")
	}
	cfg, e := pgx.ParseConfig(dsn)
	require.NoError(t, e)
	require.True(t, strings.HasPrefix(cfg.Host, "/tmp/powerx-control-owned-"))
	require.Equal(t, "storage_control_test", cfg.User)
	require.Equal(t, "postgres", cfg.Database)
	db, e := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, e)
	pool, e := db.DB()
	require.NoError(t, e)
	previous := model.PowerXSchema
	schema := "target_http_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	require.NoError(t, db.Exec("CREATE SCHEMA "+schema).Error)
	model.PowerXSchema = schema
	t.Cleanup(func() {
		require.NoError(t, db.Exec("DROP SCHEMA "+schema+" CASCADE").Error)
		model.PowerXSchema = previous
		require.NoError(t, pool.Close())
	})
	require.NoError(t, database.MigrateStorageGovernanceModels(db))
	require.NoError(t, db.AutoMigrate(&mevent.TaskHistory{}))
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	base, e := event_bus.NewRedisStreamTaskDriver(client, 10*time.Second, "owned-target-http")
	require.NoError(t, e)
	queue := taskqueue.NewStreamDriver(db, base)
	dir := t.TempDir()
	coordinator, e := budget.New(budget.Config{Directory: filepath.Join(dir, "runtime"), FilesystemPath: dir, HostID: "host", FilesystemID: "disk", InstanceID: "owned-target", MinFreeBytes: 1 << 20, CriticalFreeBytes: 1})
	require.NoError(t, e)
	var svc *gov.Service
	control, e := gov.NewControlService(db, gov.ControlOptions{Scope: gov.ControlScope{HostID: "host", FilesystemID: "disk", InstanceID: "owned-target"}, Queue: queue, Changed: func() {
		if svc != nil {
			svc.PublishChange("configuration_changed")
		}
	}})
	require.NoError(t, e)
	svc, e = gov.NewService(gov.Options{Identity: gov.Identity{HostID: "host", FilesystemID: "disk", InstanceID: "owned-target", Environment: "test"}, FilesystemPath: dir, Collector: targetCollector{}, DB: db, Budget: coordinator, Control: control, ControlWorkerEnabled: true})
	require.NoError(t, e)
	mock := &targetVersionedS3{}
	s3 := httptest.NewServer(http.HandlerFunc(mock.serve))
	t.Cleanup(s3.Close)
	sources := []archive.Source{}
	if policyMode {
		logs := filepath.Join(dir, "logs")
		require.NoError(t, os.Mkdir(logs, 0700))
		for _, name := range []string{"app.log", "app.log.1.gz", "app.log.2.gz", "app.log.3.gz"} {
			require.NoError(t, os.WriteFile(filepath.Join(logs, name), []byte("owned sample "+name), 0600))
			stamp := time.Now().Add(-48 * time.Hour)
			require.NoError(t, os.Chtimes(filepath.Join(logs, name), stamp, stamp))
		}
		media := filepath.Join(dir, "media")
		require.NoError(t, os.Mkdir(media, 0700))
		require.NoError(t, os.WriteFile(filepath.Join(media, "owned.txt"), []byte("owned media"), 0600))
		sources = append(sources, archive.Source{ID: "owned-media", Name: "隔离媒体", Kind: "media", Directory: media})
		sources = append(sources, archive.Source{ID: "owned-logs", Name: "隔离应用日志", Kind: "application_logs", Directory: logs, RotationFormat: "numeric", ActiveFiles: []string{"app.log"}})
	}
	a := archive.NewService(context.Background(), archive.Config{Sources: sources, Directory: filepath.Join(dir, "metadata"), Instance: "owned-target", ExecutionNodeID: "owned-target", S3Credentials: map[string]archive.S3Credential{"archive-test": {AccessKey: "owned-access", SecretKey: "owned-secret"}}}, archive.Options{RetentionNow: func() time.Time { return time.Now().Add(2 * 24 * time.Hour) }})
	t.Cleanup(a.Close)
	root := reqctx.WithClaims(context.Background(), &reqctx.CoreXClaims{UserID: 11, IsRoot: true, Scope: "access"})
	old, e := a.SaveTarget(root, archive.Target{Name: "旧归档目标", Driver: "s3", Endpoint: s3.URL, Bucket: "archives", Prefix: "owned/original", Region: "us-east-1", CredentialRef: "archive-test", ForcePathStyle: true})
	require.NoError(t, e)
	oldFile := filepath.Join(dir, "metadata", "targets", old.ID+".json")
	original, e := os.ReadFile(oldFile)
	require.NoError(t, e)
	var legacyPolicy archive.Policy
	var policyFile string
	var policyOriginal []byte
	if policyMode {
		legacyPolicy, e = a.SavePolicy(root, archive.Policy{Name: "旧应用日志策略", SourceID: "owned-logs", TargetID: old.ID, IntervalMinutes: 5, RetentionDays: 7, RetentionCount: 10, RemoteRetentionDays: 1, RemoteRetentionCount: 100, RemoteMaxBytes: 1 << 30})
		require.NoError(t, e)
		policyFile = filepath.Join(dir, "metadata", "policies", legacyPolicy.ID+".json")
		policyOriginal, e = os.ReadFile(policyFile)
		require.NoError(t, e)
	}
	require.NoError(t, svc.BindArchiveControl(a))
	ctx, cancel := context.WithCancel(context.Background())
	svc.StartControl(ctx, func(e error) { t.Logf("owned control worker: %v", e) })
	t.Cleanup(func() {
		cancel()
		select {
		case <-control.Done():
		case <-time.After(10 * time.Second):
			t.Error("owned worker did not stop")
		}
	})
	secret := []byte("owned-archive-target-http-no-deployment-key")
	const issuer = "owned-archive-target-http"
	sign := func(claims reqctx.CoreXClaims) string {
		claims.TenantUUID = uuid.NewString()
		claims.MemberUUID = uuid.NewString()
		token, e := auth.GenerateAccessJWT(claims, issuer, []string{"admin"}, time.Hour, secret)
		require.NoError(t, e)
		return token
	}
	token := sign(reqctx.CoreXClaims{UserID: 11, IsRoot: true})
	router := gin.New()
	protected := router.Group("/custom/v2", authmw.JwtMiddleware(secret, issuer, []string{"admin"}, []string{"access"}, nil))
	registerProtectedRoutes(protected, &shared.Deps{DB: db, Archives: a, StorageGovernance: svc})
	finish := make(chan struct{}, 1)
	router.POST("/custom/v2/admin/ops/backup/archives/__owned-test/finish", func(c *gin.Context) {
		if c.GetHeader("Authorization") != "Bearer "+token {
			c.Status(403)
			return
		}
		select {
		case finish <- struct{}{}:
		default:
		}
		c.Status(204)
	})
	server := httptest.NewServer(router)
	address := strings.TrimPrefix(server.URL, "http://")
	t.Logf("owned target HTTP: pid=%d cwd=%s command=go-test listener=%s; S3 fixture=%s", os.Getpid(), dir, address, s3.URL)
	t.Cleanup(func() {
		server.Close()
		conn, e := net.DialTimeout("tcp", address, 100*time.Millisecond)
		if e == nil {
			conn.Close()
			t.Error("owned HTTP listener remains open")
		}
	})
	request := func(method, path string, input any, jwt string) (int, map[string]any) {
		var body io.Reader
		if input != nil {
			raw, e := json.Marshal(input)
			require.NoError(t, e)
			body = bytes.NewReader(raw)
		}
		r, e := http.NewRequest(method, server.URL+"/custom/v2/admin/ops/backup/archives"+path, body)
		require.NoError(t, e)
		r.Header.Set("Content-Type", "application/json")
		if strings.HasSuffix(path, "/run") {
			r.Header.Set("Idempotency-Key", uuid.NewString())
		}
		if jwt != "" {
			r.Header.Set("Authorization", "Bearer "+jwt)
		}
		resp, e := http.DefaultClient.Do(r)
		require.NoError(t, e)
		defer resp.Body.Close()
		raw, e := io.ReadAll(resp.Body)
		require.NoError(t, e)
		var out map[string]any
		require.NoError(t, json.Unmarshal(raw, &out))
		return resp.StatusCode, out
	}
	for _, candidate := range []struct {
		token  string
		status int
	}{{"", 401}, {sign(reqctx.CoreXClaims{UserID: 12}), 403}, {sign(reqctx.CoreXClaims{UserID: 11, IsRoot: true, Platforms: []string{"api_key"}}), 403}} {
		status, _ := request("GET", "/target-configuration", nil, candidate.token)
		require.Equal(t, candidate.status, status)
		status, _ = request("POST", "/targets", map[string]any{}, candidate.token)
		require.Equal(t, candidate.status, status)
		for _, path := range []string{"/policy-configuration", "/policy-configuration/requests/" + uuid.NewString()} {
			status, _ = request("GET", path, nil, candidate.token)
			require.Equal(t, candidate.status, status)
		}
		status, _ = request("POST", "/policy-configuration/adoptions", map[string]any{}, candidate.token)
		require.Equal(t, candidate.status, status)
	}
	destination := os.Getenv("POWERX_TARGET_BROWSER_DIR")
	if policyMode {
		destination = os.Getenv("POWERX_POLICY_BROWSER_DIR")
	}
	if destination != "" {
		require.True(t, filepath.IsAbs(destination))
		credential := filepath.Join(destination, "owned-token")
		require.NoError(t, os.WriteFile(credential, []byte(token), 0600))
		defer os.Remove(credential)
		owner, _ := json.Marshal(map[string]any{"owner": "codex archive target HTTP/S3/PG fixture", "pid": os.Getpid(), "url": server.URL, "cwd": dir, "command": "go test ./internal/transport/http/admin/backup -run ^" + t.Name() + "$ -count=1 -v", "credential_file": credential, "s3_endpoint": s3.URL})
		require.NoError(t, os.WriteFile(filepath.Join(destination, "http-owner.json"), owner, 0600))
		select {
		case <-finish:
		case <-time.After(5 * time.Minute):
			t.Fatal("owned target browser timed out")
		}
	} else {
		status, out := request("GET", "/target-configuration", nil, token)
		require.Equal(t, 200, status)
		configuration := out["data"].(map[string]any)
		status, _ = request("POST", "/targets", old, token)
		require.Equal(t, 400, status, "old JSON payload cannot write configuration")
		r, e := http.NewRequestWithContext(ctx, "GET", server.URL+"/custom/v2/admin/ops/backup/events", nil)
		require.NoError(t, e)
		r.Header.Set("Authorization", "Bearer "+token)
		resp, e := http.DefaultClient.Do(r)
		require.NoError(t, e)
		defer resp.Body.Close()
		require.Equal(t, 200, resp.StatusCode)
		reader := bufio.NewReader(resp.Body)
		_, e = reader.ReadString('\n')
		require.NoError(t, e)
		key := uuid.NewString()
		adopt := map[string]any{"request_id": key, "legacy_digest": configuration["legacy_digest"], "confirm": true}
		status, _ = request("POST", "/target-configuration/adoptions", adopt, token)
		require.Equal(t, 200, status)
		require.Eventually(t, func() bool { out, e := a.TargetConfiguration(root); return e == nil && out.State == "applied" }, 5*time.Second, 20*time.Millisecond)
		require.NoError(t, a.TestTarget(root, old.ID))
		outview, e := a.Overview(root)
		require.NoError(t, e)
		require.True(t, outview.Targets[0].VerificationValid)
		status, out = request("GET", "/target-configuration/requests/"+key, nil, token)
		require.Equal(t, 200, status)
		require.EqualValues(t, 1, out["data"].(map[string]any)["operation_revision"])
		status, _ = request("POST", "/target-configuration/adoptions", adopt, token)
		require.Equal(t, 200, status)
		status, _ = request("POST", "/targets", map[string]any{"request_id": uuid.NewString(), "expected_revision": 1, "expected_target_revision": 2, "target_id": old.ID, "settings": map[string]any{"secret_key": "forbidden"}}, token)
		require.Equal(t, 400, status)
		if policyMode {
			status, out = request("GET", "/policy-configuration", nil, token)
			require.Equal(t, 200, status)
			policyConfiguration := out["data"].(map[string]any)
			adoptPolicy := map[string]any{"request_id": uuid.NewString(), "legacy_digest": policyConfiguration["legacy_digest"], "confirm": true}
			status, _ = request("POST", "/policy-configuration/adoptions", adoptPolicy, token)
			require.Equal(t, 200, status)
			require.Eventually(t, func() bool { out, e := a.PolicyConfiguration(root); return e == nil && out.State == "applied" }, 5*time.Second, 20*time.Millisecond)
			state, e := a.PolicyConfiguration(root)
			require.NoError(t, e)
			require.False(t, state.Policies[0].Enabled)
			require.False(t, state.Policies[0].RemoteCleanupEnabled)
			status, _ = request("POST", "/policies", legacyPolicy, token)
			require.Equal(t, 400, status)
			target := outview.Targets[0]
			change := map[string]any{"request_id": uuid.NewString(), "expected_revision": 1, "expected_policy_revision": legacyPolicy.Revision + 1, "policy_id": legacyPolicy.ID, "action": "status", "enabled": true, "remote_cleanup_enabled": false, "confirm": true, "expected_target_revision": target.Revision, "expected_target_fingerprint": "wrong"}
			status, _ = request("POST", "/policies", change, token)
			require.Equal(t, 409, status)
			change["expected_target_fingerprint"] = target.VerifiedFingerprint
			status, _ = request("POST", "/policies", change, token)
			require.Equal(t, 200, status)
			require.Eventually(t, func() bool {
				out, e := a.PolicyConfiguration(root)
				return e == nil && out.State == "applied" && out.DesiredRevision == 2
			}, 5*time.Second, 20*time.Millisecond)
			// 单连接池下的真正 S3 写入与删除；策略、目标锁复用同一 SQL 事务。
			pool.SetMaxOpenConns(1)
			lastState := ""
			for n := 1; n <= 2; n++ {
				status, _ = request("POST", "/policies/"+legacyPolicy.ID+"/run", map[string]any{}, token)
				require.Equal(t, 202, status)
				require.Eventually(t, func() bool {
					v, e := a.Overview(root)
					if e != nil {
						return false
					}
					count := 0
					summary := ""
					for _, j := range v.Jobs {
						summary += j.Status + "/" + j.RemoteState + "/" + j.Error + "/" + j.RemoteError + ";"
						if j.Status == "success" && j.RemoteState == "success" {
							count++
						}
					}
					if summary != lastState {
						t.Logf("owned archive job: %s", summary)
						lastState = summary
					}
					return count == n
				}, 5*time.Second, 20*time.Millisecond)
			}
			change["request_id"] = uuid.NewString()
			change["expected_revision"] = 2
			change["expected_policy_revision"] = legacyPolicy.Revision + 2
			change["remote_cleanup_enabled"] = true
			status, _ = request("POST", "/policies", change, token)
			require.Equal(t, 200, status)
			require.Eventually(t, func() bool {
				out, e := a.PolicyConfiguration(root)
				return e == nil && out.State == "applied" && out.DesiredRevision == 3
			}, 5*time.Second, 20*time.Millisecond)
			status, out = request("POST", "/policies/"+legacyPolicy.ID+"/remote-cleanup-previews", map[string]any{}, token)
			require.Equal(t, 200, status)
			preview := out["data"].(map[string]any)
			require.Greater(t, preview["candidate_versions"].(float64), float64(0))
			previewID := preview["id"].(string)
			status, _ = request("POST", "/remote-cleanup-runs/"+previewID, map[string]any{"confirmation": previewID}, token)
			require.Equal(t, 202, status)
			require.Eventually(t, func() bool {
				v, e := a.Overview(root)
				if e != nil {
					return false
				}
				for _, run := range v.RemoteRuns {
					if run.ID == previewID {
						return run.State == "completed"
					}
				}
				return false
			}, 5*time.Second, 20*time.Millisecond)
			change["request_id"] = uuid.NewString()
			change["expected_revision"] = 3
			change["expected_policy_revision"] = legacyPolicy.Revision + 3
			change["enabled"] = false
			change["remote_cleanup_enabled"] = false
			change["expected_target_revision"] = 0
			change["expected_target_fingerprint"] = ""
			status, _ = request("POST", "/policies", change, token)
			require.Equal(t, 200, status)
			require.Eventually(t, func() bool {
				out, e := a.PolicyConfiguration(root)
				return e == nil && out.State == "applied" && out.DesiredRevision == 4
			}, 5*time.Second, 20*time.Millisecond)
			status, _ = request("POST", "/policies/"+legacyPolicy.ID+"/run", map[string]any{}, token)
			require.Equal(t, 409, status)
		}

	}
	final, e := os.ReadFile(oldFile)
	require.NoError(t, e)
	require.Equal(t, original, final)
	view, e := a.Overview(root)
	require.NoError(t, e)
	if !policyMode {
		require.Empty(t, view.Policies)
		require.Empty(t, view.Jobs)
	} else {
		final, e := os.ReadFile(policyFile)
		require.NoError(t, e)
		require.Equal(t, policyOriginal, final)
	}
	mock.mu.Lock()
	if !policyMode {
		require.Empty(t, mock.objects, "verification objects must be cleaned")
	} else {
		for _, object := range mock.objects {
			require.NotContains(t, object.key, "/_tests/", "verification objects must be cleaned")
		}
	}
	mock.mu.Unlock()
}
