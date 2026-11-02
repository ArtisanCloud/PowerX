package archive_ops

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type testS3Object struct {
	key, version string
	body         []byte
	at           time.Time
}
type versionedS3 struct {
	mu           sync.Mutex
	objects      []testS3Object
	counter      int
	corrupt      bool
	public       bool
	publicPolicy string
}

func (m *versionedS3) serve(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()
	q := r.URL.Query()
	key := strings.TrimPrefix(r.URL.Path, "/archives/")
	errOut := func(code string, status int) {
		w.WriteHeader(status)
		_, _ = fmt.Fprintf(w, "<Error><Code>%s</Code><Message>test</Message></Error>", code)
	}
	if !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 ") {
		errOut("AccessDenied", 403)
		return
	}
	if r.Method == "GET" && q.Has("versioning") {
		_, _ = io.WriteString(w, "<VersioningConfiguration><Status>Enabled</Status></VersioningConfiguration>")
		return
	}
	if r.Method == "GET" && q.Has("policy") {
		if m.publicPolicy != "" {
			_, _ = io.WriteString(w, m.publicPolicy)
			return
		}
		if m.public {
			_, _ = io.WriteString(w, `{"Statement":[{"Effect":"Allow","Principal":"*"}]}`)
		} else {
			errOut("NoSuchBucketPolicy", 404)
		}
		return
	}
	if r.Method == "GET" && q.Has("acl") {
		_, _ = io.WriteString(w, "<AccessControlPolicy><AccessControlList/></AccessControlPolicy>")
		return
	}
	if r.Method == "GET" && q.Has("lifecycle") {
		errOut("NoSuchLifecycleConfiguration", 404)
		return
	}
	if r.Method == "GET" && q.Has("versions") {
		type row struct {
			Key     string `xml:"Key"`
			Version string `xml:"VersionId"`
			Latest  bool   `xml:"IsLatest"`
			Size    int    `xml:"Size"`
			At      string `xml:"LastModified"`
		}
		var out struct {
			XMLName   xml.Name `xml:"ListVersionsResult"`
			Rows      []row    `xml:"Version"`
			Truncated bool     `xml:"IsTruncated"`
		}
		latest := map[string]string{}
		for _, o := range m.objects {
			latest[o.key] = o.version
		}
		for _, o := range m.objects {
			if strings.HasPrefix(o.key, q.Get("prefix")) {
				out.Rows = append(out.Rows, row{o.key, o.version, latest[o.key] == o.version, len(o.body), o.at.Format(time.RFC3339)})
			}
		}
		_ = xml.NewEncoder(w).Encode(out)
		return
	}
	if r.Method == "PUT" {
		if r.Header.Get("If-None-Match") == "*" {
			for _, o := range m.objects {
				if o.key == key {
					errOut("PreconditionFailed", 412)
					return
				}
			}
		}
		raw, _ := io.ReadAll(r.Body)
		m.counter++
		version := strconv.Itoa(m.counter)
		m.objects = append(m.objects, testS3Object{key, version, raw, time.Now().UTC()})
		w.Header().Set("x-amz-version-id", version)
		return
	}
	if r.Method == "GET" {
		for i := len(m.objects) - 1; i >= 0; i-- {
			o := m.objects[i]
			if o.key == key && (q.Get("versionId") == "" || q.Get("versionId") == o.version) {
				data := o.body
				if m.corrupt && strings.Contains(key, "/objects/") {
					data = bytes.Repeat([]byte("!"), len(data))
				}
				w.Header().Set("x-amz-version-id", o.version)
				w.Header().Set("Content-Length", strconv.Itoa(len(data)))
				_, _ = w.Write(data)
				return
			}
		}
		errOut("NoSuchVersion", 404)
		return
	}
	if r.Method == "DELETE" {
		for i, o := range m.objects {
			if o.key == key && o.version == q.Get("versionId") {
				m.objects = append(m.objects[:i], m.objects[i+1:]...)
				w.WriteHeader(204)
				return
			}
		}
		errOut("NoSuchVersion", 404)
		return
	}
	errOut("InvalidRequest", 400)
}
func s3Fixture(t *testing.T) (*Service, *versionedS3, Target, Policy) {
	t.Helper()
	m := &versionedS3{}
	server := httptest.NewServer(http.HandlerFunc(m.serve))
	t.Cleanup(server.Close)
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	for i, name := range []string{"app.log", "app.log.1.gz", "app.log.2.gz", "app.log.3.gz"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("audit sample "+name+"\n"), 0600))
		stamp := time.Now().Add(-time.Duration(i+1) * 24 * time.Hour)
		require.NoError(t, os.Chtimes(filepath.Join(dir, name), stamp, stamp))
	}
	s := NewService(context.Background(), Config{Directory: t.TempDir(), Instance: "fixture", Sources: []Source{{ID: "logs", Name: "test logs", Kind: "application_logs", Directory: dir, RotationFormat: "numeric", ActiveFiles: []string{"app.log"}}}, S3Credentials: map[string]S3Credential{"archive-test": {AccessKey: "fixture-access", SecretKey: "fixture-secret"}}}, Options{})
	t.Cleanup(s.Close)
	target, err := s.SaveTarget(archiveRoot(), Target{Driver: "s3", Name: "independent target", Endpoint: server.URL, Bucket: "archives", Prefix: "hosts/test", Region: "us-east-1", CredentialRef: "archive-test", ForcePathStyle: true})
	require.NoError(t, err)
	p, err := s.SavePolicy(archiveRoot(), Policy{Name: "log policy", SourceID: "logs", TargetID: target.ID, IntervalMinutes: 5, RetentionDays: 7, RetentionCount: 10, RemoteRetentionDays: 180, RemoteRetentionCount: 100, RemoteMaxBytes: 10 << 20})
	require.NoError(t, err)
	return s, m, target, p
}
func TestS3ExplicitEnableVersionedReadbackAndHistoricalRetrieval(t *testing.T) {
	s, m, target, p := s3Fixture(t)
	_, err := s.Start(archiveRoot(), p.ID, uuid.NewString(), "root.test", "")
	require.Error(t, err)
	p.Enabled = true
	_, err = s.SavePolicy(archiveRoot(), p)
	require.Error(t, err, "testing the target must precede enable")
	require.NoError(t, s.TestTarget(archiveRoot(), target.ID))
	p, err = s.SavePolicy(archiveRoot(), p)
	require.NoError(t, err)
	j := captureTest(t, s, p)
	require.Equal(t, "success", j.RemoteState, j.RemoteError)
	require.NotEmpty(t, j.RemoteManifestVersion)
	require.Len(t, j.Files, 2)
	for _, f := range j.Files {
		require.NoError(t, os.Remove(s.objectPath(f.SHA256)))
	}
	for _, name := range []string{"app.log.2.gz", "app.log.3.gz"} {
		require.NoError(t, os.Remove(filepath.Join(s.cfg.Sources[0].Directory, name)))
	}
	var downloaded bytes.Buffer
	require.NoError(t, s.Download(archiveRoot(), j.ID, &downloaded))
	require.Greater(t, downloaded.Len(), 0)
	page, err := s.Catalog(archiveRoot(), CatalogFilter{Query: "app.log.2", Page: 1, PageSize: 20})
	require.NoError(t, err)
	require.Len(t, page.Items, 1, "source removal must not remove archive history")
	p.Enabled = false
	p, err = s.SavePolicy(archiveRoot(), p)
	require.NoError(t, err)
	m.mu.Lock()
	before := m.counter
	m.mu.Unlock()
	_, err = s.Start(archiveRoot(), p.ID, uuid.NewString(), "root.test", "")
	require.Error(t, err)
	m.mu.Lock()
	require.Equal(t, before, m.counter)
	m.mu.Unlock()
	// 关闭归档后仍能读取已有历史，配置与清理不会隐式删除归档。
	downloaded.Reset()
	require.NoError(t, s.Download(archiveRoot(), j.ID, &downloaded))
}
func TestS3CorruptionPublicBucketAndChangedTargetFailClosed(t *testing.T) {
	s, m, target, p := s3Fixture(t)
	m.mu.Lock()
	m.public = true
	m.mu.Unlock()
	require.Error(t, s.TestTarget(archiveRoot(), target.ID))
	m.mu.Lock()
	m.public = false
	m.mu.Unlock()
	require.NoError(t, s.TestTarget(archiveRoot(), target.ID))
	p.Enabled = true
	var err error
	p, err = s.SavePolicy(archiveRoot(), p)
	require.NoError(t, err)
	j := captureTest(t, s, p)
	require.Equal(t, "success", j.RemoteState, j.RemoteError)
	m.mu.Lock()
	m.corrupt = true
	m.mu.Unlock()
	for _, f := range j.Files {
		require.NoError(t, os.Remove(s.objectPath(f.SHA256)))
	}
	var download bytes.Buffer
	require.Error(t, s.Download(archiveRoot(), j.ID, &download))
	require.Zero(t, download.Len(), "do not stream a successful empty/corrupted archive")
	target.Name = "changed target"
	_, err = s.SaveTarget(archiveRoot(), target)
	require.NoError(t, err)
	_, err = s.Start(archiveRoot(), p.ID, uuid.NewString(), "root.test", "")
	require.Error(t, err)
	_, err = s.SaveTarget(archiveRoot(), target)
	require.Error(t, err, "concurrent stale target revision must fail")
}
func TestS3CredentialIsPrivateAndNeverReturned(t *testing.T) {
	s, _, _, _ := s3Fixture(t)
	view, err := s.SaveS3Credential(archiveRoot(), SaveS3CredentialInput{Ref: "private-new", AccessKey: "test-key", SecretKey: "test-secret"})
	require.NoError(t, err)
	require.EqualValues(t, 1, view.Revision)
	info, err := os.Stat(filepath.Join(s.cfg.Directory, "secrets", "private-new.json"))
	require.NoError(t, err)
	require.Zero(t, info.Mode().Perm()&0077)
	out, err := s.Overview(archiveRoot())
	require.NoError(t, err)
	raw, err := json.Marshal(out)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "test-secret")
	require.NotContains(t, string(raw), "test-key")
	_, err = s.SaveS3Credential(archiveRoot(), SaveS3CredentialInput{Ref: "private-new", AccessKey: "changed", SecretKey: "changed"})
	require.Error(t, err)
}

func TestRemoteExpiryKeepsSharedVersionsAndRejectsChangedPreview(t *testing.T) {
	s, m, target, p := s3Fixture(t)
	require.NoError(t, s.TestTarget(archiveRoot(), target.ID))
	p.Enabled = true
	var err error
	p, err = s.SavePolicy(archiveRoot(), p)
	require.NoError(t, err)
	first := captureTest(t, s, p)
	require.Equal(t, "success", first.RemoteState, first.RemoteError)
	path := filepath.Join(s.cfg.Sources[0].Directory, "app.log.2.gz")
	require.NoError(t, os.WriteFile(path, []byte("next immutable log segment"), 0600))
	stamp := time.Now().Add(-72 * time.Hour)
	require.NoError(t, os.Chtimes(path, stamp, stamp))
	second := captureTest(t, s, p)
	require.Equal(t, "success", second.RemoteState, second.RemoteError)
	s.opts.RetentionNow = func() time.Time { return time.Now().Add(181 * 24 * time.Hour) }
	preview, err := s.PreviewRemoteGC(archiveRoot(), p.ID)
	require.NoError(t, err)
	require.Equal(t, 2, preview.CandidateVersions, "only the old manifest and unshared content version may expire")
	_, err = s.Protect(archiveRoot(), first.ID, true)
	require.NoError(t, err)
	run, err := s.StartRemoteGC(archiveRoot(), preview.ID)
	require.NoError(t, err)
	require.Equal(t, "queued", run.State, "the response must remain a snapshot while the worker advances")
	require.Eventually(t, func() bool {
		var result remoteGCPlan
		return readJSON(s.path("remote-gc", run.ID), &result) == nil && result.State == "blocked"
	}, 5*time.Second, 10*time.Millisecond)
	require.Equal(t, "queued", run.State)
	_, err = s.Protect(archiveRoot(), first.ID, false)
	require.NoError(t, err)
	preview, err = s.PreviewRemoteGC(archiveRoot(), p.ID)
	require.NoError(t, err)
	run, err = s.StartRemoteGC(archiveRoot(), preview.ID)
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		var result remoteGCPlan
		return readJSON(s.path("remote-gc", run.ID), &result) == nil && result.State == "completed"
	}, 5*time.Second, 10*time.Millisecond)
	old, err := s.Job(archiveRoot(), first.ID)
	require.NoError(t, err)
	require.Equal(t, "expired", old.Status)
	var data bytes.Buffer
	require.Eventually(t, func() bool { data.Reset(); return s.Download(archiveRoot(), second.ID, &data) == nil }, time.Second, 10*time.Millisecond)
	require.Greater(t, data.Len(), 0)
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, f := range second.Files {
		found := false
		for _, o := range m.objects {
			if o.key == f.ArchiveKey && o.version == f.ArchiveVersion {
				found = true
			}
		}
		require.True(t, found, "latest archive references must remain")
	}
}

func TestMigrationCreatesVerifiedProtectedCopyAndPreservesOrigin(t *testing.T) {
	s, m, target, p := s3Fixture(t)
	require.NoError(t, s.TestTarget(archiveRoot(), target.ID))
	p.Enabled = true
	var err error
	p, err = s.SavePolicy(archiveRoot(), p)
	require.NoError(t, err)
	old := captureTest(t, s, p)
	target2 := target
	target2.ID = ""
	target2.Revision = 0
	target2.Name = "new destination"
	target2.Prefix = "hosts/destination"
	target2, err = s.SaveTarget(archiveRoot(), target2)
	require.NoError(t, err)
	require.NoError(t, s.TestTarget(archiveRoot(), target2.ID))
	dest := p
	dest.ID = ""
	dest.Revision = 0
	dest.TargetID = target2.ID
	dest.Name = "destination"
	dest, err = s.SavePolicy(archiveRoot(), dest)
	require.NoError(t, err)
	key := uuid.NewString()
	job, err := s.MigrateArchive(archiveRoot(), old.ID, dest.ID, key, "root.test")
	require.NoError(t, err)
	require.Eventually(t, func() bool { j, e := s.readJob(job.ID); return e == nil && j.Status != "running" }, 5*time.Second, 10*time.Millisecond)
	done, err := s.readJob(job.ID)
	require.NoError(t, err)
	require.Equal(t, "success", done.RemoteState, done.Error)
	require.True(t, done.Protected)
	require.Equal(t, old.ID, done.MigratedFrom)
	duplicate, err := s.MigrateArchive(archiveRoot(), old.ID, dest.ID, key, "root.test")
	require.NoError(t, err)
	require.Equal(t, job.ID, duplicate.ID)
	for _, f := range old.Files {
		_ = os.Remove(s.objectPath(f.SHA256))
	}
	var data bytes.Buffer
	require.Eventually(t, func() bool { data.Reset(); return s.Download(archiveRoot(), job.ID, &data) == nil }, time.Second, 10*time.Millisecond)
	require.NotZero(t, data.Len())
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, f := range old.Files {
		found := false
		for _, v := range m.objects {
			if v.key == f.ArchiveKey && v.version == f.ArchiveVersion {
				found = true
			}
		}
		require.True(t, found, "migration must preserve every origin version")
	}
}

func TestVerificationIsBoundToExecutionNode(t *testing.T) {
	s, _, target, _ := s3Fixture(t)
	require.NoError(t, s.TestTarget(archiveRoot(), target.ID))
	target, err := s.target(target.ID)
	require.NoError(t, err)
	require.True(t, s.targetVerified(target))
	oldHost := s.hostIdentity
	s.hostIdentity = "another-machine-with-the-same-logical-node"
	require.False(t, s.targetVerified(target))
	s.hostIdentity = oldHost
	s.cfg.ExecutionNodeID = "another-host"
	require.False(t, s.targetVerified(target), "copying private config to another executor must invalidate loopback and target proofs")
}

func TestAutomaticTickSkipsUnchangedSealedFilesButDetectsSameMtimeReplacement(t *testing.T) {
	s, m, target, p := s3Fixture(t)
	require.NoError(t, s.TestTarget(archiveRoot(), target.ID))
	p.Enabled = true
	var err error
	p, err = s.SavePolicy(archiveRoot(), p)
	require.NoError(t, err)
	first := captureTest(t, s, p)
	require.Equal(t, "success", first.RemoteState)
	stored, err := s.readJob(first.ID)
	require.NoError(t, err)
	stored.CreatedAt = time.Now().Add(-10 * time.Minute)
	require.NoError(t, s.saveJob(&stored))
	m.mu.Lock()
	before := m.counter
	m.mu.Unlock()
	require.NoError(t, s.Tick(archiveRoot()))
	m.mu.Lock()
	require.Equal(t, before, m.counter)
	m.mu.Unlock()
	ids, err := s.records("jobs")
	require.NoError(t, err)
	require.Len(t, ids, 1)
	path := filepath.Join(s.cfg.Sources[0].Directory, "app.log.2.gz")
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, bytes.Repeat([]byte("x"), int(info.Size())), 0600))
	require.NoError(t, os.Chtimes(path, info.ModTime(), info.ModTime()))
	require.NoError(t, s.Tick(archiveRoot()))
	ids, err = s.records("jobs")
	require.NoError(t, err)
	require.Len(t, ids, 2, "ctime/inode changes must prevent an unchanged decision even with identical size and mtime")
}

func TestAllowNotPrincipalAndUnknownPrincipalCannotPassPrivateBucketProof(t *testing.T) {
	s, m, target, _ := s3Fixture(t)
	for _, policy := range []string{`{"Statement":[{"Effect":"Allow","NotPrincipal":{"AWS":"arn:aws:iam::123:root"}}]}`, `{"Statement":[{"Effect":"Allow"}]}`} {
		m.mu.Lock()
		m.publicPolicy = policy
		m.mu.Unlock()
		require.Error(t, s.TestTarget(archiveRoot(), target.ID))
	}
}
