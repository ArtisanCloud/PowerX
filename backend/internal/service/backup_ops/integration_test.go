package backup_ops

import (
	"context"
	"errors"
	"fmt"
	modelops "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	"github.com/jackc/pgx/v5"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// 必须显式提供测试管理员连接；测试仅创建和删除它自己生成的新数据库。
func TestBackupIntegration(t *testing.T) {
	adminDSN := os.Getenv("POWERX_BACKUP_TEST_ADMIN_DSN")
	if adminDSN == "" {
		t.Skip("POWERX_BACKUP_TEST_ADMIN_DSN is required")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(ctx)
	name := fmt.Sprintf("powerx_backup_test_%d", time.Now().UnixNano())
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, e := admin.Exec(ctx, "DROP DATABASE "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)"); e != nil {
			t.Error(e)
		}
	}()
	cfg, err := pgx.ParseConfig(adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	u := url.URL{Scheme: "postgres", Host: cfg.Host, Path: "/" + name, User: url.UserPassword(cfg.User, cfg.Password)}
	// Unix socket 连接保留在 query 的 host 中。
	q := url.Values{"sslmode": {"disable"}, "port": {fmt.Sprint(cfg.Port)}}
	if strings.HasPrefix(cfg.Host, "/") {
		u.Host = ""
		q.Set("host", cfg.Host)
	}
	u.RawQuery = q.Encode()
	db, err := gorm.Open(postgres.Open(u.String()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), DisableForeignKeyConstraintWhenMigrating: true})
	if err != nil {
		t.Fatal(err)
	}
	pool, _ := db.DB()
	defer pool.Close()
	if err = db.AutoMigrate(&modelops.BackupPolicy{}, &modelops.BackupJob{}, &modelops.BackupArtifact{}, &modelops.RestoreDrillRecord{}); err != nil {
		t.Fatal(err)
	}
	if err = db.Exec("CREATE SCHEMA sample_plugin; CREATE TABLE sample_plugin.orders (id integer, name text); INSERT INTO sample_plugin.orders VALUES (1,'first order'),(2,'second order')").Error; err != nil {
		t.Fatal(err)
	}
	root, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	t.Setenv("POWERX_OPS_BACKUP_ARTIFACT_DIR", root)
	scriptDir, err := filepath.Abs("../../../scripts/ops")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("POWERX_OPS_SCRIPT_DIR", scriptDir)
	t.Setenv("POWERX_OPS_RESTORE_KEEP_DB", "1")
	ps := NewPolicyService(db)
	ps.auditor = nil
	policy, err := ps.CreatePolicy(ctx, CreatePolicyRequest{Name: "integration", IntervalHours: 1, RetentionCount: 1, RetentionDays: 7, RetentionMode: "age_and_count"})
	if err != nil {
		t.Fatal(err)
	}
	if policy.Enabled || policy.DrillEnabled {
		t.Fatal("new policy was automatically enabled")
	}
	svc := NewJobService(db)
	svc.auditor = nil
	svc.alertSvc = nil
	svc.restoreSvc = nil
	first, err := svc.TriggerJob(ctx, TriggerJobRequest{PolicyID: policy.ID})
	if err != nil || first.Status != "success" {
		t.Fatalf("backup failed: %v %+v", err, first)
	}
	artifact, err := svc.artifactRepo.GetLatestByJobID(ctx, first.ID)
	if err != nil || artifact == nil {
		t.Fatalf("artifact missing %v", err)
	}
	firstPath := strings.TrimPrefix(artifact.StorageURI, "file://")
	info, err := os.Stat(firstPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{".json", ".sha256"} {
		if _, e := os.Stat(firstPath + suffix); e != nil {
			t.Fatalf("missing backup sidecar: %v", e)
		}
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("unsafe file mode: %v", info.Mode())
	}
	if err = svc.SetProtected(ctx, first.ID, true, "test", ""); err != nil {
		t.Fatal(err)
	}
	drillSvc := NewRestoreDrillService(db)
	drillSvc.auditor = nil
	drill, err := drillSvc.Trigger(ctx, TriggerRestoreDrillRequest{SourceJobID: first.ID})
	if err != nil || drill.Status != "success" {
		t.Fatalf("restore failed: %v %+v", err, drill)
	}
	if !strings.Contains(drill.ReportURI, "tables=") || !strings.Contains(drill.ReportURI, "keep_db=1") {
		t.Fatal(drill.ReportURI)
	}
	var restoredDB string
	for _, field := range strings.Fields(drill.ReportURI) {
		if strings.HasPrefix(field, "db=") {
			restoredDB = strings.TrimPrefix(field, "db=")
		}
	}
	if !strings.HasPrefix(restoredDB, "powerx_restore_probe_") || restoredDB == name {
		t.Fatal("invalid isolated restore database")
	}
	defer admin.Exec(ctx, "DROP DATABASE IF EXISTS "+pgx.Identifier{restoredDB}.Sanitize()+" WITH (FORCE)")
	restoreURL := u
	restoreURL.Path = "/" + restoredDB
	restored, err := pgx.Connect(ctx, restoreURL.String())
	if err != nil {
		t.Fatal(err)
	}
	var orders int
	err = restored.QueryRow(ctx, "SELECT count(*) FROM sample_plugin.orders").Scan(&orders)
	restored.Close(ctx)
	if err != nil || orders != 2 {
		t.Fatalf("business data did not survive restore: count=%d err=%v", orders, err)
	}
	t.Setenv("POWERX_OPS_RESTORE_KEEP_DB", "0")
	cleanupDrill, err := drillSvc.Trigger(ctx, TriggerRestoreDrillRequest{SourceJobID: first.ID})
	if err != nil || cleanupDrill.Status != "success" {
		t.Fatalf("verification cleanup failed: %v %+v", err, cleanupDrill)
	}
	var leaked int
	if err = admin.QueryRow(ctx, "SELECT count(*) FROM pg_database WHERE datname LIKE 'powerx_restore_probe_%' AND datname <> $1", restoredDB).Scan(&leaked); err != nil {
		t.Fatal(err)
	}
	// 本次执行生成的目标应已删除（其他测试并行留下的库不作为本次泄漏）。
	var deletedDB string
	for _, field := range strings.Fields(cleanupDrill.ReportURI) {
		if strings.HasPrefix(field, "db=") {
			deletedDB = strings.TrimPrefix(field, "db=")
		}
	}
	var exists bool
	if err = admin.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname=$1)", deletedDB).Scan(&exists); err != nil || exists {
		t.Fatalf("temporary verification database not removed: %v", err)
	}

	second, err := svc.TriggerJob(ctx, TriggerJobRequest{PolicyID: policy.ID})
	if err != nil || second.Status != "success" {
		t.Fatal(err)
	}
	if _, err = os.Stat(firstPath); err != nil {
		t.Fatal("protected file was deleted")
	}
	if err = svc.SetProtected(ctx, first.ID, false, "test", ""); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.cleanupSvc.CleanupByPolicy(ctx, policy.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(firstPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("expired physical file was not deleted")
	}
	for _, suffix := range []string{".json", ".sha256"} {
		if _, e := os.Stat(firstPath + suffix); !errors.Is(e, os.ErrNotExist) {
			t.Fatal("expired sidecar retained")
		}
	}
	existing, err := svc.GetJob(ctx, first.ID)
	if err != nil || existing == nil {
		t.Fatal("job audit was deleted")
	}
	unlock, err := lockBackupPolicy(ctx, db, policy.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.TriggerJob(ctx, TriggerJobRequest{PolicyID: policy.ID}); !errors.Is(err, ErrBackupJobAlreadyRunning) {
		unlock()
		t.Fatal("concurrent backup not blocked")
	}
	unlock()
	// 重启后的调度必须根据持久化时间执行一次；下一次扫描不能重复执行。
	if err = db.Model(policy).Updates(map[string]any{"enabled": true, "created_at": time.Now().Add(-2 * time.Hour)}).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Model(&modelops.BackupJob{}).Where("policy_id = ?", policy.ID).Update("started_at", time.Now().Add(-2*time.Hour)).Error; err != nil {
		t.Fatal(err)
	}
	restarted := NewJobService(db)
	restarted.auditor = nil
	restarted.alertSvc = nil
	restarted.restoreSvc = nil
	if err = restarted.RunDue(ctx); err != nil {
		t.Fatal(err)
	}
	var n int64
	db.Model(&modelops.BackupJob{}).Where("policy_id = ?", policy.ID).Count(&n)
	if n != 3 {
		t.Fatalf("overdue job skipped after restart: %d", n)
	}
	if err = restarted.RunDue(ctx); err != nil {
		t.Fatal(err)
	}
	var after int64
	db.Model(&modelops.BackupJob{}).Where("policy_id = ?", policy.ID).Count(&after)
	if after != n {
		t.Fatal("duplicate scheduled job")
	}
	t.Logf("isolated database=%s: dump, restore, protection, physical cleanup, locks, restart schedule verified", name)
}

func TestRestoreSwitchNamesTransactional(t *testing.T) {
	dsn := os.Getenv("POWERX_BACKUP_TEST_ADMIN_DSN")
	if dsn == "" {
		t.Skip("POWERX_BACKUP_TEST_ADMIN_DSN required")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	prefix := fmt.Sprintf("powerx_backup_switch_%d", time.Now().UnixNano())
	original, candidate, kept, failed := prefix+"_a", prefix+"_b", prefix+"_old", prefix+"_failed"
	quote := func(s string) string { return pgx.Identifier{s}.Sanitize() }
	for _, name := range []string{original, candidate} {
		if _, err = conn.Exec(ctx, "CREATE DATABASE "+quote(name)); err != nil {
			t.Fatal(err)
		}
	}
	defer func() {
		for _, name := range []string{original, candidate, kept, failed} {
			if _, e := conn.Exec(ctx, "DROP DATABASE IF EXISTS "+quote(name)); e != nil {
				t.Error(e)
			}
		}
	}()
	var originalOID, candidateOID uint32
	if err = conn.QueryRow(ctx, "SELECT oid FROM pg_database WHERE datname=$1", original).Scan(&originalOID); err != nil {
		t.Fatal(err)
	}
	if err = conn.QueryRow(ctx, "SELECT oid FROM pg_database WHERE datname=$1", candidate).Scan(&candidateOID); err != nil {
		t.Fatal(err)
	}
	if _, err = conn.Exec(ctx, "BEGIN; ALTER DATABASE "+quote(original)+" RENAME TO "+quote(kept)+"; ALTER DATABASE "+quote(candidate)+" RENAME TO "+quote(original)+"; COMMIT;"); err != nil {
		t.Fatal(err)
	}
	var current uint32
	if err = conn.QueryRow(ctx, "SELECT oid FROM pg_database WHERE datname=$1", original).Scan(&current); err != nil || current != candidateOID {
		t.Fatalf("candidate not switched: %v", err)
	}
	if _, err = conn.Exec(ctx, "BEGIN; ALTER DATABASE "+quote(original)+" RENAME TO "+quote(failed)+"; ALTER DATABASE "+quote(kept)+" RENAME TO "+quote(original)+"; COMMIT;"); err != nil {
		t.Fatal(err)
	}
	if err = conn.QueryRow(ctx, "SELECT oid FROM pg_database WHERE datname=$1", original).Scan(&current); err != nil || current != originalOID {
		t.Fatalf("original not preserved for rollback: %v", err)
	}
	t.Log("transactional rename and rollback verified only on new isolated test databases")
}
