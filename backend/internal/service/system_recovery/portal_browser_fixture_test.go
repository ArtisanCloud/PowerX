package system_recovery

// Explicit browser fixture for the real embedded recovery HTTP service. The
// driver is controlled; real PG/media/config replacement is a separate test.
import (
	"context"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	snapshots "github.com/ArtisanCloud/PowerX/internal/service/snapshot_ops"
	"github.com/stretchr/testify/require"
)

func TestRecoveryPortalBrowserFixture(t *testing.T) {
	if os.Getenv("POWERX_RECOVERY_PORTAL_BROWSER") != "1" {
		t.Skip("explicit recovery portal browser fixture only")
	}
	c, id := portalFixture(t)
	port, err := strconv.Atoi(os.Getenv("POWERX_RECOVERY_PORTAL_BROWSER_PORT"))
	require.NoError(t, err)
	require.Greater(t, port, 1024)
	listener, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
	require.NoError(t, err)
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	var businessReady atomic.Bool
	var mu sync.Mutex
	var release chan struct{}
	var releaseOnce sync.Once
	reset := func() {
		t.Helper()
		mu.Lock()
		defer mu.Unlock()
		jobs, err := c.Store.List()
		require.NoError(t, err)
		for _, job := range jobs {
			require.False(t, activeBrowserJob(job))
			require.NoError(t, os.Remove(filepath.Join(c.Store.Directory, "jobs", job.ID+".json")))
		}
		businessReady.Store(false)
		release = make(chan struct{})
		releaseOnce = sync.Once{}
		require.NoError(t, c.saveActive(Active{Generation: 1, Deployment: c.Initial}))
	}
	reset()
	c.Checkpoint = func(phase string) error {
		if phase == "maintenance" {
			mu.Lock()
			wait := release
			mu.Unlock()
			select {
			case <-wait:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		if phase == "complete" {
			businessReady.Store(true)
		}
		return nil
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/__fixture/reset":
			if r.Method != "POST" {
				w.WriteHeader(405)
				return
			}
			reset()
			w.WriteHeader(204)
			return
		case "/__fixture/continue":
			if r.Method != "POST" {
				w.WriteHeader(405)
				return
			}
			mu.Lock()
			releaseOnce.Do(func() { close(release) })
			mu.Unlock()
			w.WriteHeader(204)
			return
		case "/ops/backup":
			if businessReady.Load() {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				_, _ = w.Write([]byte(`<!doctype html><title>PowerX business fixture</title><p id="business-ready">业务恢复完成</p>`))
				return
			}
		}
		c.gateway(w, r)
	})}
	defer server.Close()
	done := make(chan error, 1)
	go func() { done <- c.Run(ctx) }()
	go func() { _ = server.Serve(listener) }()
	record := map[string]any{"owner": "recovery-portal-browser-fixture", "pid": os.Getpid(), "port": port, "cwd": func() string { v, _ := os.Getwd(); return v }(), "command": os.Args, "started_at": time.Now().UTC(), "key_file": c.recoveryKeyFile(), "snapshot_id": id}
	file := os.Getenv("POWERX_RECOVERY_PORTAL_BROWSER_RECORD")
	require.NotEmpty(t, file)
	require.NoError(t, os.MkdirAll(filepath.Dir(file), 0700))
	require.NoError(t, snapshots.WriteCoordinatorJSON(file, record))
	<-ctx.Done()
	_ = server.Close()
	err = <-done
	require.ErrorIs(t, err, context.Canceled)
	record["stopped_at"] = time.Now().UTC()
	require.NoError(t, snapshots.WriteCoordinatorJSON(file, record))
	// The temporary key and sessions are removed by Go's test cleanup.
}
func activeBrowserJob(job snapshots.ManagedJob) bool {
	return job.Status == "running" || job.Status == "queued"
}
