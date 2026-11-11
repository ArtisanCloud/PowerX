package manager

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager/router"
	"github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager/supervisor"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// 子进程仅为本测试提供回环健康端点；所有 PID/端口由父测试记录并停止。
func TestEnableCancellationProcessFixture(t *testing.T) {
	mode := os.Getenv("PX_TEST_ENABLE_FIXTURE")
	if mode == "" {
		return
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if mode == "admin" {
			w.WriteHeader(503)
		} else {
			w.WriteHeader(200)
		}
	})
	if err := http.ListenAndServe("127.0.0.1:"+os.Getenv("PORT"), mux); err != nil {
		t.Fatal(err)
	}
}

func cancellationManager(t *testing.T, withAdmin bool) (*managerImpl, string) {
	t.Helper()
	m, root := newCredentialInstallManager(t)
	m.opts.RuntimeCredential = func(context.Context, string) (*PluginRuntimeCredential, error) { return testInstallCredential(), nil }
	m.opts.CoreConfig.Server.Port = 8077
	src := writeReplaceTestPlugin(t, filepath.Join(root, "source"), "fixture")
	p, err := m.InstallFromFile(context.Background(), src, plugin_mgr.InstallOptions{})
	require.NoError(t, err)
	reg := m.opts.Registry.(*JSONRegistry)
	desc := Descriptor{Manifest: plugin_mgr.Manifest{ID: p.ID, Version: p.Version, Runtime: p.Runtime, Frontend: p.Frontend}, Paths: p.Paths, HostConfig: p.HostConfig, Revision: p.Revision}
	exe, err := os.Executable()
	require.NoError(t, err)
	desc.Paths.Entry = exe
	desc.Manifest.Runtime.Kind = plugin_mgr.RuntimeKindProcess
	desc.Manifest.Runtime.Args = []string{"-test.run=^TestEnableCancellationProcessFixture$"}
	desc.Manifest.Runtime.Env = map[string]string{"PX_TEST_ENABLE_FIXTURE": "api"}
	desc.Manifest.Runtime.Health.HTTPPath = "/healthz"
	desc.Manifest.Runtime.Health.Interval = "500ms"
	desc.Manifest.Runtime.Health.Timeout = "1s"
	if withAdmin {
		desc.Manifest.Frontend.Admin.Kind = plugin_mgr.FrontendKindProcess
		// 沿用真实类型，通过 YAML 清单装配 process 字段。
		raw := []byte("kind: process\nprocess:\n  entry: " + exe + "\n  args: ['-test.run=^TestEnableCancellationProcessFixture$']\n  env:\n    PX_TEST_ENABLE_FIXTURE: admin\n  health:\n    http: /healthz\n    interval: 10ms\n    timeout: 100ms\n")
		require.NoError(t, yaml.Unmarshal(raw, &desc.Manifest.Frontend.Admin))
	}
	require.NoError(t, reg.Put(context.Background(), desc, plugin_mgr.StateInstalled))
	require.NoError(t, reg.Save(context.Background()))
	m.sup = supervisor.New()
	m.http = router.NewDynamicRouter("/_p", gin.New())
	t.Cleanup(func() { require.NoError(t, m.sup.Stop(p.ID+"_admin")); require.NoError(t, m.sup.Stop(p.ID)) })
	return m, p.ID
}

func TestEnableCancellationAfterBackendStartStopsNewProcess(t *testing.T) {
	m, id := cancellationManager(t, false)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	port := 0
	m.opts.PostEnablePlugin = func(context.Context, plugin_mgr.Plugin, string) error {
		info, ok := m.sup.Status(id)
		require.True(t, ok)
		port = info.Port
		cwd, _ := os.Getwd()
		t.Logf("owned fixture pid=%d command=test-binary cwd=%s port=%d", info.PID, cwd, port)
		cancel()
		return nil
	}
	err := m.Enable(ctx, id)
	require.ErrorIs(t, err, context.Canceled)
	_, ok := m.sup.Status(id)
	require.False(t, ok)
	p, found := m.opts.Registry.Get(context.Background(), id)
	require.True(t, found)
	require.NotEqual(t, plugin_mgr.StateEnabled, p.State)
	conn, e := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 100*time.Millisecond)
	if conn != nil {
		conn.Close()
	}
	require.Error(t, e, "owned listener must be released")
}

func TestEnableCancellationDuringAdminProbeDoesNotFallbackToEnabled(t *testing.T) {
	m, id := cancellationManager(t, true)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		deadline := time.NewTimer(5 * time.Second)
		defer deadline.Stop()
		tick := time.NewTicker(5 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-deadline.C:
				cancel()
				return
			case <-tick.C:
				if info, ok := m.sup.Status(id + "_admin"); ok && info.PID > 0 {
					cancel()
					return
				}
			}
		}
	}()
	err := m.Enable(ctx, id)
	<-done
	require.True(t, errors.Is(err, context.Canceled), "%v", err)
	for _, pid := range []string{id, id + "_admin"} {
		_, ok := m.sup.Status(pid)
		require.False(t, ok, "owned process must be removed")
	}
	p, ok := m.opts.Registry.Get(context.Background(), id)
	require.True(t, ok)
	require.NotEqual(t, plugin_mgr.StateEnabled, p.State)
}

func TestAdminStaticFallbackRequiresEntryPage(t *testing.T) {
	p := plugin_mgr.Plugin{Paths: plugin_mgr.InstalledPaths{FrontendAdminDir: t.TempDir()}}
	require.False(t, adminStaticFallbackReady(p))
	require.NoError(t, os.WriteFile(filepath.Join(p.Paths.FrontendAdminDir, "index.html"), []byte("fixture"), 0600))
	require.True(t, adminStaticFallbackReady(p))
}

func TestCommittedFirstInstallRetainsArtifactsWhenActivationFails(t *testing.T) {
	m, root := newCredentialInstallManager(t)
	m.opts.CoreConfig.Server.Port = 8077
	m.opts.RuntimeCredential = func(context.Context, string) (*PluginRuntimeCredential, error) { return testInstallCredential(), nil }
	src := writeReplaceTestPlugin(t, filepath.Join(root, "source"), "fixture")
	p, err := m.InstallFromFile(context.Background(), src, plugin_mgr.InstallOptions{AutoEnable: true})
	var pending *plugin_mgr.InstallCompletionError
	require.ErrorAs(t, err, &pending)
	require.Equal(t, p.ID, pending.PluginID)
	require.Equal(t, p.Version, pending.Version)
	require.NotEmpty(t, pending.Revision)
	current, ok := m.opts.Registry.Get(context.Background(), p.ID)
	require.True(t, ok)
	require.NotEqual(t, plugin_mgr.StateEnabled, current.State)
	_, err = os.Stat(current.Paths.Entry)
	require.NoError(t, err, "committed package must be retained for explicit resume")
}

func TestCancelledEnableDoesNotStopPreviouslyHealthyRuntime(t *testing.T) {
	m, id := cancellationManager(t, false)
	require.NoError(t, m.Enable(context.Background(), id))
	before, ok := m.sup.Status(id)
	require.True(t, ok)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, m.Enable(ctx, id), context.Canceled)
	after, ok := m.sup.Status(id)
	require.True(t, ok)
	require.Equal(t, before.PID, after.PID)
	require.NoError(t, m.sup.Stop(id))
	conn, e := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(before.Port)), 100*time.Millisecond)
	if conn != nil {
		conn.Close()
	}
	require.Error(t, e)
}

func TestEnableExpectedRevisionIsCheckedInsideManagerMutationLock(t *testing.T) {
	m, id := cancellationManager(t, false)
	err := m.Enable(plugin_mgr.WithExpectedInstallation(context.Background(), "0.1.0", "outdated-revision"), id)
	require.True(t, plugin_mgr.IsCode(err, plugin_mgr.CodeConflict), "%v", err)
	_, running := m.sup.Status(id)
	require.False(t, running)
}
