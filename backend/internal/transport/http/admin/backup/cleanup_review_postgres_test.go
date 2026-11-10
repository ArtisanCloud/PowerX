package backup

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	storagerecovery "github.com/ArtisanCloud/PowerX/internal/infra/storage_recovery"
	backupops "github.com/ArtisanCloud/PowerX/internal/service/backup_ops"
	modelops "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	budget "github.com/ArtisanCloud/PowerX/internal/infra/storage_budget"
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

func TestCleanupReviewNativeHTTPAndSSE(t *testing.T) {
	dsn := os.Getenv("POWERX_STORAGE_CONTROL_TEST_DSN")
	if dsn == "" {
		t.Skip("owned temporary PostgreSQL only")
	}
	cfg, e := pgx.ParseConfig(dsn)
	require.NoError(t, e)
	require.True(t, strings.HasPrefix(cfg.Host, "/tmp/powerx-control-owned-"))
	require.Equal(t, "storage_control_test", cfg.User)
	db, e := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, e)
	pool, e := db.DB()
	require.NoError(t, e)
	previous := model.PowerXSchema
	schema := "backup_policy_http_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	require.NoError(t, db.Exec("CREATE SCHEMA "+schema).Error)
	model.PowerXSchema = schema
	t.Cleanup(func() {
		require.NoError(t, db.Exec("DROP SCHEMA "+schema+" CASCADE").Error)
		model.PowerXSchema = previous
		require.NoError(t, pool.Close())
	})
	require.NoError(t, database.MigrateStorageGovernanceModels(db))
	require.NoError(t, database.MigrateBackupPolicyControlModels(db))
	require.NoError(t, db.AutoMigrate(&mevent.TaskHistory{}))
	mini := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = redisClient.Close() })
	base, e := event_bus.NewRedisStreamTaskDriver(redisClient, 10*time.Second, "owned-monitoring-http")
	require.NoError(t, e)
	queue := taskqueue.NewStreamDriver(db, base)
	dir, e := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, e)
	t.Setenv("POWERX_OPS_BACKUP_ARTIFACT_DIR", dir)
	minimum, critical := int64(1<<20), int64(1)
	if os.Getenv("POWERX_BACKUP_REVIEW_BROWSER_DIR") != "" {
		minimum = 20 << 30
		critical = 10 << 30
	}
	coordinator, e := budget.New(budget.Config{Directory: filepath.Join(dir, "runtime"), FilesystemPath: dir, HostID: "host", FilesystemID: "disk", InstanceID: "owned-http", MinFreeBytes: minimum, CriticalFreeBytes: critical})
	require.NoError(t, e)
	var svc *gov.Service
	control, e := gov.NewControlService(db, gov.ControlOptions{Scope: gov.ControlScope{HostID: "host", FilesystemID: "disk", InstanceID: "owned-http"}, Queue: queue, Changed: func() {
		if svc != nil {
			svc.PublishChange("configuration_changed")
		}
	}})
	require.NoError(t, e)
	svc, e = gov.NewService(gov.Options{Identity: gov.Identity{HostID: "host", FilesystemID: "disk", InstanceID: "owned-http", Environment: "test"}, FilesystemPath: dir, Collector: targetCollector{}, DB: db, Budget: coordinator, Control: control, ControlWorkerEnabled: true})
	require.NoError(t, e)
	maintenance := filepath.Join(dir, "maintenance")
	require.NoError(t, os.Mkdir(maintenance, 0700))
	anchor, e := storagerecovery.Initialize(maintenance, "owned-http")
	require.NoError(t, e)
	require.NoError(t, svc.BindRecoveryAnchor(anchor))
	policies, jobs := backupops.NewPolicyService(db), backupops.NewJobService(db)
	require.NoError(t, svc.BindDatabaseBackupControl(policies, jobs, "test"))
	ctx, cancel := context.WithCancel(context.Background())
	svc.StartControl(ctx, func(err error) { t.Logf("owned budget control worker: %v", err) })
	t.Cleanup(func() {
		cancel()
		select {
		case <-control.Done():
		case <-time.After(10 * time.Second):
			t.Error("owned publication worker did not stop")
		}
	})
	secret := []byte("owned-monitor-http-no-deployment-key")
	const issuer = "owned-monitoring"
	token, e := auth.GenerateAccessJWT(reqctx.CoreXClaims{UserID: 1, IsRoot: true, TenantUUID: uuid.NewString(), MemberUUID: uuid.NewString()}, issuer, []string{"admin"}, time.Hour, secret)
	require.NoError(t, e)
	router := gin.New()
	protected := router.Group("/custom/v2", authmw.JwtMiddleware(secret, issuer, []string{"admin"}, []string{"access"}, nil))
	registerProtectedRoutes(protected, &shared.Deps{DB: db, StorageGovernance: svc, DatabaseBackupPolicies: policies, DatabaseBackupJobs: jobs})
	finish := make(chan struct{}, 1)
	if os.Getenv("POWERX_BACKUP_REVIEW_BROWSER_DIR") != "" {
		protected.POST("/__owned-test/finish", RequireBackupRoot(), func(c *gin.Context) {
			select {
			case finish <- struct{}{}:
			default:
			}
			c.Status(204)
		})
	}
	protected.POST("/__owned-test/sync", RequireBackupRoot(), func(c *gin.Context) { svc.PublishChange("backup_cleanup_changed"); c.Status(204) })
	protected.POST("/__owned-test/change", RequireBackupRoot(), func(c *gin.Context) {
		require.NoError(t, db.Model(&modelops.BackupArtifact{}).Where("cleanup_state IN ?", []string{"pending", "unconfirmed"}).Update("updated_at", time.Now().UTC()).Error)
		svc.PublishChange("backup_cleanup_changed")
		c.Status(204)
	})
	server := httptest.NewServer(router)
	address := strings.TrimPrefix(server.URL, "http://")
	fixtureCWD, cwdErr := os.Getwd()
	require.NoError(t, cwdErr)
	t.Logf("owned HTTP server: pid=%d cwd=%s command=go-test listener=%s artifacts=%s", os.Getpid(), fixtureCWD, address, dir)
	t.Cleanup(func() {
		server.Close()
		c, e := net.DialTimeout("tcp", address, 100*time.Millisecond)
		if e == nil {
			c.Close()
			t.Error("owned HTTP listener still open")
		}
	})

	t.Setenv("POWERX_OPS_BACKUP_ARTIFACT_DIR", dir)
	job := modelops.BackupJob{PolicyID: 1, Status: modelops.BackupJobStatusSuccess, TriggerType: modelops.BackupTriggerTypeManual, Operator: "owned-review"}
	require.NoError(t, db.Create(&job).Error)
	path := filepath.Join(dir, "owned.dump")
	content := []byte("owned backup data")
	hash := sha256.Sum256(content)
	artifact := modelops.BackupArtifact{JobID: job.ID, StorageURI: "file://" + path, SizeBytes: int64(len(content)), Checksum: hex.EncodeToString(hash[:]), CleanupState: "unconfirmed", CleanupToken: uuid.NewString()}
	require.NoError(t, db.Create(&artifact).Error)
	for _, suffix := range []string{"", ".json", ".sha256"} {
		require.NoError(t, os.WriteFile(path+suffix, content, 0600))
	}
	call := func(path, jwt string) (int, []byte) {
		req, e := http.NewRequest("GET", server.URL+"/custom/v2/admin/ops/backup"+path, nil)
		require.NoError(t, e)
		if jwt != "" {
			req.Header.Set("Authorization", "Bearer "+jwt)
		} else {
			req.Header.Set("X-API-Key", "owned-key")
		}
		res, e := server.Client().Do(req)
		require.NoError(t, e)
		defer res.Body.Close()
		b, e := io.ReadAll(res.Body)
		require.NoError(t, e)
		return res.StatusCode, b
	}
	sign := func(c reqctx.CoreXClaims) string {
		c.TenantUUID = uuid.NewString()
		c.MemberUUID = uuid.NewString()
		v, e := auth.GenerateAccessJWT(c, issuer, []string{"admin"}, time.Hour, secret)
		require.NoError(t, e)
		return v
	}
	detail := fmt.Sprintf("/cleanup-reviews/%d?expected_token=%s", artifact.ID, artifact.CleanupToken)
	for _, jwt := range []string{"", sign(reqctx.CoreXClaims{UserID: 2}), sign(reqctx.CoreXClaims{UserID: 1, IsRoot: true, Platforms: []string{"service_actor"}}), sign(reqctx.CoreXClaims{UserID: 1, IsRoot: true, Platforms: []string{"api_key"}})} {
		for _, p := range []string{"/cleanup-reviews", detail, fmt.Sprintf("/cleanup-reviews/%d/verification?expected_token=%s", artifact.ID, artifact.CleanupToken), fmt.Sprintf("/cleanup-reviews/%d/retentions?expected_token=%s", artifact.ID, artifact.CleanupToken), "/cleanup-retentions/" + uuid.NewString()} {
			status, _ := call(p, jwt)
			require.Contains(t, []int{401, 403}, status)
		}
	}
	status, b := call("/cleanup-reviews", token)
	require.Equal(t, 200, status)
	require.Contains(t, string(b), artifact.CleanupToken)
	require.NotContains(t, string(b), dir)
	status, b = call(detail, token)
	require.Equal(t, 200, status)
	require.Contains(t, string(b), `"state":"present_unverified"`)
	require.Contains(t, string(b), `"content_verified":false`)
	require.NotContains(t, string(b), path)
	for _, p := range []string{"/cleanup-reviews?page=0", "/cleanup-reviews?page=1&page=2", "/cleanup-reviews?page_size=101", "/cleanup-reviews?token=private", "/cleanup-reviews/01?expected_token=" + artifact.CleanupToken, "/cleanup-reviews/1?expected_token=bad"} {
		status, _ = call(p, token)
		require.Equal(t, 400, status)
	}
	// Unknown intents survive metadata inspection and remain safe after file absence.
	require.NoError(t, os.Remove(path))
	status, b = call(detail, token)
	require.Equal(t, 200, status)
	require.Contains(t, string(b), `"state":"partial"`)
	require.NoError(t, os.WriteFile(path, content, 0600))
	oldToken := artifact.CleanupToken
	artifact.CleanupToken = uuid.NewString()
	require.NoError(t, db.Model(&artifact).Update("cleanup_token", artifact.CleanupToken).Error)
	status, _ = call(detail, token)
	require.Equal(t, 409, status)
	status, _ = call(fmt.Sprintf("/cleanup-reviews/%d/verification?expected_token=%s", artifact.ID, oldToken), token)
	require.Equal(t, 409, status)
	require.NoError(t, db.First(&artifact, artifact.ID).Error)
	require.Equal(t, "unconfirmed", artifact.CleanupState)
	require.NotEqual(t, oldToken, artifact.CleanupToken)
	// Existing SSE is shared; notify exactly the new invalidation resource.
	req, e := http.NewRequest("GET", server.URL+"/custom/v2/admin/ops/backup/events", nil)
	require.NoError(t, e)
	req.Header.Set("Authorization", "Bearer "+token)
	res, e := server.Client().Do(req)
	require.NoError(t, e)
	require.Equal(t, 200, res.StatusCode)
	reader := bufio.NewReader(res.Body)
	initial := ""
	for !strings.HasSuffix(initial, "\n\n") {
		line, e := reader.ReadString('\n')
		require.NoError(t, e)
		initial += line
	}
	require.Contains(t, initial, "backup_cleanup")
	svc.PublishChange("backup_cleanup_changed")
	event := make(chan string, 1)
	go func() {
		raw := ""
		for !strings.HasSuffix(raw, "\n\n") {
			line, e := reader.ReadString('\n')
			if e != nil {
				return
			}
			raw += line
		}
		event <- raw
	}()
	select {
	case raw := <-event:
		require.Contains(t, raw, "backup_cleanup")
	case <-time.After(4 * time.Second):
		t.Fatal("missing cleanup invalidation")
	}
	require.NoError(t, res.Body.Close())
	exerciseCleanupVerificationHTTP(t, db, server.URL, token, path, artifact, job.PolicyID, content)
	exerciseCleanupRetentionHTTP(t, db, svc, policies, jobs, server.URL, token, artifact)
	exerciseRetentionHistoryHTTP(t, db, jobs, server.URL, token, artifact)
	if target := os.Getenv("POWERX_BACKUP_REVIEW_BROWSER_DIR"); target != "" {
		exerciseCleanupResolutionHTTP(t, db, jobs, server.URL, token, artifact)
		if os.Getenv("POWERX_BACKUP_QUARANTINE_BROWSER") == "1" {
			exerciseCleanupQuarantineHTTP(t, db, jobs, server.URL, token, artifact)
			if os.Getenv("POWERX_QUARANTINE_INVENTORY_BROWSER") == "1" {
				root := reqctx.WithClaims(context.Background(), &reqctx.CoreXClaims{UserID: 1, IsRoot: true, Scope: "access"})
				preview, e := jobs.PreviewCleanupQuarantine(root, artifact.ID, artifact.CleanupToken, func(backupops.CleanupVerification) error { return nil })
				require.NoError(t, e)
				require.True(t, preview.CanQuarantine)
				input := backupops.CleanupQuarantineInput{CleanupResolutionInput: backupops.CleanupResolutionInput{RequestID: uuid.NewString(), ExpectedToken: artifact.CleanupToken, ReviewVersion: preview.ReviewVersion, HistoryDigest: preview.HistoryDigest, Reason: "隔离台账浏览器验收，仅自有数据", Confirm: true}, ExpectedDiagnosis: preview.Diagnosis, DiagnosticDigest: preview.DiagnosticDigest}
				_, e = jobs.ResolveCleanupQuarantine(root, artifact.ID, input, func(backupops.CleanupResolutionEvent) error { return nil })
				require.NoError(t, e)
				protected.POST("/__owned-test/repair-file", RequireBackupRoot(), func(c *gin.Context) { require.NoError(t, os.WriteFile(path, content, 0600)); c.Status(204) })
				protected.POST("/__owned-test/replay-marker", RequireBackupRoot(), func(c *gin.Context) {
					require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
						if e := tx.Model(&modelops.BackupArtifact{}).Where("id=?", artifact.ID).Update("cleanup_state", "unconfirmed").Error; e != nil {
							return e
						}
						return tx.Model(&modelops.BackupCleanupResolution{}).Where("request_id=?", input.RequestID).Updates(map[string]any{"state": "checking", "completed_at": nil}).Error
					}))
					svc.PublishChange("backup_cleanup_changed")
					c.Status(204)
				})
			}
		}
		// Isolated browser-only fault. No production route can inject SQL failures.
		protected.POST("/__owned-test/ledger-orphan", RequireBackupRoot(), func(c *gin.Context) {
			clone := modelops.BackupArtifact{JobID: artifact.JobID, StorageURI: artifact.StorageURI, SizeBytes: artifact.SizeBytes, Checksum: artifact.Checksum, CleanupState: "unconfirmed", CleanupToken: uuid.NewString()}
			require.NoError(t, db.Create(&clone).Error)
			view, e := jobs.InspectCleanupReview(c.Request.Context(), clone.ID, clone.CleanupToken)
			require.NoError(t, e)
			input := backupops.CleanupRetainInput{RequestID: uuid.NewString(), ExpectedToken: clone.CleanupToken, ReviewVersion: view.Item.ReviewVersion, Reason: "隔离验收：SQL 接纳前中断，保留独立凭据", Confirm: true}
			name := "owned_browser_pre_sql_fault"
			require.NoError(t, db.Callback().Create().Before("gorm:create").Register(name, func(tx *gorm.DB) {
				if tx.Statement.Table == "ops_backup_cleanup_retentions" {
					tx.AddError(errors.New("owned browser SQL admission fault"))
				}
			}))
			_, e = jobs.RetainCleanupReview(c.Request.Context(), clone.ID, input, func(backupops.CleanupRetainEvent) error { return nil })
			db.Callback().Create().Remove(name)
			require.Error(t, e)
			c.JSON(200, gin.H{"request_id": input.RequestID, "artifact_id": fmt.Sprint(clone.ID), "token": clone.CleanupToken})
		})
		credential := filepath.Join(target, "owned-token")
		require.NoError(t, os.WriteFile(credential, []byte(token), 0600))
		defer os.Remove(credential)
		owner, _ := json.Marshal(map[string]any{"owner": "codex cleanup review fixture", "pid": os.Getpid(), "cwd": fixtureCWD, "artifact_directory": dir, "command": "go test ./internal/transport/http/admin/backup -run ^TestCleanupReviewNativeHTTPAndSSE$ -count=1 -v", "url": server.URL, "credential_file": credential, "artifact_id": artifact.ID, "job_id": job.ID})
		require.NoError(t, os.WriteFile(filepath.Join(target, "http-owner.json"), owner, 0600))
		select {
		case <-finish:
		case <-time.After(8 * time.Minute):
			t.Fatal("browser fixture timeout")
		}
		return // Browser may deliberately settle the intent; remaining read-only race fixtures run in the native-only invocation.
	}

	// Include orphaned and soft-deleted unknown intents without making them actionable.
	orphan := modelops.BackupArtifact{JobID: 99999, StorageURI: "file://" + path, SizeBytes: 17, Checksum: artifact.Checksum, CleanupState: "pending", CleanupToken: uuid.NewString()}
	require.NoError(t, db.Create(&orphan).Error)
	status, b = call("/cleanup-reviews", token)
	require.Equal(t, 200, status)
	require.Contains(t, string(b), "BACKUP_REVIEW_RECORD_UNAVAILABLE")
	require.NoError(t, db.Delete(&orphan).Error)
	status, b = call("/cleanup-reviews", token)
	require.Equal(t, 200, status)
	require.Contains(t, string(b), orphan.CleanupToken)
	// A concurrent state change during the first SELECT invalidates the snapshot.
	inspectionDB, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	inspectionPool, err := inspectionDB.DB()
	require.NoError(t, err)
	defer inspectionPool.Close()
	inspectionJobs := backupops.NewJobService(inspectionDB)
	changed := false
	require.NoError(t, inspectionDB.Callback().Row().After("gorm:row").Register("owned_review_race", func(tx *gorm.DB) {
		if !changed && strings.Contains(tx.Statement.SQL.String(), "a.cleanup_state") {
			changed = true
			require.NoError(t, db.Model(&modelops.BackupArtifact{}).Where("id = ?", artifact.ID).Update("cleanup_state", "pending").Error)
		}
	}))
	_, err = inspectionJobs.InspectCleanupReview(reqctx.WithClaims(context.Background(), &reqctx.CoreXClaims{UserID: 1, IsRoot: true, Scope: "access"}), artifact.ID, artifact.CleanupToken)
	require.NoError(t, inspectionDB.Callback().Row().Remove("owned_review_race"))
	require.True(t, changed)
	require.ErrorContains(t, err, "BACKUP_REVIEW_INTENT_CHANGED")
	require.NoError(t, db.Model(&artifact).Update("cleanup_state", "unconfirmed").Error)
	// A missing control column returns a typed 503 without SQL or file paths.
	table := modelops.BackupArtifact{}.TableName()
	require.NoError(t, db.Exec("ALTER TABLE "+table+" RENAME COLUMN cleanup_token TO owned_missing_token").Error)
	status, b = call("/cleanup-reviews", token)
	require.Equal(t, 503, status)
	require.Contains(t, string(b), "BACKUP_CLEANUP_REVIEW_SCHEMA_NOT_READY")
	require.NotContains(t, string(b), table)
	status, b = call(fmt.Sprintf("/cleanup-reviews/%d/verification?expected_token=%s", artifact.ID, artifact.CleanupToken), token)
	require.Equal(t, 503, status)
	require.Contains(t, string(b), "BACKUP_CLEANUP_REVIEW_SCHEMA_NOT_READY")
	require.NotContains(t, string(b), table)
	require.NoError(t, db.Exec("ALTER TABLE "+table+" RENAME COLUMN owned_missing_token TO cleanup_token").Error)
	// Read-only APIs did not remove data or clear the unknown marker.
	var count int64
	require.NoError(t, db.Model(&modelops.BackupArtifact{}).Where("cleanup_state = ? AND cleanup_token = ?", "unconfirmed", artifact.CleanupToken).Count(&count).Error)
	require.Equal(t, int64(1), count)
	got, e := os.ReadFile(path)
	require.NoError(t, e)
	require.True(t, bytes.Equal(content, got))
	exerciseCleanupResolutionHTTP(t, db, jobs, server.URL, token, artifact)
	qjob := modelops.BackupJob{PolicyID: job.PolicyID, Status: modelops.BackupJobStatusSuccess, Operator: "owned-quarantine"}
	require.NoError(t, db.Create(&qjob).Error)
	qartifact := modelops.BackupArtifact{JobID: qjob.ID, StorageURI: artifact.StorageURI, SizeBytes: artifact.SizeBytes, Checksum: artifact.Checksum, CleanupState: "unconfirmed", CleanupToken: uuid.NewString()}
	require.NoError(t, db.Create(&qartifact).Error)
	exerciseCleanupQuarantineHTTP(t, db, jobs, server.URL, token, qartifact)
}
