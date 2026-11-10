package archive_ops

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	recovery "github.com/ArtisanCloud/PowerX/internal/infra/storage_recovery"
	"github.com/ArtisanCloud/PowerX/internal/testutil/archivefault"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

type restartCase struct {
	Policy, Request, Job, Scene, Mode string
	WaitTick                          bool
}

// 子进程只读取父测试创建的私有目录；不读取日常配置，不增加生产故障接口。
func TestArchiveRestartOwnedChild(t *testing.T) {
	root := os.Getenv("POWERX_ARCHIVE_RESTART_OWNED_ROOT")
	if root == "" {
		t.Skip("owned process helper")
	}
	require.True(t, strings.HasPrefix(root, "/tmp/powerx-archive-process-owned-"))
	info, e := os.Lstat(root)
	require.NoError(t, e)
	require.True(t, info.IsDir())
	require.Zero(t, info.Mode().Perm()&0077)
	var input restartCase
	require.NoError(t, readJSON(filepath.Join(root, "case.json"), &input))
	cfg, e := LoadConfig(filepath.Join(root, "archive.yaml"))
	require.NoError(t, e)
	s := NewService(context.Background(), cfg, Options{})
	defer s.Close()
	require.NoError(t, s.initErr)

	maintenance := filepath.Join(root, "maintenance")
	require.NoError(t, os.MkdirAll(maintenance, 0700))
	anchor, e := recovery.Initialize(maintenance, "owned-process")
	require.NoError(t, e)
	require.NoError(t, s.BindRecoveryAnchor(anchor))
	s.StartScheduler()
	t.Logf("owned archive component pid=%d cwd=%s command=%q listeners=none mode=%s", os.Getpid(), mustCWD(t), os.Args, input.Mode)
	if input.Mode == "start" {
		j, e := s.Start(archiveRoot(), input.Policy, input.Request, "owned.root", "")
		require.NoError(t, e)
		require.Equal(t, "active", j.ExecutionState)
		input.Job = j.ID
		require.NoError(t, writeJSON(filepath.Join(root, "accepted.json"), input))
		select {}
	}
	require.Equal(t, "verify", input.Mode)
	j, e := s.Job(archiveRoot(), input.Job)
	require.NoError(t, e)
	require.True(t, j.InterruptionPending)
	require.Equal(t, "interrupted", j.ExecutionState)
	if input.Scene == "capture" {
		require.Equal(t, "interrupted", j.Status)
	} else {
		require.Equal(t, "success", j.Status)
		require.Equal(t, "unconfirmed", j.RemoteState)
		require.NotEmpty(t, j.Files)
	}
	before, e := os.ReadFile(s.path("jobs", j.ID))
	require.NoError(t, e)
	receipt, e := s.RunRequest(archiveRoot(), input.Policy, input.Request)
	require.NoError(t, e)
	require.Equal(t, j.ID, receipt.JobID)
	require.True(t, receipt.InterruptionPending)
	after, e := os.ReadFile(s.path("jobs", j.ID))
	require.NoError(t, e)
	require.Equal(t, before, after, "lookup must stay read-only after process restart")
	_, e = s.Start(archiveRoot(), input.Policy, uuid.NewString(), "owned.root", "")
	require.Equal(t, 409, dto.StatusCode(e))
	replay, e := s.Start(archiveRoot(), input.Policy, input.Request, "owned.root", "")
	require.NoError(t, e)
	require.Equal(t, j.ID, replay.ID)
	require.NoError(t, s.Tick(archiveRoot()))
	if input.WaitTick {
		t.Log("waiting for the production minute scheduler in owned process")
		time.Sleep(62 * time.Second)
	}
	ids, e := s.records("jobs")
	require.NoError(t, e)
	require.Equal(t, []string{j.ID}, ids)
	j, e = s.Job(archiveRoot(), j.ID)
	require.NoError(t, e)
	require.True(t, j.InterruptionPending)
	require.Equal(t, 0, j.Attempts-expectedAttempts(input.Scene))
	if input.Scene == "capture" {
		_, e = s.cleanupApproved(archiveRoot(), input.Policy)
		require.NoError(t, e)
		_, e = s.Retry(archiveRoot(), j.ID)
		require.Equal(t, 409, dto.StatusCode(e))
		paused, e := s.policy(archiveRoot(), input.Policy)
		require.NoError(t, e)
		paused.Enabled = false
		paused.RemoteCleanupEnabled = false
		_, e = s.SavePolicy(archiveRoot(), paused)
		require.NoError(t, e)
		review, e := s.ReviewInterruptedCapture(credentialRootContext(), j.ID, nil)
		require.NoError(t, e)
		receipt, e := s.RetainInterruptedCapture(credentialRootContext(), j.ID, retainInput(review))
		require.NoError(t, e)
		require.Equal(t, "retained", receipt.State)
		require.True(t, receipt.CurrentRecordMatches)
		retained, e := s.Job(credentialRootContext(), j.ID)
		require.NoError(t, e)
		require.Equal(t, "retained", retained.Status)
		require.True(t, retained.Protected)
		require.False(t, retained.InterruptionPending)
	} else {
		src := s.cfg.Sources[0]
		file, e := os.Stat(filepath.Join(src.Directory, "app.log.2.gz"))
		require.NoError(t, e)
		proof, e := s.VerifyLifecycleFile(archiveRoot(), src.Directory, file.Name(), file.Size(), file.ModTime())
		require.NoError(t, e)
		require.False(t, proof)
		original, _ := s.readJob(j.ID)
		complete, e := s.retry(archiveRoot(), j.ID, true)
		require.NoError(t, e)
		require.Equal(t, j.ID, complete.ID)
		require.Equal(t, "success", complete.RemoteState)
		require.False(t, complete.InterruptionPending)
		current, e := s.readJob(j.ID)
		require.NoError(t, e)
		require.NotEqual(t, original.ExecutionID, current.ExecutionID)
		require.Equal(t, original.RequestKey, current.RequestKey)
		require.Equal(t, original.InterruptedAt, current.InterruptedAt)
		remoteManifest, e := s.readS3Job(archiveRoot(), current)
		require.NoError(t, e)
		require.False(t, remoteManifest.InterruptionPending)
		require.Empty(t, remoteManifest.ExecutionState)
		proof, e = s.VerifyLifecycleFile(archiveRoot(), src.Directory, file.Name(), file.Size(), file.ModTime())
		require.NoError(t, e)
		require.True(t, proof)
	}
	require.NoError(t, writeJSON(filepath.Join(root, "verified.json"), map[string]any{"job_id": j.ID, "scene": input.Scene, "original_request": input.Request, "minute_scheduler_observed": input.WaitTick, "no_replacement_job": true, "source_cleanup_before_retry": false, "partial_capture_preserved": true, "controlled_capture_retention": input.Scene == "capture"}))
}

func expectedAttempts(scene string) int {
	if scene == "capture" {
		return 0
	}
	return 1
}
func mustCWD(t *testing.T) string { t.Helper(); p, e := os.Getwd(); require.NoError(t, e); return p }
func restartObjects(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	require.NoError(t, filepath.WalkDir(filepath.Join(root, "objects"), func(p string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if !d.IsDir() {
			b, e := os.ReadFile(p)
			if e != nil {
				return e
			}
			sum := sha256.Sum256(b)
			out[strings.TrimPrefix(p, root)] = hex.EncodeToString(sum[:])
		}
		return nil
	}))
	return out
}

// 这是生产归档组件的真实 OS 进程验收，不冒称整套 cmd/app 重启或云 OSS 验收。
func TestArchiveOwnedProcessRestart(t *testing.T) {
	if os.Getenv("POWERX_ARCHIVE_RESTART_ACCEPTANCE") != "1" {
		t.Skip("explicit owned process acceptance only")
	}
	for _, scene := range []string{"capture", "object-ack", "manifest-ack"} {
		t.Run(scene, func(t *testing.T) {
			root, e := os.MkdirTemp("/tmp", "powerx-archive-process-owned-")
			require.NoError(t, e)
			t.Cleanup(func() { require.NoError(t, os.RemoveAll(root)) })
			source := filepath.Join(root, "source")
			require.NoError(t, os.Mkdir(source, 0700))
			cfg := Config{Directory: filepath.Join(root, "archive"), Instance: "owned-process", Sources: []Source{{ID: "source", Name: "owned", Kind: "media", Directory: source}}}
			baselineDeletes := 0
			var peer *archivefault.Peer
			var unlockCapture func()
			var checkpoint chan struct{}
			var releaseRemote func()
			if scene == "capture" {
				dsn := os.Getenv("POWERX_STORAGE_CONTROL_TEST_DSN")
				if dsn == "" {
					t.Skip("owned PostgreSQL capture checkpoint required")
				}
				pc, e := pgx.ParseConfig(dsn)
				require.NoError(t, e)
				require.True(t, strings.HasPrefix(pc.Host, "/tmp/powerx-control-owned-"))
				require.Equal(t, "storage_control_test", pc.User)
				require.Equal(t, "postgres", pc.Database)
				conn, e := pgx.Connect(context.Background(), dsn)
				require.NoError(t, e)
				t.Cleanup(func() { _ = conn.Close(context.Background()) })
				_, e = conn.Exec(context.Background(), "CREATE TABLE public.media_assets (driver text,bucket text,storage_key text,size_bytes bigint,expected_checksum text,deleted_at timestamptz,upload_state text)")
				require.NoError(t, e)
				t.Cleanup(func() {
					_, e := conn.Exec(context.Background(), "DROP TABLE public.media_assets")
					require.NoError(t, e)
				})
				tx, e := conn.Begin(context.Background())
				require.NoError(t, e)
				_, e = tx.Exec(context.Background(), "LOCK TABLE public.media_assets IN ACCESS EXCLUSIVE MODE")
				require.NoError(t, e)
				unlockCapture = func() { _ = tx.Rollback(context.Background()) }
				defer unlockCapture()
				cfg.Sources[0].DatabaseDSN = dsn + " application_name=owned_archive_capture_restart"
				for i := 0; i < 24; i++ {
					require.NoError(t, os.WriteFile(filepath.Join(source, fmt.Sprintf("image-%02d.jpg", i)), []byte(fmt.Sprintf("owned image %02d", i)), 0600))
				}
			} else {
				for i, name := range []string{"app.log", "app.log.1.gz", "app.log.2.gz"} {
					path := filepath.Join(source, name)
					require.NoError(t, os.WriteFile(path, []byte("owned archive "+name), 0600))
					at := time.Now().Add(-time.Duration(i+1) * 24 * time.Hour)
					require.NoError(t, os.Chtimes(path, at, at))
				}
				cfg.Sources[0].Kind = "application_logs"
				cfg.Sources[0].RotationFormat = "numeric"
				cfg.Sources[0].ActiveFiles = []string{"app.log"}
				cfg.S3Credentials = map[string]S3Credential{"owned": {AccessKey: "fixture-access", SecretKey: "fixture-secret"}}
				peer = archivefault.New(t)
			}
			seed := NewService(context.Background(), cfg, Options{})
			require.NoError(t, seed.initErr)
			p := Policy{Name: "owned policy", SourceID: "source", IntervalMinutes: 5, RetentionDays: 7, RetentionCount: 10, RemoteRetentionDays: 90, RemoteRetentionCount: 90, RemoteMaxBytes: 10 << 20, IncludeDatabase: scene == "capture"}
			if peer != nil {
				target, e := seed.SaveTarget(archiveRoot(), Target{Name: "owned", Driver: "s3", Endpoint: peer.URL, Bucket: "archives", Prefix: "process/owned", Region: "us-east-1", CredentialRef: "owned", ForcePathStyle: true})
				require.NoError(t, e)
				require.NoError(t, seed.TestTarget(archiveRoot(), target.ID))
				p.TargetID = target.ID
			}
			p, e = seed.SavePolicy(archiveRoot(), p)
			require.NoError(t, e)
			p.Enabled = true
			p, e = seed.SavePolicy(archiveRoot(), p)
			require.NoError(t, e)
			raw, e := marshalConfig(seed.cfg)
			require.NoError(t, e)
			require.NoError(t, os.WriteFile(filepath.Join(root, "archive.yaml"), raw, 0600))
			seed.Close()
			if peer != nil {
				_, baselineDeletes, _, _ = peer.Snapshot()
				checkpoint = make(chan struct{})
				release := make(chan struct{})
				releaseRemote = func() {
					select {
					case <-release:
					default:
						close(release)
					}
				}
				t.Cleanup(releaseRemote)
				peer.Arm(scene, func() error { close(checkpoint); <-release; return nil })
			}
			input := restartCase{Policy: p.ID, Request: uuid.NewString(), Scene: scene, Mode: "start"}
			require.NoError(t, writeJSON(filepath.Join(root, "case.json"), input))
			start := func(mode string) *exec.Cmd {
				cmd := exec.Command(os.Args[0], "-test.run=^TestArchiveRestartOwnedChild$", "-test.v", "-test.timeout=150s")
				cmd.Env = append(os.Environ(), "POWERX_ARCHIVE_RESTART_OWNED_ROOT="+root)
				cmd.Dir = mustCWD(t)
				cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
				log, e := os.OpenFile(filepath.Join(root, mode+".log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
				require.NoError(t, e)
				cmd.Stdout = log
				cmd.Stderr = log
				require.NoError(t, cmd.Start())
				require.NoError(t, log.Close())
				t.Logf("owned pid=%d command=%q cwd=%s listeners=none mode=%s", cmd.Process.Pid, cmd.Args, cmd.Dir, mode)
				t.Cleanup(func() {
					if cmd.ProcessState == nil {
						_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
						_ = cmd.Wait()
					}
				})
				return cmd
			}
			first := start("start")
			require.Eventually(t, func() bool { return readJSON(filepath.Join(root, "accepted.json"), &input) == nil }, 15*time.Second, 20*time.Millisecond)
			if peer != nil {
				select {
				case <-checkpoint:
				case <-time.After(20 * time.Second):
					t.Fatal("remote write checkpoint did not fire")
				}
			} else {
				conn, e := pgx.Connect(context.Background(), os.Getenv("POWERX_STORAGE_CONTROL_TEST_DSN"))
				require.NoError(t, e)
				require.Eventually(t, func() bool {
					var n int
					e := conn.QueryRow(context.Background(), "SELECT count(*) FROM pg_stat_activity WHERE application_name='owned_archive_capture_restart' AND wait_event_type='Lock'").Scan(&n)
					return e == nil && n == 1
				}, 20*time.Second, 20*time.Millisecond)
				require.NoError(t, conn.Close(context.Background()))
			}
			require.NoError(t, syscall.Kill(-first.Process.Pid, syscall.SIGKILL))
			require.Error(t, first.Wait())
			require.Error(t, syscall.Kill(first.Process.Pid, 0), "original process must be gone")
			if releaseRemote != nil {
				releaseRemote()
			}
			if unlockCapture != nil {
				unlockCapture()
			}
			localBefore := restartObjects(t, filepath.Join(root, "archive"))
			require.NotEmpty(t, localBefore)
			// 仅测试夹具回拨原记录时间，使下一轮真实分钟定时器确实到期；不是运行时策略变更。
			var original jobRecord
			jobPath := filepath.Join(root, "archive", "jobs", input.Job+".json")
			require.NoError(t, readJSON(jobPath, &original))
			original.CreatedAt = time.Now().Add(-48 * time.Hour)
			require.NoError(t, writeJSON(jobPath, original))
			input.Mode = "verify"
			input.WaitTick = scene == "object-ack" && os.Getenv("POWERX_ARCHIVE_RESTART_WAIT_TICK") == "1"
			require.NoError(t, writeJSON(filepath.Join(root, "case.json"), input))
			second := start("verify")
			e = second.Wait()
			log, _ := os.ReadFile(filepath.Join(root, "verify.log"))
			require.NoError(t, e, string(log))
			t.Logf("restarted result: %s", log)
			require.Equal(t, localBefore, restartObjects(t, filepath.Join(root, "archive")), "all local CAS bytes must survive restart and cleanup")
			if peer != nil {
				objects, deletes, fired, e := peer.Snapshot()
				require.NoError(t, e)
				require.True(t, fired)
				require.Equal(t, baselineDeletes, deletes, "no archive DELETE beyond the explicit target verification probe")
				require.NotEmpty(t, objects)
			}
			if out := os.Getenv("POWERX_ARCHIVE_RESTART_EVIDENCE_DIR"); out != "" {
				require.NoError(t, os.MkdirAll(out, 0700))
				body, e := os.ReadFile(filepath.Join(root, "verified.json"))
				require.NoError(t, e)
				require.NoError(t, os.WriteFile(filepath.Join(out, scene+".json"), body, 0600))
			}
		})
	}
}
