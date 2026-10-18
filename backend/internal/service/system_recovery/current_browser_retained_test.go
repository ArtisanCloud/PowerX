package system_recovery

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/infra/opswatch"
	snapshots "github.com/ArtisanCloud/PowerX/internal/service/snapshot_ops"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// 只重新使用本验收创建并保留的夹具，避免调试 UI 时重复初始化整个数据库。
func TestCurrentRetainedBrowserFixture(t *testing.T) {
	root := os.Getenv("POWERX_CURRENT_BROWSER_REUSE_ROOT")
	if root == "" {
		t.Skip("explicit owned retained fixture only")
	}
	require.True(t, strings.HasPrefix(root, "/private/tmp/pxr-"))
	app := filepath.Join(root, "source")
	file := filepath.Join(app, "config.yaml")
	raw, err := os.ReadFile(file)
	require.NoError(t, err)
	var cfg config.Config
	require.NoError(t, yaml.Unmarshal(raw, &cfg))
	require.Equal(t, "dev", cfg.Deployment.Env)
	require.Equal(t, filepath.Join(app, "sockets"), cfg.Database.Host)
	require.Equal(t, "powerx_recovery", cfg.Database.UserName)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	n := &Native{Profile: Profile{Tools: Tools{PostgresBin: "/opt/homebrew/bin", Sandbox: "/usr/bin/sandbox-exec"}}}
	d := Deployment{ID: "source", Root: app, PostgresData: filepath.Join(app, "postgres"), SocketDirectory: cfg.Database.Host, PostgresPort: cfg.Database.Port}
	files := []string{}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 30*time.Second)
		defer stop()
		for i := len(files) - 1; i >= 0; i-- {
			require.NoError(t, stopOwned(cleanup, files[i]))
		}
	})
	require.NoError(t, n.startPostgres(ctx, d))
	files = append(files, filepath.Join(app, "processes/postgres.json"))
	redisRecord := filepath.Join(app, "processes/redis.json")
	_, err = startOwned(ctx, redisRecord, "/opt/homebrew/bin/redis-server", app, []string{"--bind", "127.0.0.1", "--port", fmt.Sprint(cfg.Cache.Port), "--dir", filepath.Join(app, "redis"), "--save", "", "--appendonly", "no"}, cleanEnvironment(app), []int{cfg.Cache.Port})
	require.NoError(t, err)
	files = append(files, redisRecord)
	coreRecord := filepath.Join(app, "processes/current-core.json")
	web := filepath.Join(app, "web-output")
	_, err = startOwned(ctx, coreRecord, filepath.Join(app, "powerx"), app, nil, cleanEnvironment(app, "POWERX_CONFIG="+file, "POWERX_WEB_OUTPUT="+web, "PLATFORM_CAPABILITIES_DIR="+filepath.Join(app, "platform_capabilities")), []int{cfg.Server.Port, cfg.Server.GRPC.Port})
	require.NoError(t, err)
	files = append(files, coreRecord)
	webRecord := filepath.Join(app, "processes/current-web.json")
	_, err = startOwned(ctx, webRecord, "/opt/homebrew/bin/node", app, []string{filepath.Join(web, "server/index.mjs")}, cleanEnvironment(app, "PORT=3489", "HOST=127.0.0.1", fmt.Sprintf("POWERX_BACKEND=http://127.0.0.1:%d", cfg.Server.Port)), []int{3489})
	require.NoError(t, err)
	files = append(files, webRecord)
	ready := os.Getenv("POWERX_CURRENT_BROWSER_READY_FILE")
	require.NotEmpty(t, ready)
	var owned processRecord
	require.NoError(t, snapshots.ReadCoordinatorJSON(coreRecord, &owned))
	sub, err := opswatch.Subscribe(ctx, map[string][]string{"done": {filepath.Dir(ready)}}, func(path string) bool { return filepath.Base(path) == filepath.Base(ready)+".done.json" })
	require.NoError(t, err)
	defer sub.Close()
	require.NoError(t, snapshots.WriteCoordinatorJSON(ready, map[string]any{"url": "http://127.0.0.1:3489", "core_url": fmt.Sprintf("http://127.0.0.1:%d", cfg.Server.Port), "core_pid": owned.PID, "snapshot_id": os.Getenv("POWERX_CURRENT_BROWSER_SNAPSHOT"), "key_file": filepath.Join(currentDirectory(cfg.SystemSnapshots.Directory, file), "recovery-auth/administrator.key"), "done_file": ready + ".done.json", "web_process_record": webRecord}))
	select {
	case <-ctx.Done():
		t.Fatal("browser fixture timeout")
	case <-sub.Changes:
	}
	var result struct {
		Passed   bool `json:"passed"`
		Finished bool `json:"finished"`
	}
	require.NoError(t, snapshots.ReadCoordinatorJSON(ready+".done.json", &result))
	require.True(t, result.Passed)
}
