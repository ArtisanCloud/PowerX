package bootstrap

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	snapshotops "github.com/ArtisanCloud/PowerX/internal/service/snapshot_ops"
	auditmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// 走 Core 的真实装配入口，不能仅用绝对路径直接构造 PostgresBackend，
// 否则会漏掉“预检查已解析、采集器仍拿原始相对路径”的集成错误。
func TestNativeSystemSnapshotBootstrapWithRelativeMediaDirectory(t *testing.T) {
	dsn := os.Getenv("POWERX_SNAPSHOT_TEST_DSN")
	if dsn == "" {
		t.Skip("explicit opt-in native PostgreSQL")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, dsn)
	require.NoError(t, err)
	defer admin.Close(ctx)
	database := "px_bootstrap_snapshot_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	copyDatabase := database + "_copy"
	_, err = admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{database}.Sanitize())
	require.NoError(t, err)
	defer func() {
		_, err := admin.Exec(ctx, "DROP DATABASE "+pgx.Identifier{copyDatabase}.Sanitize()+" WITH (FORCE)")
		if err != nil && !strings.Contains(err.Error(), "does not exist") {
			t.Error(err)
		}
		_, err = admin.Exec(ctx, "DROP DATABASE "+pgx.Identifier{database}.Sanitize()+" WITH (FORCE)")
		require.NoError(t, err)
	}()
	u, err := url.Parse(dsn)
	require.NoError(t, err)
	u.Path = "/" + database
	db, err := gorm.Open(postgres.Open(u.String()), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	defer sqlDB.Close()
	require.NoError(t, db.AutoMigrate(&auditmodel.AuditEvent{}))
	require.NoError(t, db.Exec(`CREATE TABLE public.media_assets(id int PRIMARY KEY,driver text,bucket text,storage_key text,size_bytes bigint,expected_checksum text,upload_state text,deleted_at timestamptz);
CREATE TABLE public.customers(id int PRIMARY KEY,name text); INSERT INTO public.customers VALUES(1,'customer A');
CREATE SCHEMA plugin_aicraft; CREATE TABLE plugin_aicraft.tracks(id int PRIMARY KEY,label text); INSERT INTO plugin_aicraft.tracks VALUES(1,'track A')`).Error)
	root := t.TempDir()
	t.Chdir(root)
	t.Setenv("POWERX_LINKS_ROOT", root)
	catalog := filepath.Join(root, "backend/config/platform_capabilities")
	require.NoError(t, os.MkdirAll(catalog, 0700))
	t.Setenv("PLATFORM_CAPABILITIES_DIR", catalog)
	t.Setenv("POWERX_WEB_OUTPUT", "")
	media := filepath.Join(root, "storage", "media")
	require.NoError(t, os.MkdirAll(media, 0700))
	original := []byte("bootstrap media A")
	sum := sha256.Sum256(original)
	require.NoError(t, os.WriteFile(filepath.Join(media, "photo.jpg"), original, 0600))
	require.NoError(t, db.Exec("INSERT INTO public.media_assets VALUES(1,'local','','photo.jpg',?,?, 'ready',NULL)", len(original), hex.EncodeToString(sum[:])).Error)
	web := filepath.Join(root, "web-admin/.output/server/index.mjs")
	require.NoError(t, os.MkdirAll(filepath.Dir(web), 0700))
	require.NoError(t, os.WriteFile(web, []byte("// native fixture web output"), 0600))
	plugin := filepath.Join(root, "plugins/installed/ai-craft/fixture/plugin.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(plugin), 0700))
	require.NoError(t, os.WriteFile(plugin, []byte("id: ai-craft\nversion: fixture\n"), 0600))
	cfg := config.GetDefaults()
	cfg.Version = "bootstrap-native-test"
	cfg.Database.Driver = "postgres"
	cfg.Database.DSN = u.String()
	cfg.Storage.DefaultDriver = "local"
	cfg.Storage.Local.BasePath = "./storage/media"
	cfg.Plugin.InstalledDir = "./plugins/installed"
	cfg.Plugin.RegistryFile = ""
	deps := &shared.Deps{DB: db}
	configureSystemSnapshots(ctx, cfg, deps)
	service := deps.SystemSnapshots
	defer service.Close()
	actor := reqctx.WithIsRoot(reqctx.WithClaims(ctx, &reqctx.CoreXClaims{IsRoot: true, UserID: 1}), true)
	overview, err := service.Overview(actor)
	require.NoError(t, err)
	require.True(t, overview.Ready, overview.Problems)
	policy, err := service.PutPolicy(actor, "", "relative media", snapshotops.Policy{IntervalMinutes: 1, RetentionDays: 7, RetentionCount: 7})
	require.NoError(t, err)
	manual, err := service.StartForPolicy(actor, uuid.NewString(), policy.ID)
	require.NoError(t, err)
	waitSnapshot := func(id string) snapshotops.Snapshot {
		var row snapshotops.Snapshot
		require.Eventually(t, func() bool {
			row, err = service.Get(actor, id)
			return err == nil && (row.Status == "ready" || row.Status == "failed")
		}, 45*time.Second, 20*time.Millisecond)
		require.Equal(t, "ready", row.Status, row.Error)
		return row
	}
	manual = waitSnapshot(manual.ID)
	require.Equal(t, "manual", manual.Trigger)
	require.Equal(t, policy.ID, manual.PolicyID)
	require.Len(t, manual.Components, 4)
	require.False(t, manual.RecoverySupported, "在线包不宣称具备同一时点 Redis 队列")
	require.Equal(t, "./storage/media", cfg.Storage.Local.BasePath, "不能改写运行中业务的媒体配置")

	// 将测试策略的调度锚点推进到过去，走正式 Tick，避免等待一分钟。
	_, err = service.SetPolicyEnabled(actor, policy.ID, true)
	require.NoError(t, err)
	policyFile := filepath.Join(overview.Directory, "policies.json")
	raw, err := os.ReadFile(policyFile)
	require.NoError(t, err)
	var state map[string]any
	require.NoError(t, json.Unmarshal(raw, &state))
	state["policies"].([]any)[0].(map[string]any)["schedule_anchor"] = time.Now().Add(-2 * time.Minute).UTC().Format(time.RFC3339Nano)
	raw, err = json.Marshal(state)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(policyFile, raw, 0600))
	// 下一次执行还受上一次快照时间约束，仅在测试账本中回拨该时间。
	manualFile := filepath.Join(overview.Directory, "snapshots", manual.ID+".json")
	raw, err = os.ReadFile(manualFile)
	require.NoError(t, err)
	var record map[string]any
	require.NoError(t, json.Unmarshal(raw, &record))
	record["created_at"] = time.Now().Add(-2 * time.Minute).UTC().Format(time.RFC3339Nano)
	raw, err = json.Marshal(record)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(manualFile, raw, 0600))
	require.NoError(t, service.Tick(ctx))
	overview, err = service.Overview(actor)
	require.NoError(t, err)
	require.Len(t, overview.Snapshots, 2)
	scheduled := waitSnapshot(overview.Snapshots[0].ID)
	require.Equal(t, "scheduled", scheduled.Trigger)
	_, err = service.SetPolicyEnabled(actor, policy.ID, false)
	require.NoError(t, err)

	// 使用导出包恢复到另一数据库，证明采集成功不只代表任务状态变绿。
	file, cleanup, err := service.ExportFile(actor, manual.ID, "bootstrap-native-passphrase")
	require.NoError(t, err)
	defer cleanup()
	require.NoError(t, db.Exec("UPDATE public.customers SET name='customer B'; INSERT INTO plugin_aicraft.tracks VALUES(2,'after snapshot')").Error)
	require.NoError(t, os.WriteFile(filepath.Join(media, "photo.jpg"), []byte("media B"), 0600))
	target := snapshotops.NewService(ctx, snapshotops.Config{Directory: t.TempDir(), Instance: "isolated", Version: cfg.Version}, snapshotops.Options{Backend: &snapshotops.PostgresBackend{Restore: func(run context.Context, dump string) (string, string, error) {
		if _, err := admin.Exec(run, "CREATE DATABASE "+pgx.Identifier{copyDatabase}.Sanitize()); err != nil {
			return "", "", err
		}
		conn, err := pgx.ParseConfig(dsn)
		if err != nil {
			return "", "", err
		}
		cmd := exec.CommandContext(run, "pg_restore", "--exit-on-error", "--no-owner", "--no-privileges", "--host", conn.Host, "--port", fmt.Sprint(conn.Port), "--username", conn.User, "--dbname", copyDatabase, dump)
		cmd.Env = append(os.Environ(), "PGPASSWORD="+conn.Password, "PGSSLMODE=disable")
		if err := cmd.Run(); err != nil {
			return "", "", fmt.Errorf("owned native restore failed: %w", err)
		}
		return uuid.NewString(), copyDatabase, nil
	}}})
	defer target.Close()
	input, err := os.Open(file)
	require.NoError(t, err)
	_, err = target.Import(actor, input, "bootstrap-native-passphrase")
	input.Close()
	require.NoError(t, err)
	restore, err := target.StartRestore(actor, manual.ID, uuid.NewString())
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		restore, err = target.GetRestore(actor, restore.ID)
		return err == nil && (restore.Status == "prepared" || restore.Status == "failed")
	}, 45*time.Second, 20*time.Millisecond)
	require.Equal(t, "prepared", restore.Status, restore.Error)
	u.Path = "/" + copyDatabase
	copy, err := pgx.Connect(ctx, u.String())
	require.NoError(t, err)
	defer copy.Close(ctx)
	var customer string
	var tracks int
	require.NoError(t, copy.QueryRow(ctx, "SELECT name FROM public.customers WHERE id=1").Scan(&customer))
	require.Equal(t, "customer A", customer)
	require.NoError(t, copy.QueryRow(ctx, "SELECT count(*) FROM plugin_aicraft.tracks").Scan(&tracks))
	require.Equal(t, 1, tracks)
	restored, err := os.ReadFile(filepath.Join(restore.Directory, "media", "photo.jpg"))
	require.NoError(t, err)
	require.Equal(t, original, restored)
	restoredPlugin, err := os.ReadFile(filepath.Join(restore.Directory, "programs/plugins/ai-craft/fixture/plugin.yaml"))
	require.NoError(t, err)
	require.Equal(t, "id: ai-craft\nversion: fixture\n", string(restoredPlugin))
	_, err = os.Stat(filepath.Join(restore.Directory, "programs/core/web-admin/server/index.mjs"))
	require.NoError(t, err)
	require.NoError(t, db.Raw("SELECT name FROM public.customers WHERE id=1").Scan(&customer).Error)
	require.Equal(t, "customer B", customer)
	require.NoError(t, db.Raw("SELECT count(*) FROM plugin_aicraft.tracks").Scan(&tracks).Error)
	require.Equal(t, 2, tracks)
	live, err := os.ReadFile(filepath.Join(media, "photo.jpg"))
	require.NoError(t, err)
	require.Equal(t, "media B", string(live))
	t.Log("Real Core bootstrap with ./storage/media: manual + scheduled capture ready; exported/imported package restores customer, plugin tracks and media A to an isolated database; source B unchanged")
}
