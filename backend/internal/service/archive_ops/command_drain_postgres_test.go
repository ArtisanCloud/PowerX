package archive_ops

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

func TestCommandDrainRealPostgresDumpBlockedThenServiceClose(t *testing.T) {
	dsn := os.Getenv("POWERX_ARCHIVE_TEST_DSN")
	if dsn == "" {
		t.Skip("owned native PostgreSQL only")
	}
	ctx := context.Background()
	admin, e := pgx.Connect(ctx, dsn)
	require.NoError(t, e)
	defer admin.Close(ctx)
	name := "powerx_command_drain_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	_, e = admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize())
	require.NoError(t, e)
	defer func() {
		_, e := admin.Exec(ctx, "DROP DATABASE "+pgx.Identifier{name}.Sanitize())
		require.NoError(t, e)
	}()
	cfg, e := pgx.ParseConfig(dsn)
	require.NoError(t, e)
	cfg.Database = name
	db, e := pgx.ConnectConfig(ctx, cfg)
	require.NoError(t, e)
	defer db.Close(ctx)
	_, e = db.Exec(ctx, `CREATE TABLE media_assets(id int PRIMARY KEY,driver text,bucket text,storage_key text,size_bytes bigint,expected_checksum text,upload_state text,deleted_at timestamptz);CREATE TABLE sentinel(id int PRIMARY KEY,value text);INSERT INTO sentinel VALUES(1,'untouched')`)
	require.NoError(t, e)
	dir := t.TempDir()
	content := []byte("owned media retained during pg_dump cancellation")
	sum := sha256.Sum256(content)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.jpg"), content, 0600))
	_, e = db.Exec(ctx, "INSERT INTO media_assets VALUES(1,'local','','a.jpg',$1,$2,'ready',NULL)", len(content), hex.EncodeToString(sum[:]))
	require.NoError(t, e)
	tx, e := db.Begin(ctx)
	require.NoError(t, e)
	defer tx.Rollback(ctx)
	_, e = tx.Exec(ctx, "LOCK TABLE sentinel IN ACCESS EXCLUSIVE MODE")
	require.NoError(t, e)
	u, e := url.Parse(dsn)
	require.NoError(t, e)
	u.Path = "/" + name
	sourceDSN := u.String()
	archiveConfig := Config{Directory: t.TempDir(), Instance: "owned-command-drain", Sources: []Source{{ID: "media", Kind: "media", Directory: dir, DatabaseDSN: sourceDSN}}}
	s := NewService(ctx, archiveConfig, Options{})
	defer s.Close()
	p, e := s.SavePolicy(archiveRoot(), Policy{Name: "owned snapshot", SourceID: "media", IntervalMinutes: 60, RetentionDays: 7, RetentionCount: 3, RemoteRetentionDays: 90, RemoteRetentionCount: 90, IncludeDatabase: true})
	require.NoError(t, e)
	job, e := s.Start(archiveRoot(), p.ID, uuid.NewString(), "owned", "")
	require.NoError(t, e)
	var proof commandRecord
	require.Eventually(t, func() bool {
		proof, e = s.readCommandRecord(job.ID)
		if e != nil || proof.Kind != "pg_dump" || proof.State != "running" {
			return false
		}
		var blocked int
		e = admin.QueryRow(ctx, "SELECT count(*) FROM pg_stat_activity WHERE datname=$1 AND application_name='pg_dump' AND wait_event_type='Lock'", name).Scan(&blocked)
		return e == nil && blocked > 0
	}, 10*time.Second, 20*time.Millisecond)
	t.Logf("owned native pg_dump pid=%d pgid=%d db=%s command=pg_dump --no-password --format=custom --snapshot=<owned> cwd=%s", proof.PID, proof.PGID, name, archiveConfig.Directory)
	objects := restartObjects(t, s.cfg.Directory)
	require.NotEmpty(t, objects)
	s.Close()
	assertChildGone(t, commandChildProof{PID: proof.PID, PGID: proof.PGID})
	var active int
	require.NoError(t, admin.QueryRow(ctx, "SELECT count(*) FROM pg_stat_activity WHERE datname=$1 AND application_name='pg_dump'", name).Scan(&active))
	require.Zero(t, active)
	original, e := s.readJob(job.ID)
	require.NoError(t, e)
	require.Equal(t, "interrupted", original.Status)
	require.True(t, original.InterruptionPending)
	current, e := s.readCommandRecord(job.ID)
	require.NoError(t, e)
	require.Equal(t, "drained", current.State)
	require.Equal(t, objects, restartObjects(t, s.cfg.Directory))
	raw, e := os.ReadFile(filepath.Join(dir, "a.jpg"))
	require.NoError(t, e)
	require.Equal(t, content, raw)
	_, e = os.Stat(filepath.Join(s.cfg.Directory, "staging", job.ID+".dump"))
	require.True(t, os.IsNotExist(e))
	peer := NewService(ctx, archiveConfig, Options{})
	defer peer.Close()
	out, e := peer.Job(archiveRoot(), job.ID)
	require.NoError(t, e)
	require.Equal(t, "drained", out.CommandDrainState)
	require.True(t, out.InterruptionPending)
	jobs, e := peer.records("jobs")
	require.NoError(t, e)
	require.Equal(t, []string{job.ID}, jobs)
	require.NoError(t, tx.Rollback(ctx))
	var sentinel string
	require.NoError(t, db.QueryRow(ctx, "SELECT value FROM sentinel WHERE id=1").Scan(&sentinel))
	require.Equal(t, "untouched", sentinel)
}
