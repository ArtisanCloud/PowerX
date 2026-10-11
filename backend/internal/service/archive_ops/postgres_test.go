package archive_ops

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMediaDatabaseSnapshotNativePostgres(t *testing.T) {
	dsn := os.Getenv("POWERX_ARCHIVE_TEST_DSN")
	if dsn == "" {
		t.Skip("opt-in native PostgreSQL")
	}
	ctx := context.Background()
	admin, e := pgx.Connect(ctx, dsn)
	require.NoError(t, e)
	defer admin.Close(ctx)
	name := "powerx_archive_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	_, e = admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize())
	require.NoError(t, e)
	defer admin.Exec(ctx, "DROP DATABASE "+pgx.Identifier{name}.Sanitize())
	u, e := url.Parse(dsn)
	require.NoError(t, e)
	u.Path = "/" + name
	source, e := pgx.Connect(ctx, u.String())
	require.NoError(t, e)
	defer source.Close(ctx)
	_, e = source.Exec(ctx, `CREATE TABLE media_assets(id int PRIMARY KEY, driver text,bucket text,storage_key text,size_bytes bigint,expected_checksum text,upload_state text,deleted_at timestamptz);CREATE TABLE sentinel(id int PRIMARY KEY,value text);INSERT INTO sentinel VALUES(1,'unchanged')`)
	require.NoError(t, e)
	dir := t.TempDir()
	content := []byte("media snapshot content")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.jpg"), content, 0600))
	sum := sha256.Sum256(content)
	_, e = source.Exec(ctx, "INSERT INTO media_assets VALUES(1,'local','','a.jpg',$1,$2,'ready',NULL)", len(content), hex.EncodeToString(sum[:]))
	require.NoError(t, e)
	s := NewService(ctx, Config{Directory: t.TempDir(), Instance: "native", Sources: []Source{{ID: "media", Kind: "media", Directory: dir, DatabaseDSN: u.String()}}}, Options{})
	defer s.Close()
	p, e := s.SavePolicy(archiveRoot(), Policy{Name: "paired", SourceID: "media", IntervalMinutes: 60, RetentionDays: 7, RetentionCount: 2, RemoteRetentionDays: 90, RemoteRetentionCount: 90, IncludeDatabase: true})
	require.NoError(t, e)
	j := captureTest(t, s, p)
	require.Equal(t, "success", j.Status, j.Error)
	require.Equal(t, name, j.Database)
	require.NotEmpty(t, j.SnapshotID)
	require.Equal(t, 2, j.FileCount)
	restoreName := name + "_restore"
	_, e = admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{restoreName}.Sanitize())
	require.NoError(t, e)
	defer admin.Exec(ctx, "DROP DATABASE "+pgx.Identifier{restoreName}.Sanitize())
	cfg, e := pgx.ParseConfig(u.String())
	require.NoError(t, e)
	var dump Entry
	for _, f := range j.Files {
		if f.Path == "database/database.dump" {
			dump = f
		}
	}
	require.NotEmpty(t, dump.SHA256)
	command := exec.Command("pg_restore", "--no-owner", "--no-privileges", "-h", cfg.Host, "-p", urlPort(cfg.Port), "-U", cfg.User, "-d", restoreName, s.objectPath(dump.SHA256))
	command.Env = append(os.Environ(), "PGPASSWORD="+cfg.Password, "PGSSLMODE=disable")
	require.NoError(t, command.Run())
	copyCfg := cfg.Copy()
	copyCfg.Database = restoreName
	copy, e := pgx.ConnectConfig(ctx, copyCfg)
	require.NoError(t, e)
	defer copy.Close(ctx)
	var value string
	require.NoError(t, copy.QueryRow(ctx, "SELECT value FROM sentinel WHERE id=1").Scan(&value))
	require.Equal(t, "unchanged", value)
	copy.Close(ctx)
	var count int
	require.NoError(t, source.QueryRow(ctx, "SELECT count(*) FROM sentinel").Scan(&count))
	require.Equal(t, 1, count)
	// 元数据和文件不一致必须失败，不能给损坏媒体快照报成功。
	_, e = source.Exec(ctx, "UPDATE media_assets SET expected_checksum=$1", strings.Repeat("0", 64))
	require.NoError(t, e)
	failed := captureTest(t, s, p)
	require.Equal(t, "failed", failed.Status)
	require.Contains(t, failed.Error, "SHA256")
	source.Close(ctx)
	t.Logf("native paired dump restored to owned database; source sentinel preserved; fixtures cleaned source=%s tables=2", name)
}
func urlPort(value uint16) string { return fmt.Sprint(value) }
