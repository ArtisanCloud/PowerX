package systemintegration

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
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
deployment:
  env: dev
server:
  host: 127.0.0.1
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
  jwt_secret: preflight-test-key-at-least-32-bytes
  issuer: powerx-auth
  audience_user: user
  audience_customer: customer
log:
  level: info
  console: true
`
	key := make([]byte, 32)
	_, err := rand.Read(key)
	require.NoError(t, err)
	cfgText = strings.Replace(cfgText, "server:\n", "server:\n  secret_key: "+base64.StdEncoding.EncodeToString(key)+"\n", 1)
	require.NoError(t, os.WriteFile(runtimeCfg, []byte(cfgText), 0o644))

	// 编译不属于服务就绪等待；go run 超时既会误判，也可能遗留其 app 子进程。
	buildCtx, buildCancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer buildCancel()
	binary := filepath.Join(t.TempDir(), "powerx-setup-test")
	build := exec.CommandContext(buildCtx, "go", "build", "-o", binary, "./cmd/app")
	build.Dir = filepath.Join(repoRoot, "backend")
	buildOutput, err := build.CombinedOutput()
	require.NoError(t, err, string(buildOutput))
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := listener.Addr().(*net.TCPAddr).Port
	require.NoError(t, listener.Close())

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary)
	// 服务会生成 OpenAPI 文件；把运行目录隔离在临时目录，不能写回仓库。
	cmd.Dir = filepath.Dir(runtimeCfg)
	cmd.Env = append(os.Environ(),
		"POWERX_CONFIG="+runtimeCfg,
		fmt.Sprintf("POWERX_BACKEND_PORT=%d", port),
		"POWERX_WEB_ADMIN_PORT=13098",
		"POWERX_ENV=prod",
	)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	require.NoError(t, cmd.Start())
	t.Logf("test-owned service PID=%d command=%s cwd=%s port=%d", cmd.Process.Pid, binary, cmd.Dir, port)
	t.Cleanup(func() {
		cancel()
		if cmd.ProcessState == nil {
			_ = cmd.Wait()
		}
	})
	client := &http.Client{Timeout: time.Second}
	ready := false
	for ctx.Err() == nil {
		response, requestErr := client.Get(fmt.Sprintf("http://127.0.0.1:%d/api/v1/admin/setup/status", port))
		if requestErr == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				ready = true
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	cancel()
	_ = cmd.Wait()
	logText := output.String()
	connection, portErr := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 100*time.Millisecond)
	if connection != nil {
		_ = connection.Close()
	}
	require.Error(t, portErr, "test-owned service listener must be released")
	require.True(t, ready, logText)

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
