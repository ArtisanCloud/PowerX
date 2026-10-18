package snapshot_ops

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

func TestNativePostgresPortableRestoreAWithSourceAtB(t *testing.T) {
	for _, scheduled := range []bool{false, true} {
		name := "manual"
		if scheduled {
			name = "scheduled"
		}
		t.Run(name, func(t *testing.T) { nativePortableOnlineRestore(t, scheduled) })
	}
}
func nativePortableOnlineRestore(t *testing.T, scheduled bool) {
	dsn := os.Getenv("POWERX_SNAPSHOT_TEST_DSN")
	if dsn == "" {
		t.Skip("explicit opt-in native PostgreSQL")
	}
	ctx := context.Background()
	admin, e := pgx.Connect(ctx, dsn)
	require.NoError(t, e)
	defer admin.Close(ctx)
	sourceName := "px_snapshot_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	copyName := sourceName + "_copy"
	_, e = admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{sourceName}.Sanitize())
	require.NoError(t, e)
	defer func() {
		_, e := admin.Exec(ctx, "DROP DATABASE "+pgx.Identifier{sourceName}.Sanitize())
		require.NoError(t, e)
	}()
	sourceCfg, e := pgx.ParseConfig(dsn)
	require.NoError(t, e)
	sourceCfg.Database = sourceName
	sourceURL, e := url.Parse(dsn)
	require.NoError(t, e)
	sourceURL.Path = "/" + sourceName
	sourceDSN := sourceURL.String()
	source, e := pgx.ConnectConfig(ctx, sourceCfg)
	require.NoError(t, e)
	defer source.Close(ctx)
	_, e = source.Exec(ctx, `CREATE TABLE public.media_assets(id int PRIMARY KEY,driver text,bucket text,storage_key text,size_bytes bigint,expected_checksum text,upload_state text,deleted_at timestamptz);CREATE TABLE public.orders(id int PRIMARY KEY, label text);INSERT INTO public.orders VALUES(1,'state A');CREATE SCHEMA plugin_aicraft;CREATE TABLE plugin_aicraft.tracks(id int PRIMARY KEY, label text);INSERT INTO plugin_aicraft.tracks VALUES(1,'track A')`)
	require.NoError(t, e)
	media := t.TempDir()
	a := []byte("original image A")
	sum := sha256.Sum256(a)
	require.NoError(t, os.WriteFile(filepath.Join(media, "a.jpg"), a, 0600))
	_, e = source.Exec(ctx, "INSERT INTO public.media_assets VALUES(1,'local','','a.jpg',$1,$2,'ready',NULL)", len(a), hex.EncodeToString(sum[:]))
	require.NoError(t, e)
	seed, _, _ := fixtureService(t)
	cfg := seed.cfg
	cfg.Directory = t.TempDir()
	cfg.MediaRoot = media
	cfg.DatabaseDSN = sourceDSN
	cfg.EffectiveConfig = []byte("deployment: test\nprivate: snapshot-test-only\n")
	backend := &PostgresBackend{DSN: sourceDSN, MediaRoot: media}
	service := NewService(ctx, cfg, Options{Backend: backend})
	defer service.Close()
	// 在 pg_dump 启动前暂停工具：此时 MVCC 已建立、媒体已读取，业务仍必须可提交。
	realDump, e := exec.LookPath("pg_dump")
	require.NoError(t, e)
	barrier := t.TempDir()
	shim := filepath.Join(t.TempDir(), "pg_dump")
	script := `#!/bin/sh
: > "$PX_SNAPSHOT_BARRIER/entered"
while [ ! -f "$PX_SNAPSHOT_BARRIER/resume" ]; do sleep 0.02; done
exec '` + strings.ReplaceAll(realDump, "'", "'\\''") + `' "$@"
`
	require.NoError(t, os.WriteFile(shim, []byte(script), 0700))
	t.Setenv("PATH", filepath.Dir(shim)+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("PX_SNAPSHOT_BARRIER", barrier)
	defer os.WriteFile(filepath.Join(barrier, "resume"), nil, 0600)
	require.NoError(t, os.WriteFile(filepath.Join(media, "in-progress-upload.tmp"), []byte("not registered"), 0600))
	var j Snapshot
	if scheduled {
		_, e = service.SavePolicy(rootContext(), Policy{Enabled: true, IntervalMinutes: 1, RetentionDays: 1, RetentionCount: 3})
		require.NoError(t, e)
		state, err := service.policyState()
		require.NoError(t, err)
		for i := range state.Policies {
			state.Policies[i].Anchor = time.Now().Add(-2 * time.Minute)
		}
		require.NoError(t, atomicJSON(filepath.Join(cfg.Directory, "policies.json"), state))
		require.NoError(t, service.Tick(ctx))
		overview, err := service.Overview(rootContext())
		require.NoError(t, err)
		require.Len(t, overview.Snapshots, 1)
		j = overview.Snapshots[0]
		require.Equal(t, "scheduled", j.Trigger)
	} else {
		j, e = service.Start(rootContext(), uuid.NewString())
		require.NoError(t, e)
	}
	require.Eventually(t, func() bool { _, err := os.Stat(filepath.Join(barrier, "entered")); return err == nil }, 15*time.Second, 10*time.Millisecond)
	writer, cancelWriter := context.WithTimeout(ctx, 2*time.Second)
	_, e = source.Exec(writer, "INSERT INTO public.orders VALUES(2,'tracking order submitted during capture');INSERT INTO plugin_aicraft.tracks VALUES(2,'track created during backup')")
	cancelWriter()
	writeErr := e
	// 已读取的媒体在 dump 完成前被覆盖，包仍必须保留恢复点 A 的内容与引用。
	require.NoError(t, os.WriteFile(filepath.Join(media, "a.jpg"), []byte("changed B"), 0600))
	bSum := sha256.Sum256([]byte("changed B"))
	_, e = source.Exec(ctx, "UPDATE public.media_assets SET size_bytes=$1,expected_checksum=$2 WHERE id=1", len("changed B"), hex.EncodeToString(bSum[:]))
	// 总是先释放工具，避免断言失败时服务 Close 等待。
	require.NoError(t, os.WriteFile(filepath.Join(barrier, "resume"), nil, 0600))
	require.NoError(t, writeErr, "tracking order must commit while backup is in progress")
	require.NoError(t, e, "online capture must allow DML to commit before pg_dump completes")
	t.Log("Tracking order INSERT and media metadata UPDATE committed while pg_dump was still waiting; media file replaced during capture")
	require.Eventually(t, func() bool {
		current, err := service.Get(rootContext(), j.ID)
		if err == nil {
			j = current
		}
		return err == nil && (j.Status == "ready" || j.Status == "failed")
	}, 30*time.Second, 10*time.Millisecond)
	require.Equal(t, "ready", j.Status, j.Error)
	waitIdle(t, service)
	pkg := packageBytes(t, service, j.ID)
	_, e = source.Exec(ctx, "UPDATE public.orders SET label='state B';INSERT INTO public.orders VALUES(3,'after snapshot')")
	require.NoError(t, e)
	require.NoError(t, os.WriteFile(filepath.Join(media, "a.jpg"), []byte("changed B"), 0600))
	_, e = source.Exec(ctx, "UPDATE public.media_assets SET size_bytes=$1,expected_checksum=$2 WHERE id=1", len("changed B"), hex.EncodeToString(bSum[:]))
	require.NoError(t, e)
	// 目标服务没有源 DSN、媒体路径、配置或程序来源，只持有待导入包和目标版本。
	targetCfg := Config{Directory: t.TempDir(), Instance: "isolated-target", Version: cfg.Version}
	created := false
	targetBackend := &PostgresBackend{Restore: func(ctx context.Context, file string) (string, string, error) {
		if _, e := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{copyName}.Sanitize()); e != nil {
			return "", "", e
		}
		created = true
		c, e := pgx.ParseConfig(dsn)
		if e != nil {
			return "", "", e
		}
		cmd := exec.CommandContext(ctx, "pg_restore", "--exit-on-error", "--no-owner", "--no-privileges", "--host", c.Host, "--port", fmt.Sprint(c.Port), "--username", c.User, "--dbname", copyName, file)
		cmd.Env = append(os.Environ(), "PGPASSWORD="+c.Password, "PGSSLMODE=disable")
		if e = cmd.Run(); e != nil {
			return "", "", fmt.Errorf("owned fixture pg_restore failed: %w", e)
		}
		return uuid.NewString(), copyName, nil
	}}
	defer func() {
		if created {
			_, e := admin.Exec(ctx, "DROP DATABASE "+pgx.Identifier{copyName}.Sanitize())
			require.NoError(t, e)
		}
	}()
	target := NewService(ctx, targetCfg, Options{Backend: targetBackend})
	defer target.Close()
	_, e = target.Import(rootContext(), bytes.NewReader(pkg), "independent-test-passphrase")
	require.NoError(t, e)
	service.Close()
	require.NoError(t, os.RemoveAll(service.cfg.Directory))
	offlineMedia := filepath.Join(t.TempDir(), "offline-source-media")
	require.NoError(t, os.Rename(media, offlineMedia))
	r, e := target.StartRestore(rootContext(), j.ID, uuid.NewString())
	require.NoError(t, e)
	require.Eventually(t, func() bool {
		got, e := target.GetRestore(rootContext(), r.ID)
		if e == nil {
			r = got
		}
		return e == nil && (r.Status == "prepared" || r.Status == "failed")
	}, 30*time.Second, 20*time.Millisecond)
	require.Equal(t, "prepared", r.Status, r.Error)
	waitIdle(t, target)
	copyCfg, e := pgx.ParseConfig(dsn)
	require.NoError(t, e)
	copyCfg.Database = copyName
	restored, e := pgx.ConnectConfig(ctx, copyCfg)
	require.NoError(t, e)
	defer restored.Close(ctx)
	var value string
	var count int
	require.NoError(t, restored.QueryRow(ctx, "SELECT label FROM public.orders WHERE id=1").Scan(&value))
	require.Equal(t, "state A", value)
	sourceMedia, err := os.ReadFile(filepath.Join(offlineMedia, "a.jpg"))
	require.NoError(t, err)
	require.Equal(t, "changed B", string(sourceMedia))
	require.NoError(t, restored.QueryRow(ctx, "SELECT count(*) FROM public.orders").Scan(&count))
	require.Equal(t, 1, count)
	require.NoError(t, restored.QueryRow(ctx, "SELECT count(*) FROM plugin_aicraft.tracks").Scan(&count))
	require.Equal(t, 1, count, "plugin schema restored at A")
	require.NoError(t, source.QueryRow(ctx, "SELECT count(*) FROM plugin_aicraft.tracks").Scan(&count))
	require.Equal(t, 2, count, "new plugin track remains in live source")
	raw, e := os.ReadFile(filepath.Join(r.Directory, "media", "a.jpg"))
	require.NoError(t, e)
	require.Equal(t, a, raw)
	_, err = os.Stat(filepath.Join(r.Directory, "media", "in-progress-upload.tmp"))
	require.True(t, os.IsNotExist(err), "unregistered uploads do not belong to the recovery point")
	var mediaHash string
	require.NoError(t, restored.QueryRow(ctx, "SELECT expected_checksum FROM public.media_assets WHERE id=1").Scan(&mediaHash))
	require.Equal(t, hex.EncodeToString(sum[:]), mediaHash)
	require.NoError(t, source.QueryRow(ctx, "SELECT expected_checksum FROM public.media_assets WHERE id=1").Scan(&mediaHash))
	require.Equal(t, hex.EncodeToString(bSum[:]), mediaHash)
	require.NoError(t, source.QueryRow(ctx, "SELECT label FROM public.orders WHERE id=1").Scan(&value))
	require.Equal(t, "state B", value)
	require.NoError(t, source.QueryRow(ctx, "SELECT count(*) FROM public.orders").Scan(&count))
	require.Equal(t, 3, count)
	t.Log("native PostgreSQL A/B restore passed; media restored without source files or source snapshot key; source B preserved; only owned fixture databases used")
}
