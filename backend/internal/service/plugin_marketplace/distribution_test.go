package plugin_marketplace

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	market "github.com/ArtisanCloud/PowerX/internal/infra/plugin/marketplace"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type distributionFixture struct {
	mu              sync.Mutex
	server          *httptest.Server
	registrations   int
	heartbeats      int
	credentials     map[string]string
	tickets         map[string]market.Download
	downloadKeys    []string
	reports         [][]byte
	accepted        []market.TransferReport
	failReports     int
	rejectReports   int
	failDownloads   int
	badRegistration bool
	revoked         bool
}

func newDistributionFixture(t *testing.T) *distributionFixture {
	t.Helper()
	f := &distributionFixture{credentials: map[string]string{}, tickets: map[string]market.Download{}}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		respond := func(v any) { _ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": v}) }
		if r.URL.Path == "/api/v1/host/instances/register" {
			var body map[string]string
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			require.True(t, strings.HasPrefix(r.Header.Get("Authorization"), "Bearer pxm_enroll_"))
			require.Equal(t, runtime.GOOS, body["os"])
			require.Equal(t, runtime.GOARCH, body["arch"])
			require.Equal(t, "1.0.0", body["powerx_version"])
			f.registrations++
			if f.badRegistration {
				_, _ = w.Write([]byte("lost response"))
				return
			}
			token := "pxm_instance_" + uuid.NewString()
			f.credentials[token] = body["fingerprint_sha256"]
			respond(map[string]any{"instance": map[string]string{"id": uuid.NewString()}, "instance_token": token})
			return
		}
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if f.revoked {
			w.WriteHeader(401)
			return
		}
		require.NotEmpty(t, f.credentials[token])
		require.Equal(t, f.credentials[token], r.Header.Get("X-PowerX-Fingerprint"))
		switch {
		case r.URL.Path == "/api/v1/host/instances/heartbeat":
			f.heartbeats++
			respond(map[string]string{"status": "recorded"})
		case strings.HasSuffix(r.URL.Path, "/versions"):
			respond(market.Page[market.Version]{Items: []market.Version{{PluginID: "com.test.demo", Semver: "1.2.0", MinCore: "1.0.0", Target: market.Target{OS: runtime.GOOS, Arch: runtime.GOARCH, SHA256: strings.Repeat("a", 64), Size: 123}}}, Total: 1})
		case r.URL.Path == "/api/v1/downloads":
			key := r.Header.Get("Idempotency-Key")
			require.NotEmpty(t, key)
			f.downloadKeys = append(f.downloadKeys, key)
			ticket, ok := f.tickets[key]
			if !ok {
				ticket = market.Download{DownloadID: uuid.NewString(), TrackingMode: "registered_instance", Status: "url_issued", SignedURL: "https://storage.invalid/package?secret-signature", ExpiresAt: time.Now().Add(time.Minute), Target: market.Target{OS: runtime.GOOS, Arch: runtime.GOARCH, SHA256: strings.Repeat("a", 64), Size: 123}}
				f.tickets[key] = ticket
			}
			if f.failDownloads > 0 {
				f.failDownloads--
				w.WriteHeader(503)
				return
			}
			respond(ticket)
		case strings.HasSuffix(r.URL.Path, "/events"):
			var event market.TransferReport
			require.NoError(t, json.NewDecoder(r.Body).Decode(&event))
			raw, err := json.Marshal(event)
			require.NoError(t, err)
			f.reports = append(f.reports, raw)
			if f.rejectReports != 0 {
				w.WriteHeader(f.rejectReports)
				return
			}
			if f.failReports > 0 {
				f.failReports--
				w.WriteHeader(503)
				return
			}
			f.accepted = append(f.accepted, event)
			respond(map[string]string{"status": event.Status})
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(f.server.Close)
	return f
}
func distributionService(t *testing.T, f *distributionFixture, dir, environment, enrollment string, inventory Inventory) *Service {
	t.Helper()
	client := market.NewClient(f.server.URL+"/api/v1", "legacy-must-not-be-used", f.server.Client())
	store, err := market.OpenDistributionStore(dir, client.SourceID(), environment)
	require.NoError(t, err)
	s := NewService(client, "v1.0.0", inventory)
	s.ConfigureDistribution(store, enrollment, time.Minute, nil)
	t.Cleanup(s.CloseDistribution)
	return s
}
func preparedAttempt(t *testing.T, s *Service, enable bool) (string, market.Download) {
	t.Helper()
	id, err := s.BeginInstall("com.test.demo", "1.2.0", "tenant-a", enable, "old-revision")
	require.NoError(t, err)
	ticket, err := s.ResolveDownload(context.Background(), "com.test.demo", "1.2.0", "tenant-a", id)
	require.NoError(t, err)
	return id, ticket
}
func downloadedAttempt(t *testing.T, s *Service, id string) {
	t.Helper()
	require.NoError(t, s.RecordDownloaded(id, plugin_mgr.DownloadVerification{SHA256: strings.Repeat("a", 64), Size: 123}))
}
func installedAttempt(t *testing.T, s *Service, id string) {
	t.Helper()
	require.NoError(t, s.RecordInstalled(id, plugin_mgr.Plugin{ID: "com.test.demo", Version: "1.2.0", Revision: "new-revision"}))
}

func TestDistributionDistinctEnvironmentsAndRestartKeepIdentity(t *testing.T) {
	f := newDistributionFixture(t)
	dir := t.TempDir()
	a := distributionService(t, f, dir, "instance-a", "pxm_enroll_a", nil)
	b := distributionService(t, f, dir, "instance-b", "pxm_enroll_b", nil)
	require.NoError(t, a.ConnectInstance(context.Background()))
	require.NoError(t, b.ConnectInstance(context.Background()))
	require.NotEqual(t, a.Status().Instance.ID, b.Status().Instance.ID)
	require.NotEqual(t, a.distribution.store.Snapshot().Fingerprint, b.distribution.store.Snapshot().Fingerprint)
	identity := a.Status().Instance.ID
	credential := a.distribution.store.Snapshot().InstanceToken
	require.NoError(t, a.HeartbeatInstance(context.Background()))
	a.CloseDistribution()
	a = distributionService(t, f, dir, "instance-a", "pxm_enroll_a", nil)
	require.NoError(t, a.ConnectInstance(context.Background()))
	require.NoError(t, a.HeartbeatInstance(context.Background()))
	require.Equal(t, identity, a.Status().Instance.ID)
	require.Equal(t, credential, a.distribution.store.Snapshot().InstanceToken)
	f.mu.Lock()
	require.Equal(t, 2, f.registrations)
	require.Equal(t, 2, f.heartbeats)
	f.mu.Unlock()
	raw, err := json.Marshal(a.Status())
	require.NoError(t, err)
	require.NotContains(t, string(raw), credential)
	require.NotContains(t, string(raw), "pxm_enroll_")
	files, err := filepath.Glob(filepath.Join(dir, "*", "distribution.json"))
	require.NoError(t, err)
	require.Len(t, files, 2)
	for _, file := range files {
		info, err := os.Stat(file)
		require.NoError(t, err)
		require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
		data, err := os.ReadFile(file)
		require.NoError(t, err)
		require.NotContains(t, string(data), "pxm_enroll_a")
	}
}

func TestDistributionOutboxSurvivesOfflineRestartAndKeepsPayload(t *testing.T) {
	f := newDistributionFixture(t)
	dir := t.TempDir()
	s := distributionService(t, f, dir, "instance-a", "pxm_enroll_a", nil)
	require.NoError(t, s.ConnectInstance(context.Background()))
	id, ticket := preparedAttempt(t, s, true)
	require.NotEmpty(t, ticket.DownloadID)
	downloadedAttempt(t, s, id)
	installedAttempt(t, s, id)
	require.NoError(t, s.RecordEnabled(id))
	f.failReports = 1
	require.NoError(t, s.FlushDistribution(context.Background()))
	require.Equal(t, 3, s.Status().Instance.PendingReports)
	require.NoError(t, s.FlushDistribution(context.Background()))
	require.Len(t, f.reports, 1) // 退避未到期，不忙重试。
	first := string(f.reports[0])
	s.CloseDistribution()
	s = distributionService(t, f, dir, "instance-a", "", nil)
	require.NoError(t, s.distribution.store.Update(func(st *market.DistributionState) error { st.Attempts[id].NextAttempt = time.Time{}; return nil }))
	require.NoError(t, s.RecoverDistribution(context.Background()))
	require.NoError(t, s.FlushDistribution(context.Background()))
	require.Zero(t, s.Status().Instance.PendingReports)
	require.Equal(t, first, string(f.reports[1]))
	require.Len(t, f.downloadKeys, 1)
	require.Equal(t, []string{"downloaded", "installed", "enabled"}, []string{f.accepted[0].Status, f.accepted[1].Status, f.accepted[2].Status})
	require.NoError(t, s.FlushDistribution(context.Background()))
	require.Len(t, f.reports, 4)
	files, err := filepath.Glob(filepath.Join(dir, "*", "distribution.json"))
	require.NoError(t, err)
	raw, err := os.ReadFile(files[0])
	require.NoError(t, err)
	require.NotContains(t, string(raw), "secret-signature")
}

func TestDistributionFailureStagesAndManualEnable(t *testing.T) {
	for _, stage := range []string{"download", "install", "enable", "manual_enable"} {
		t.Run(stage, func(t *testing.T) {
			f := newDistributionFixture(t)
			s := distributionService(t, f, t.TempDir(), "instance-a", "pxm_enroll_a", nil)
			require.NoError(t, s.ConnectInstance(context.Background()))
			id, _ := preparedAttempt(t, s, false)
			if stage != "download" {
				downloadedAttempt(t, s, id)
			}
			if stage == "enable" || stage == "manual_enable" {
				installedAttempt(t, s, id)
			}
			if stage == "manual_enable" {
				p := plugin_mgr.Plugin{ID: "com.test.demo", Version: "1.2.0", Revision: "new-revision", InstallOrigin: s.Origin()}
				require.NoError(t, s.RecordPluginEnableResult(p, "other-tenant", true))
				require.Equal(t, "installed", s.distribution.store.Snapshot().Attempts[id].Phase)
				require.NoError(t, s.RecordPluginEnableResult(p, "tenant-a", true))
				require.NoError(t, s.FlushDistribution(context.Background()))
				require.Equal(t, "enabled", f.accepted[len(f.accepted)-1].Status)
			} else {
				require.NoError(t, s.RecordFailed(id, "POWERX_TEST_FAILURE"))
				require.NoError(t, s.FlushDistribution(context.Background()))
				last := f.accepted[len(f.accepted)-1]
				require.Equal(t, "failed", last.Status)
				require.Equal(t, stage, last.Stage)
			}
		})
	}
}

func TestDistributionRegistrationAmbiguityDoesNotReuseCode(t *testing.T) {
	f := newDistributionFixture(t)
	f.badRegistration = true
	dir := t.TempDir()
	s := distributionService(t, f, dir, "instance-a", "pxm_enroll_a", nil)
	require.Error(t, s.ConnectInstance(context.Background()))
	require.False(t, s.Status().DownloadReady)
	s.CloseDistribution()
	f.badRegistration = false
	s = distributionService(t, f, dir, "instance-a", "pxm_enroll_a", nil)
	require.Error(t, s.ConnectInstance(context.Background()))
	require.Equal(t, 1, f.registrations)
	s.CloseDistribution()
	s = distributionService(t, f, dir, "instance-a", "pxm_enroll_new", nil)
	require.NoError(t, s.ConnectInstance(context.Background()))
	require.Equal(t, 2, f.registrations)
	require.True(t, s.Status().DownloadReady)
}

func TestDistributionRevocationStopsCredentialsAndLegacyFallback(t *testing.T) {
	f := newDistributionFixture(t)
	s := distributionService(t, f, t.TempDir(), "instance-a", "pxm_enroll_a", nil)
	require.NoError(t, s.ConnectInstance(context.Background()))
	id, _ := preparedAttempt(t, s, false)
	downloadedAttempt(t, s, id)
	f.revoked = true
	require.Error(t, s.HeartbeatInstance(context.Background()))
	require.Equal(t, "credential_rejected", s.Status().Instance.State)
	require.False(t, s.Status().DownloadReady)
	_, err := s.BeginInstall("com.test.demo", "1.2.0", "tenant-a", false, "")
	require.Error(t, err)
	require.NoError(t, s.FlushDistribution(context.Background()))
	require.Empty(t, f.reports)
}

func TestDistributionRecoverInstallOnlyReconcilesActualRevision(t *testing.T) {
	for _, mode := range []string{"committed", "unchanged", "enable_unconfirmed"} {
		t.Run(mode, func(t *testing.T) {
			f := newDistributionFixture(t)
			var inventory []plugin_mgr.Plugin
			lookup := func(context.Context) ([]plugin_mgr.Plugin, error) { return inventory, nil }
			dir := t.TempDir()
			s := distributionService(t, f, dir, "instance-a", "pxm_enroll_a", lookup)
			require.NoError(t, s.ConnectInstance(context.Background()))
			id, _ := preparedAttempt(t, s, mode == "enable_unconfirmed")
			downloadedAttempt(t, s, id)
			p := plugin_mgr.Plugin{ID: "com.test.demo", Version: "1.2.0", Revision: "new-revision", InstallOrigin: s.Origin()}
			p.InstallOrigin.SHA256 = strings.Repeat("a", 64)
			if mode == "unchanged" {
				p.Revision = "old-revision"
			}
			inventory = []plugin_mgr.Plugin{p}
			s.CloseDistribution()
			s = distributionService(t, f, dir, "instance-a", "", lookup)
			require.NoError(t, s.RecoverDistribution(context.Background()))
			require.NoError(t, s.FlushDistribution(context.Background()))
			last := f.accepted[len(f.accepted)-1]
			if mode == "committed" || mode == "enable_unconfirmed" {
				require.Equal(t, "installed", last.Status)
				if mode == "enable_unconfirmed" {
					require.Equal(t, 1, s.Status().Instance.UnconfirmedInstalls)
				}
			} else {
				require.Equal(t, "failed", last.Status)
				require.Equal(t, "install", last.Stage)
			}
			require.Len(t, f.downloadKeys, 1)
		})
	}
}

func TestDistributionLostDownloadResponseRecoversSameAttemptWithoutInstall(t *testing.T) {
	f := newDistributionFixture(t)
	f.failDownloads = 1
	dir := t.TempDir()
	s := distributionService(t, f, dir, "instance-a", "pxm_enroll_a", nil)
	require.NoError(t, s.ConnectInstance(context.Background()))
	id, err := s.BeginInstall("com.test.demo", "1.2.0", "tenant-a", false, "")
	require.NoError(t, err)
	_, err = s.ResolveDownload(context.Background(), "com.test.demo", "1.2.0", "tenant-a", id)
	require.Error(t, err)
	require.NoError(t, s.RecordFailed(id, "POWERX_DOWNLOAD_FAILED"))
	s.CloseDistribution()
	s = distributionService(t, f, dir, "instance-a", "", nil)
	require.NoError(t, s.RecoverDistribution(context.Background()))
	require.NoError(t, s.FlushDistribution(context.Background()))
	require.Equal(t, []string{id, id}, f.downloadKeys)
	require.Len(t, f.tickets, 1)
	require.Len(t, f.accepted, 1)
	require.Equal(t, "failed", f.accepted[0].Status)
	require.Equal(t, "download", f.accepted[0].Stage)
}

func TestDistributionLifecycleHeartbeatAndShutdown(t *testing.T) {
	f := newDistributionFixture(t)
	s := distributionService(t, f, t.TempDir(), "instance-a", "pxm_enroll_a", func(context.Context) ([]plugin_mgr.Plugin, error) { return nil, nil })
	s.distribution.interval = 25 * time.Millisecond
	s.StartDistribution(context.Background())
	require.Eventually(t, func() bool { f.mu.Lock(); defer f.mu.Unlock(); return f.heartbeats >= 2 }, time.Second, 10*time.Millisecond)
	s.CloseDistribution()
	f.mu.Lock()
	count := f.heartbeats
	f.mu.Unlock()
	time.Sleep(60 * time.Millisecond)
	f.mu.Lock()
	require.Equal(t, count, f.heartbeats)
	f.mu.Unlock()
}

func TestDistributionRejectedReportIsRetainedWithoutRetryOrPayloadLeak(t *testing.T) {
	f := newDistributionFixture(t)
	s := distributionService(t, f, t.TempDir(), "instance-a", "pxm_enroll_a", nil)
	require.NoError(t, s.ConnectInstance(context.Background()))
	id, _ := preparedAttempt(t, s, false)
	downloadedAttempt(t, s, id)
	err := plugin_mgr.Wrap(plugin_mgr.CodeChecksumMismatch, errors.New("https://storage.invalid/package?secret-signature"), plugin_mgr.WithPath("private-path"))
	require.NoError(t, s.RecordOperationFailed(id, err, "POWERX_INSTALL_FAILED"))
	f.rejectReports = 409
	require.NoError(t, s.FlushDistribution(context.Background()))
	require.Equal(t, 2, s.Status().Instance.BlockedReports)
	require.NoError(t, s.FlushDistribution(context.Background()))
	require.Len(t, f.reports, 1)
	state := s.distribution.store.Snapshot()
	failure := state.Attempts[id].Events[1].Payload
	require.Equal(t, "POWERX_CHECKSUM_MISMATCH", failure.ErrorCode)
	require.NotContains(t, failure.Message, "secret-signature")
	require.NotContains(t, failure.Message, "private-path")
}

func TestDistributionWorkerRetriesPersistedEventsAtDeadline(t *testing.T) {
	f := newDistributionFixture(t)
	s := distributionService(t, f, t.TempDir(), "instance-a", "pxm_enroll_a", nil)
	require.NoError(t, s.ConnectInstance(context.Background()))
	id, _ := preparedAttempt(t, s, false)
	downloadedAttempt(t, s, id)
	installedAttempt(t, s, id)
	f.mu.Lock()
	f.failReports = 1
	f.mu.Unlock()
	started := time.Now()
	s.StartDistribution(context.Background())
	require.Eventually(t, func() bool { f.mu.Lock(); defer f.mu.Unlock(); return len(f.accepted) == 2 }, 9*time.Second, 25*time.Millisecond)
	s.CloseDistribution()
	require.GreaterOrEqual(t, time.Since(started), 4500*time.Millisecond)
	f.mu.Lock()
	defer f.mu.Unlock()
	require.Len(t, f.downloadKeys, 1)
	require.Len(t, f.reports, 3)
	require.Equal(t, string(f.reports[0]), string(f.reports[1]))
}
