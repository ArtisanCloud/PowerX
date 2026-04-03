package systemintegration

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSetupPreflightSkipsDatabaseBootstrap(t *testing.T) {
	repoRoot := repoRootFromIntegrationTest(t)
	runtimeCfg := filepath.Join(t.TempDir(), "config.yaml")
	cfgText := `version: v2.0.2
server:
  port: 18098
  api_prefix: /api/v1
install:
  status: uninstalled
  lock_mode: strict
  allow_without_db: true
database:
  driver: postgres
  host: localhost
  port: 5432
  username: postgres
  password: postgres
  database: powerx
  ssl_mode: disable
cache:
  driver: redis
  host: localhost
  port: 6379
  db: 0
auth:
  jwt_secret: test-jwt
  issuer: powerx-auth
  audience_user: user
  audience_customer: customer
`
	require.NoError(t, os.WriteFile(runtimeCfg, []byte(cfgText), 0o644))

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", "run", "./cmd/app")
	cmd.Dir = filepath.Join(repoRoot, "backend")
	cmd.Env = append(os.Environ(),
		"POWERX_CONFIG="+runtimeCfg,
		"POWERX_BACKEND_PORT=18098",
		"POWERX_WEB_ADMIN_PORT=13098",
		"POWERX_ENV=prod",
	)
	out, err := cmd.CombinedOutput()
	logText := string(out)

	if ctx.Err() == context.DeadlineExceeded {
		// 进程被超时终止属预期（服务已启动并阻塞监听）。
	} else {
		require.NoError(t, err, logText)
	}

	require.Contains(t, logText, "setup-only preflight enabled: skip DB/Cache bootstrap")
	require.NotContains(t, logText, "初始化数据库失败")
	require.NotContains(t, strings.ToLower(logText), "failed to initialize database")
}

func repoRootFromIntegrationTest(t testing.TB) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Join(filepath.Dir(filename), "..", "..", "..", "..")
}
