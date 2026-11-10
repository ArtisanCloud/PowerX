package storagegov

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	recovery "github.com/ArtisanCloud/PowerX/internal/infra/storage_recovery"
	archive "github.com/ArtisanCloud/PowerX/internal/service/archive_ops"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestArchiveCommandConfirmationNativePublishedPauseAndDatabaseFailure(t *testing.T) {
	svc, ctrl, _, _ := monitoringNative(t)
	ctx := controlRoot()
	dsn := os.Getenv("POWERX_ARCHIVE_TEST_DSN")
	if dsn == "" {
		t.Skip("owned archive PostgreSQL required")
	}
	db, e := pgx.Connect(ctx, dsn)
	require.NoError(t, e)
	defer db.Close(ctx)
	name := "powerx_command_confirm_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	_, e = db.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize())
	require.NoError(t, e)
	defer func() { _, e := db.Exec(ctx, "DROP DATABASE "+pgx.Identifier{name}.Sanitize()); require.NoError(t, e) }()
	u, e := url.Parse(dsn)
	require.NoError(t, e)
	u.Path = "/" + name
	sourceDB, e := pgx.Connect(ctx, u.String())
	require.NoError(t, e)
	defer sourceDB.Close(ctx)
	_, e = sourceDB.Exec(ctx, `CREATE TABLE media_assets(id int PRIMARY KEY,driver text,bucket text,storage_key text,size_bytes bigint,expected_checksum text,upload_state text,deleted_at timestamptz)`)
	require.NoError(t, e)
	source, dir := t.TempDir(), t.TempDir()
	content := []byte("owned formal control media")
	sum := sha256.Sum256(content)
	require.NoError(t, os.WriteFile(filepath.Join(source, "owned.jpg"), content, 0600))
	_, e = sourceDB.Exec(ctx, "INSERT INTO media_assets VALUES(1,'local','','owned.jpg',$1,$2,'ready',NULL)", len(content), hex.EncodeToString(sum[:]))
	require.NoError(t, e)
	cfg := archive.Config{Directory: dir, Instance: "owned-command-confirmation", Sources: []archive.Source{{ID: "media", Kind: "media", Directory: source, DatabaseDSN: u.String()}}}
	seed := archive.NewService(context.Background(), cfg, archive.Options{})
	p, e := seed.SavePolicy(ctx, archive.Policy{Name: "owned paired snapshot", SourceID: "media", IntervalMinutes: 60, RetentionDays: 7, RetentionCount: 3, RemoteRetentionDays: 90, RemoteRetentionCount: 90, IncludeDatabase: true})
	require.NoError(t, e)
	job, e := seed.Start(ctx, p.ID, uuid.NewString(), "owned", "")
	require.NoError(t, e)
	require.Eventually(t, func() bool {
		j, e := seed.Job(ctx, job.ID)
		return e == nil && j.Status == "success" && j.ExecutionState != "active"
	}, 10*time.Second, 20*time.Millisecond)
	seed.Close()
	// 已真实执行并退出的 pg_dump，受控回放为 final ack 丢失的 running 记录；不是现场进程崩溃。
	commandPath := filepath.Join(dir, "command-executions", job.ID+".json")
	raw, e := os.ReadFile(commandPath)
	require.NoError(t, e)
	var command map[string]any
	require.NoError(t, json.Unmarshal(raw, &command))
	require.Equal(t, "pg_dump", command["kind"])
	require.NotNil(t, command["identity"])
	command["state"] = "running"
	raw, e = json.Marshal(command)
	require.NoError(t, e)
	require.NoError(t, os.WriteFile(commandPath, raw, 0600))
	path := filepath.Join(dir, "jobs", job.ID+".json")
	raw, e = os.ReadFile(path)
	require.NoError(t, e)
	var record map[string]any
	require.NoError(t, json.Unmarshal(raw, &record))
	record["status"] = "running"
	record["files"] = nil
	raw, e = json.Marshal(record)
	require.NoError(t, e)
	require.NoError(t, os.WriteFile(path, raw, 0600))
	a := archive.NewService(context.Background(), cfg, archive.Options{})
	defer a.Close()
	anchorDir := filepath.Join(t.TempDir(), "maintenance")
	require.NoError(t, os.Mkdir(anchorDir, 0700))
	anchor, e := recovery.Initialize(anchorDir, uuid.NewString())
	require.NoError(t, e)
	require.NoError(t, a.BindRecoveryAnchor(anchor))
	require.NoError(t, svc.BindArchiveControl(a))
	policies := &archivePolicyControl{svc, a}
	adoptNativePolicy(t, policies, ctrl)
	review, e := a.ReviewCommandDrain(ctx, job.ID)
	require.NoError(t, e)
	require.True(t, review.Eligible)
	input := archive.ConfirmCommandDrainInput{RequestID: uuid.NewString(), ExpectedVersion: review.ReviewVersion, ExecutionID: review.ExecutionID, CommandID: review.CommandID, Action: "confirm_local_command_drain", Reason: "正式控制链验证后记录本机排空证明", Confirm: true}
	offline, e := gorm.Open(postgres.Open(os.Getenv("POWERX_STORAGE_CONTROL_TEST_DSN")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, e)
	pool, e := offline.DB()
	require.NoError(t, e)
	require.NoError(t, pool.Close())
	previous := ctrl.repo.DB
	ctrl.repo.DB = offline
	defer func() { ctrl.repo.DB = previous }()
	_, e = a.ConfirmCommandDrain(ctx, job.ID, input)
	require.Error(t, e)
	rejected, e := a.CommandConfirmationRequest(ctx, input.RequestID)
	require.NoError(t, e)
	require.Equal(t, "rejected", rejected.State)
	require.False(t, rejected.CurrentRecordMatches)
	cmdAfter, e := os.ReadFile(commandPath)
	require.NoError(t, e)
	var unchanged map[string]any
	require.NoError(t, json.Unmarshal(cmdAfter, &unchanged))
	require.Equal(t, "running", unchanged["state"])
	ctrl.repo.DB = previous
	review, e = a.ReviewCommandDrain(ctx, job.ID)
	require.NoError(t, e)
	require.True(t, review.Eligible)
	input.RequestID = uuid.NewString()
	input.ExpectedVersion = review.ReviewVersion
	out, e := a.ConfirmCommandDrain(ctx, job.ID, input)
	require.NoError(t, e)
	require.True(t, out.CurrentRecordMatches)
	original, e := a.Job(ctx, job.ID)
	require.NoError(t, e)
	require.True(t, original.InterruptionPending)
	require.Equal(t, "drained", original.CommandDrainState)
	_, e = anchor.Begin(ctx, uuid.NewString(), uuid.NewString())
	require.NoError(t, e)
	historical, e := a.ConfirmCommandDrain(ctx, job.ID, input)
	require.NoError(t, e)
	require.False(t, historical.CurrentRecordMatches)
	original, e = a.Job(ctx, job.ID)
	require.NoError(t, e)
	require.Equal(t, "unconfirmed", original.CommandDrainState)
}
