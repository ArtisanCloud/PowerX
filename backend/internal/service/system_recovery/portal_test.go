package system_recovery

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/config"
	snapshots "github.com/ArtisanCloud/PowerX/internal/service/snapshot_ops"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func portalFixture(t *testing.T) (*Coordinator, string) {
	t.Helper()
	root := t.TempDir()
	store, err := snapshots.NewCoordinatorStore(filepath.Join(root, "ops"), []snapshots.RecoveryTarget{{ID: "current", Mode: "formal", Environment: "prod", AllowOverwrite: true}})
	require.NoError(t, err)
	service := snapshots.NewService(context.Background(), snapshots.Config{Directory: filepath.Join(root, "snapshots")}, snapshots.Options{Coordinator: true})
	t.Cleanup(service.Close)
	id := uuid.NewString()
	driver := &testDriver{rollbackID: uuid.NewString()}
	c := &Coordinator{Store: store, SnapshotStore: service, Driver: driver, Initial: Deployment{ID: uuid.NewString(), Root: filepath.Join(root, "instance"), ConfigFile: filepath.Join(root, "instance/etc/config.yaml")}}
	_, err = c.Active()
	require.NoError(t, err)
	require.NoError(t, snapshots.WriteCoordinatorJSON(filepath.Join(root, "snapshots/snapshots", id+".json"), snapshots.Snapshot{ID: id, Status: "ready", RecoverySupported: true, CreatedAt: time.Now(), Version: "test"}))
	_, err = c.EnsureRecoveryAccess()
	require.NoError(t, err)
	require.NoError(t, snapshots.WriteCoordinatorJSON(filepath.Join(store.Directory, "heartbeat.json"), map[string]any{"at": time.Now()}))
	return c, id
}
func portalCall(c *Coordinator, method, path string, body any, cookie *http.Cookie, csrf string) *httptest.ResponseRecorder {
	var raw []byte
	if body != nil {
		raw, _ = json.Marshal(body)
	}
	r := httptest.NewRequest(method, path, bytes.NewReader(raw))
	if body != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	if cookie != nil {
		r.AddCookie(cookie)
	}
	if csrf != "" {
		r.Header.Set("X-PowerX-Recovery-CSRF", csrf)
	}
	r.Header.Set("Idempotency-Key", "d72379f5-7564-4ffb-af39-a7f25e02b10f")
	w := httptest.NewRecorder()
	c.gateway(w, r)
	return w
}
func portalLogin(t *testing.T, c *Coordinator) (*http.Cookie, string) {
	t.Helper()
	key, err := c.readRecoveryKey()
	require.NoError(t, err)
	w := portalCall(c, "POST", recoveryEntry+"session", map[string]string{"key": key}, nil, "")
	require.Equal(t, 200, w.Code, w.Body.String())
	cookies := w.Result().Cookies()
	require.Len(t, cookies, 1)
	require.True(t, cookies[0].HttpOnly)
	require.Equal(t, http.SameSiteStrictMode, cookies[0].SameSite)
	var session struct {
		CSRF string `json:"csrf"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &session))
	require.NotEmpty(t, session.CSRF)
	require.NotContains(t, w.Body.String(), key)
	require.NotContains(t, w.Body.String(), cookies[0].Value)
	return cookies[0], session.CSRF
}
func TestRecoveryPortalAvailableDuringMaintenanceAndCoreStopped(t *testing.T) {
	c, _ := portalFixture(t)
	active, err := c.Active()
	require.NoError(t, err)
	active.Maintenance = true
	require.NoError(t, c.saveActive(active))
	for _, path := range []string{recoveryEntry, "/ops/backup", "/"} {
		r := httptest.NewRequest("GET", path, nil)
		r.Header.Set("Accept", "text/html")
		w := httptest.NewRecorder()
		c.gateway(w, r)
		require.Equal(t, 200, w.Code)
		require.Contains(t, w.Body.String(), "恢复控制台")
		require.Contains(t, w.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'")
	}
	for _, path := range []string{"app.js", "style.css"} {
		w := portalCall(c, "GET", recoveryEntry+path, nil, nil, "")
		require.Equal(t, 200, w.Code)
		require.NotEmpty(t, w.Body.String())
	}
	require.Equal(t, 401, portalCall(c, "GET", recoveryEntry+"catalog", nil, nil, "").Code)
	cookie, csrf := portalLogin(t, c)
	w := portalCall(c, "GET", recoveryEntry+"catalog", nil, cookie, "")
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `"maintenance":true`)
	require.Contains(t, w.Body.String(), `"business_online":false`)
	// A new coordinator process can read the session outside the restored tree.
	restarted := &Coordinator{Store: c.Store, SnapshotStore: c.SnapshotStore, Driver: c.Driver, Initial: c.Initial}
	require.Equal(t, 200, portalCall(restarted, "GET", recoveryEntry+"catalog", nil, cookie, "").Code)
	require.Equal(t, 200, portalCall(restarted, "DELETE", recoveryEntry+"session", nil, cookie, csrf).Code)
	require.Equal(t, 401, portalCall(c, "GET", recoveryEntry+"catalog", nil, cookie, "").Code)
}
func TestRecoveryPortalRequiresAuthenticationCSRFAndPinnedRestore(t *testing.T) {
	c, id := portalFixture(t)
	cookie, csrf := portalLogin(t, c)
	body := map[string]any{"snapshot_id": id, "confirmation": id, "generation": 1}
	require.Equal(t, 401, portalCall(c, "POST", recoveryEntry+"restore", body, nil, csrf).Code)
	require.Equal(t, 403, portalCall(c, "POST", recoveryEntry+"restore", body, cookie, "").Code)
	r := httptest.NewRequest("POST", recoveryEntry+"restore", strings.NewReader(`{}`))
	r.AddCookie(cookie)
	r.Header.Set("Origin", "https://another.invalid")
	r.Header.Set("X-PowerX-Recovery-CSRF", csrf)
	w := httptest.NewRecorder()
	c.gateway(w, r)
	require.Equal(t, 403, w.Code)
	require.Equal(t, 400, portalCall(c, "POST", recoveryEntry+"restore", map[string]any{"snapshot_id": id, "confirmation": id, "generation": 1, "dsn": "arbitrary"}, cookie, csrf).Code)
	require.Equal(t, 409, portalCall(c, "POST", recoveryEntry+"restore", map[string]any{"snapshot_id": id, "confirmation": id, "generation": 99}, cookie, csrf).Code)
	w = portalCall(c, "POST", recoveryEntry+"restore", body, cookie, csrf)
	require.Equal(t, 202, w.Code, w.Body.String())
	var receipt struct {
		Recovery snapshots.ManagedJob `json:"recovery"`
		Token    string               `json:"progress_token"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &receipt))
	require.Equal(t, "recovery-administrator", receipt.Recovery.Operator)
	// Replaying exactly the same command yields the same task, not a second restore.
	replay := portalCall(c, "POST", recoveryEntry+"restore", body, cookie, csrf)
	require.Equal(t, 202, replay.Code)
	require.Contains(t, replay.Body.String(), receipt.Recovery.ID)
	r = httptest.NewRequest("POST", recoveryEntry+"restore", strings.NewReader(`{}`))
	r.Header.Set("Authorization", "Bearer "+receipt.Token)
	w = httptest.NewRecorder()
	c.gateway(w, r)
	require.Equal(t, 401, w.Code)
	require.Equal(t, 400, portalCall(c, "GET", recoveryEntry+"catalog?token="+receipt.Token, nil, cookie, "").Code)
	// The progress ticket remains valid only for the one read-only task endpoint.
	r = httptest.NewRequest("GET", recoveryEntry+"jobs/"+receipt.Recovery.ID, nil)
	r.Header.Set("Authorization", "Bearer "+receipt.Token)
	w = httptest.NewRecorder()
	c.gateway(w, r)
	require.Equal(t, 200, w.Code)
	require.NoError(t, c.RunOnce(context.Background()))
	job, err := c.Store.Get(receipt.Recovery.ID)
	require.NoError(t, err)
	require.Equal(t, "complete", job.Status, job.Error)
}
func TestRecoveryPortalKeyAndSessionArePrivateExpiringAndRevocable(t *testing.T) {
	c, _ := portalFixture(t)
	cookie, _ := portalLogin(t, c)
	info, err := os.Stat(c.recoveryKeyFile())
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0600), info.Mode().Perm())
	r := httptest.NewRequest("GET", recoveryEntry+"catalog", nil)
	r.AddCookie(cookie)
	session, err := c.readRecoverySession(r)
	require.NoError(t, err)
	session.ExpiresAt = time.Now().Add(-time.Minute)
	require.NoError(t, snapshots.WriteCoordinatorJSON(c.sessionFile(session.ID), session))
	require.Equal(t, 401, portalCall(c, "GET", recoveryEntry+"catalog", nil, cookie, "").Code)
	require.Equal(t, 401, portalCall(c, "POST", recoveryEntry+"session", map[string]string{"key": "a-public-default"}, nil, "").Code)
	require.NoError(t, os.Chmod(c.recoveryKeyFile(), 0644))
	_, err = c.EnsureRecoveryAccess()
	require.Error(t, err)
}
func TestRecoveryPortalAcceptsLiveRootThenWorksWithoutCore(t *testing.T) {
	c, _ := portalFixture(t)
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer real-user-session" {
			w.WriteHeader(401)
			return
		}
		_, _ = w.Write([]byte(`{"data":{"is_root":true,"user":{"id":7,"is_root":true}}}`))
	}))
	defer core.Close()
	_, portText, _ := strings.Cut(core.URL, "127.0.0.1:")
	var port int
	_, err := fmt.Sscanf(portText, "%d", &port)
	require.NoError(t, err)
	cfg := config.GetDefaults()
	cfg.Server.Port = port
	cfg.Server.APIPrefix = "/api/v1"
	c.Initial.SourceManaged = true
	require.NoError(t, writeInstallationConfig(c.Initial.ConfigFile, cfg))
	require.NoError(t, c.saveActive(Active{Generation: 1, Deployment: c.Initial}))
	r := httptest.NewRequest("GET", "/api/v1/admin/ops/backup/system-snapshots/recovery", nil)
	r.Header.Set("Authorization", "Bearer real-user-session")
	w := httptest.NewRecorder()
	c.gateway(w, r)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `"recovery_entry":"/__powerx_recovery/"`)
	require.Len(t, w.Result().Cookies(), 1)
	cookie := w.Result().Cookies()[0]
	core.Close()
	require.Equal(t, 200, portalCall(c, "GET", recoveryEntry+"catalog", nil, cookie, "").Code)
	portalLogin(t, c) // first login with recovery key also works after Core stops
}

type brokenStartupDriver struct{ *testDriver }

func (d *brokenStartupDriver) EnsureActive(context.Context, Active) error {
	return errors.New("fixture Core cannot start")
}
func TestRecoveryCoordinatorRemainsAvailableWhenBusinessCannotStart(t *testing.T) {
	c, _ := portalFixture(t)
	c.Driver = &brokenStartupDriver{c.Driver.(*testDriver)}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- c.Run(ctx) }()
	defer cancel()
	require.Eventually(t, func() bool {
		_, err := os.Stat(filepath.Join(c.Store.Directory, "startup-error.json"))
		return err == nil
	}, time.Second, 10*time.Millisecond)
	cookie, _ := portalLogin(t, c)
	w := portalCall(c, "GET", recoveryEntry+"catalog", nil, cookie, "")
	require.Equal(t, 200, w.Code)
	require.Contains(t, w.Body.String(), "fixture Core cannot start")
	cancel()
	require.ErrorIs(t, <-done, context.Canceled)
}

func TestRecoveryPortalRetainsReadOnlyEntranceAfterExecutorFailure(t *testing.T) {
	c, id := portalFixture(t)
	require.NoError(t, os.WriteFile(filepath.Join(c.Store.Directory, "jobs", uuid.NewString()+".json"), []byte("not-json"), 0600))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- c.runWithStatus(ctx, "127.0.0.1:0") }()
	require.Eventually(t, func() bool { return c.executorProblem() != "" }, 2*time.Second, 10*time.Millisecond)
	select {
	case err := <-done:
		t.Fatalf("recovery entrance exited with executor: %v", err)
	default:
	}
	w := portalCall(c, "GET", recoveryEntry, nil, nil, "")
	require.Equal(t, 200, w.Code)
	require.Contains(t, w.Body.String(), "恢复控制台")
	cookie, csrf := portalLogin(t, c)
	require.Equal(t, 503, portalCall(c, "POST", recoveryEntry+"restore", map[string]any{"snapshot_id": id, "confirmation": id, "generation": 1}, cookie, csrf).Code)
	cancel()
	require.ErrorIs(t, <-done, context.Canceled)
}

func TestRecoveryPortalResumeFencesGenerationAndConcurrentWork(t *testing.T) {
	c, id := portalFixture(t)
	cookie, csrf := portalLogin(t, c)
	previous := snapshots.ManagedJob{ID: uuid.NewString(), Kind: "restore", SnapshotID: id, CandidateID: c.Initial.ID, Status: "attention_required", Phase: "maintenance", TargetID: "current"}
	require.NoError(t, c.Store.Save(previous))
	require.NoError(t, c.saveActive(Active{Generation: 2, Deployment: c.Initial, Maintenance: true, JobID: previous.ID}))
	body := map[string]any{"job_id": previous.ID, "confirmation": previous.ID, "generation": 1}
	require.Equal(t, 409, portalCall(c, "POST", recoveryEntry+"resume", body, cookie, csrf).Code)
	body["generation"] = 2
	w := portalCall(c, "POST", recoveryEntry+"resume", body, cookie, csrf)
	require.Equal(t, 202, w.Code, w.Body.String())
	require.Equal(t, 202, portalCall(c, "POST", recoveryEntry+"resume", body, cookie, csrf).Code)
	_, err := c.Store.EnqueueResume(recoveryContext(context.Background(), "recovery-administrator"), previous.ID, uuid.NewString(), 2)
	require.Error(t, err)
}
