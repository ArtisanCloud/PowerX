package archive_ops

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestLiveS3Governance 仅使用显式提供的隔离测试桶，不使用日常实例配置。
func TestLiveS3Governance(t *testing.T) {
	file := os.Getenv("POWERX_STORAGE_VALIDATION_CONFIG")
	if file == "" {
		t.Skip("isolated S3 configuration not provided")
	}
	var c struct {
		Region   string `json:"region"`
		Endpoint string `json:"endpoint"`
		Bucket   string `json:"bucket"`
		Access   string `json:"access_key"`
		Secret   string `json:"secret_key"`
	}
	raw, err := os.ReadFile(file)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &c))
	require.Equal(t, "powerx-storage-validation", c.Bucket)
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	now := time.Now()
	for i, name := range []string{"app.log", "app.log.1", "app.log.2"} {
		p := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(p, []byte("owned validation log\n"), 0600))
		when := now.Add(-time.Duration(i+1) * 48 * time.Hour)
		require.NoError(t, os.Chtimes(p, when, when))
	}
	svc := NewService(context.Background(), Config{Directory: t.TempDir(), Instance: "live-validation", Sources: []Source{{ID: "logs", Name: "isolated logs", Kind: "application_logs", Directory: dir, RotationFormat: "numeric", ActiveFiles: []string{"app.log"}}}, S3Credentials: map[string]S3Credential{"test": {AccessKey: c.Access, SecretKey: c.Secret}}}, Options{})
	t.Cleanup(svc.Close)
	target, err := svc.SaveTarget(archiveRoot(), Target{Driver: "s3", Name: "isolated MinIO", Endpoint: c.Endpoint, Bucket: c.Bucket, Prefix: "test/" + uuid.NewString() + "/primary", Region: c.Region, CredentialRef: "test", ForcePathStyle: true})
	require.NoError(t, err)
	client, e := svc.s3Client(target)
	require.NoError(t, e)
	enabled, e := client.BucketVersioningEnabled(archiveRoot(), target.Bucket)
	if e != nil {
		t.Fatalf("bucket versioning protocol failed: code=%s", minio.ToErrorResponse(e).Code)
	}
	require.True(t, enabled)
	require.NoError(t, svc.TestTarget(archiveRoot(), target.ID))
	p, err := svc.SavePolicy(archiveRoot(), Policy{Name: "validation", SourceID: "logs", TargetID: target.ID, IntervalMinutes: 5, RetentionDays: 7, RetentionCount: 10, RemoteRetentionDays: 1, RemoteRetentionCount: 100, RemoteMaxBytes: 16 << 20, Enabled: true})
	require.NoError(t, err)
	first := captureTest(t, svc, p)
	require.Equal(t, "success", first.RemoteState, first.RemoteError)
	require.NotEmpty(t, first.RemoteManifestVersion)
	for _, f := range first.Files {
		require.NoError(t, os.Remove(svc.objectPath(f.SHA256)))
	}
	require.NoError(t, os.Remove(filepath.Join(dir, "app.log.2")))
	var data bytes.Buffer
	require.NoError(t, svc.Download(archiveRoot(), first.ID, &data))
	require.NotZero(t, data.Len())
	catalog, err := svc.Catalog(archiveRoot(), CatalogFilter{Query: "app.log.2", Page: 1, PageSize: 20})
	require.NoError(t, err)
	require.Len(t, catalog.Items, 1)
	preview, err := svc.PreviewLog(archiveRoot(), first.ID, first.Files[0].Path)
	require.NoError(t, err)
	require.Contains(t, preview.Text, "owned validation log")
	require.NoError(t, os.Remove(svc.path("jobs", first.ID)))
	rebuilt, e := svc.ImportLegacy(archiveRoot(), target.ID)
	require.NoError(t, e)
	require.Equal(t, 1, rebuilt.Imported)
	recovered, e := svc.readJob(first.ID)
	require.NoError(t, e)
	require.True(t, recovered.Recovered)
	require.True(t, recovered.Protected)
	destination := target
	destination.ID = ""
	destination.Revision = 0
	destination.Prefix = target.Prefix + "-migrated"
	destination, err = svc.SaveTarget(archiveRoot(), destination)
	require.NoError(t, err)
	require.NoError(t, svc.TestTarget(archiveRoot(), destination.ID))
	destPolicy := p
	destPolicy.ID = ""
	destPolicy.Revision = 0
	destPolicy.TargetID = destination.ID
	destPolicy, err = svc.SavePolicy(archiveRoot(), destPolicy)
	require.NoError(t, err)
	copied, err := svc.MigrateArchive(archiveRoot(), first.ID, destPolicy.ID, uuid.NewString(), "root.validation")
	require.NoError(t, err)
	require.Eventually(t, func() bool { j, e := svc.readJob(copied.ID); return e == nil && j.Status != "running" }, 5*time.Second, 10*time.Millisecond)
	copyJob, err := svc.readJob(copied.ID)
	require.NoError(t, err)
	require.Equal(t, "success", copyJob.RemoteState, copyJob.Error)
	for _, f := range copyJob.Files {
		_ = os.Remove(svc.objectPath(f.SHA256))
	}
	data.Reset()
	require.Eventually(t, func() bool { data.Reset(); return svc.Download(archiveRoot(), copied.ID, &data) == nil }, time.Second, 10*time.Millisecond)
	require.NoError(t, readS3Version(archiveRoot(), client, target, first.Files[0].ArchiveKey, first.Files[0].ArchiveVersion, first.Files[0].Size, first.Files[0].SHA256, nil))
	var expiring Job
	for _, content := range []string{"old separate content\n", "newest separate content\n"} {
		path := filepath.Join(dir, "app.log.2")
		require.NoError(t, os.WriteFile(path, []byte(content), 0600))
		when := time.Now().Add(-8 * 24 * time.Hour)
		require.NoError(t, os.Chtimes(path, when, when))
		j := captureTest(t, svc, p)
		require.Equal(t, "success", j.RemoteState, j.RemoteError)
		if expiring.ID == "" {
			expiring = j
		}
	}
	svc.opts.RetentionNow = func() time.Time { return time.Now().Add(2 * 24 * time.Hour) }
	gc, e := svc.PreviewRemoteGC(archiveRoot(), p.ID)
	require.NoError(t, e)
	require.Equal(t, 2, gc.CandidateVersions)
	_, e = svc.StartRemoteGC(archiveRoot(), gc.ID)
	require.NoError(t, e)
	require.Eventually(t, func() bool {
		var state remoteGCPlan
		return readJSON(svc.path("remote-gc", gc.ID), &state) == nil && state.State == "completed"
	}, 5*time.Second, 10*time.Millisecond)
	expired, e := svc.readJob(expiring.ID)
	require.NoError(t, e)
	require.Equal(t, "expired", expired.RemoteState)
	if os.Getenv("POWERX_STORAGE_VALIDATE_AUTO") == "1" {
		automatic := p
		automatic.ID = ""
		automatic.Revision = 0
		automatic.Name = "automatic validation"
		draft := automatic
		require.Eventually(t, func() bool {
			saved, e := svc.SavePolicy(archiveRoot(), draft)
			if e == nil {
				automatic = saved
			}
			return e == nil
		}, 2*time.Second, 20*time.Millisecond)
		started := time.Now()
		svc.StartScheduler()
		require.Eventually(t, func() bool {
			view, e := svc.Overview(archiveRoot())
			if e != nil {
				return false
			}
			for _, j := range view.Jobs {
				if j.PolicyID == automatic.ID && j.RemoteState == "success" {
					return true
				}
			}
			return false
		}, 80*time.Second, 250*time.Millisecond)
		t.Logf("real automatic scheduler cycle verified after %.2f seconds", time.Since(started).Seconds())
	}
	t.Logf("real MinIO: explicit enable, exact-version readback, source-gone catalog/download/preview, remote catalog rebuild and verified migration (origin retained) and exact-version remote expiry verified; archive=%s files=%d bytes=%d", first.ID, len(first.Files), data.Len())
}
