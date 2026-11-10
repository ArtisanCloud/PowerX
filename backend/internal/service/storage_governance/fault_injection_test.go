package storagegov

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	archive "github.com/ArtisanCloud/PowerX/internal/service/archive_ops"
	"github.com/ArtisanCloud/PowerX/internal/testutil/archivefault"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

// A real control transaction and formal TaskBus publication guard the synthetic
// versioned S3 writes. This does not assert real OSS or production acceptance.
func TestArchiveAcceptedNativeFaultsPreserveIdentityAndContent(t *testing.T) {
	for _, mode := range []string{"object-ack", "manifest-ack", "credential", "database-connection"} {
		t.Run(mode, func(t *testing.T) {
			s, ctrl, _, db := monitoringNative(t)
			peer := archivefault.New(t)
			root, e := filepath.EvalSymlinks(t.TempDir())
			require.NoError(t, e)
			source, state := filepath.Join(root, "source"), filepath.Join(root, "archive")
			require.NoError(t, os.Mkdir(source, 0700))
			before := map[string][]byte{}
			for i, name := range []string{"app.log", "app.log.1", "app.log.2", "app.log.3"} {
				before[name] = []byte("owned fault canary " + mode + " " + name + "\n")
				path := filepath.Join(source, name)
				require.NoError(t, os.WriteFile(path, before[name], 0600))
				at := time.Now().AddDate(0, 0, -10-i)
				require.NoError(t, os.Chtimes(path, at, at))
			}
			a := archive.NewService(context.Background(), archive.Config{Directory: state, Instance: "owned-install", ExecutionNodeID: "owned-node", Sources: []archive.Source{{ID: "logs", Name: "Owned logs", Kind: "application_logs", Directory: source, RotationFormat: "numeric", ActiveFiles: []string{"app.log"}}}, S3Credentials: map[string]archive.S3Credential{"owned": {AccessKey: "fixture", SecretKey: "fixture-secret"}}}, archive.Options{})
			t.Cleanup(a.Close)
			ctx := controlRoot()
			target, e := a.SaveTarget(ctx, archive.Target{Name: "Owned protocol peer", Driver: "s3", Endpoint: peer.URL, Bucket: "archives", Prefix: "owned/host", Region: "us-east-1", CredentialRef: "owned", ForcePathStyle: true})
			require.NoError(t, e)
			policy, e := a.SavePolicy(ctx, archive.Policy{Name: "Owned logs", SourceID: "logs", TargetID: target.ID, IntervalMinutes: 5, RetentionDays: 1, RetentionCount: 1, RemoteRetentionDays: 30, RemoteRetentionCount: 100, RemoteMaxBytes: 16 << 20})
			require.NoError(t, e)
			require.NoError(t, s.BindArchiveControl(a))
			adoptNativeTarget(t, &archiveTargetControl{s, a}, ctrl)
			require.NoError(t, a.TestTarget(ctx, target.ID))
			policies := &archivePolicyControl{s, a}
			adoptNativePolicy(t, policies, ctrl)
			active, e := policies.Resolve(ctx, policy.ID)
			require.NoError(t, e)
			currentView, e := a.Overview(ctx)
			require.NoError(t, e)
			require.Len(t, currentView.Targets, 1)
			require.True(t, currentView.Targets[0].VerificationValid)
			yes, no := true, false
			_, e = policies.Save(ctx, archive.PolicyChange{RequestID: uuid.NewString(), ExpectedRevision: 1, ExpectedPolicyRevision: active.Revision, ExpectedTargetRevision: currentView.Targets[0].Revision, ExpectedTargetFingerprint: currentView.Targets[0].VerifiedFingerprint, PolicyID: policy.ID, Action: "status", Enabled: &yes, RemoteCleanupEnabled: &no, Confirm: true})
			require.NoError(t, e)
			applyMonitoring(t, ctrl, ctrl.opts.Queue)

			pool, e := db.DB()
			require.NoError(t, e)
			pool.SetMaxOpenConns(1)
			var backendPID int
			require.NoError(t, db.Raw("SELECT pg_backend_pid()").Scan(&backendPID).Error)
			entered, release := make(chan struct{}), make(chan struct{})
			var released sync.Once
			releaseFault := func() { released.Do(func() { close(release) }) }
			t.Cleanup(releaseFault)
			_, probeDeletes, _, _ := peer.Snapshot()
			peer.Arm(mode, func() error {
				close(entered)
				<-release
				if mode != "database-connection" {
					return nil
				}
				// Only the single connection of this owned schema's pool is killed.
				// The server and all other sessions (including daily Core) are untouched.
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				conn, e := pgx.Connect(ctx, os.Getenv("POWERX_STORAGE_CONTROL_TEST_DSN"))
				if e != nil {
					return e
				}
				defer conn.Close(ctx)
				var terminated bool
				e = conn.QueryRow(ctx, "SELECT pg_terminate_backend($1)", backendPID).Scan(&terminated)
				if e == nil && !terminated {
					return os.ErrInvalid
				}
				return e
			})
			key := uuid.NewString()
			accepted, e := a.Start(ctx, policy.ID, key, "owned-test", "owned-fault-trace")
			require.NoError(t, e)
			select {
			case <-entered:
			case <-time.After(10 * time.Second):
				current, _ := a.Job(ctx, accepted.ID)
				t.Fatalf("fault point not reached: local=%s phase=%s error=%s remote=%s remote_error=%s", current.Status, current.Phase, current.Error, current.RemoteState, current.RemoteError)
			}
			releaseFault()
			var failed archive.Job
			require.Eventually(t, func() bool {
				failed, e = a.Job(ctx, accepted.ID)
				return e == nil && failed.RemoteState == "failed"
			}, 10*time.Second, 10*time.Millisecond)
			require.Equal(t, "success", failed.Status, "local capture and remote result are independent")
			require.Equal(t, accepted.ID, failed.ID)
			require.Equal(t, 1, failed.Attempts)
			require.Len(t, failed.Files, 2, "exercise nonempty sealed-file content and cleanup proofs")
			require.NotEmpty(t, failed.RemoteError)
			require.NotContains(t, failed.RemoteError, "fixture-private-upstream-detail")
			require.NotContains(t, failed.RemoteError, "fixture-secret")
			objects, deletes, fired, hookError := peer.Snapshot()
			require.True(t, fired)
			require.NoError(t, hookError)
			require.Equal(t, probeDeletes, deletes, "a partial transfer must not delete any remote version")
			manifestCount := 0
			for _, o := range objects {
				if strings.Contains(o.Key, "/manifests/") {
					manifestCount++
				}
			}
			if mode == "manifest-ack" {
				require.Equal(t, 1, manifestCount)
			} else {
				require.Zero(t, manifestCount)
			}
			for name, content := range before {
				raw, e := os.ReadFile(filepath.Join(source, name))
				require.NoError(t, e)
				require.Equal(t, content, raw)
			}
			for _, file := range failed.Files {
				raw, e := os.ReadFile(filepath.Join(state, "objects", file.SHA256[:2], file.SHA256))
				require.NoError(t, e)
				sum := sha256.Sum256(raw)
				require.Equal(t, file.SHA256, hex.EncodeToString(sum[:]))
				ok, e := a.VerifyLifecycleFile(ctx, source, filepath.Base(file.Path), file.Size, file.ModifiedAt)
				require.NoError(t, e)
				require.False(t, ok, "an unconfirmed remote write must not authorize source cleanup")
			}
			view, e := a.Overview(ctx)
			require.NoError(t, e)
			require.Equal(t, 1, view.LogPendingTransfers)
			replayed, e := a.Start(ctx, policy.ID, key, "owned-test", "owned-fault-trace")
			require.NoError(t, e)
			require.Equal(t, accepted.ID, replayed.ID)
			files, e := filepath.Glob(filepath.Join(state, "jobs", "*.json"))
			require.NoError(t, e)
			require.Len(t, files, 1)
			// Recovery is explicit in this test, on the original job, after the
			// control transaction / provider has recovered. No new job is admitted.
			peer.Arm("", nil)
			retried, e := a.Retry(ctx, accepted.ID)
			require.NoError(t, e)
			require.Equal(t, accepted.ID, retried.ID)
			var complete archive.Job
			require.Eventually(t, func() bool {
				complete, e = a.Job(ctx, accepted.ID)
				return e == nil && complete.RemoteState == "success"
			}, 10*time.Second, 10*time.Millisecond)
			require.Equal(t, 2, complete.Attempts)
			objects, deletes, _, _ = peer.Snapshot()
			require.Equal(t, probeDeletes, deletes, "fault recovery must not delete remote versions")
			counts := map[string]int{}
			for _, o := range objects {
				if strings.Contains(o.Key, "/objects/") {
					counts[o.Key]++
				}
			}
			for _, count := range counts {
				require.Equal(t, 1, count, "read-verified content objects are reused after a lost acknowledgement")
			}
			for _, file := range complete.Files {
				ok, e := a.VerifyLifecycleFile(ctx, source, filepath.Base(file.Path), file.Size, file.ModifiedAt)
				require.NoError(t, e)
				require.True(t, ok, "fresh full readback is required before deletion eligibility")
			}
			files, e = filepath.Glob(filepath.Join(state, "jobs", "*.json"))
			require.NoError(t, e)
			require.Len(t, files, 1)
			t.Logf("accepted fault=%s original_job_preserved=true request_replay_same_job=true local_content_verified=true unsafe_delete=false recovered_same_job=true", mode)
		})
	}
}
