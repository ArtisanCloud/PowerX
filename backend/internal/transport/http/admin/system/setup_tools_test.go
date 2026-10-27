package system

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultSetupCommandsSelectMatchingTools(t *testing.T) {
	for _, layout := range []string{"repo source", "backend source", "repo source with binaries", "backend source with binaries", "release binaries"} {
		t.Run(layout, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "setup workspace")
			require.NoError(t, os.MkdirAll(root, 0o755))
			root, err := filepath.EvalSymlinks(root)
			require.NoError(t, err)
			workDir, sourceDir := root, root
			if strings.HasPrefix(layout, "repo source") {
				sourceDir = filepath.Join(root, "backend")
			}
			if layout != "release binaries" {
				writeSetupSourceFixture(t, sourceDir)
			}
			if strings.Contains(layout, "binaries") {
				writeSetupToolFixture(t, root, "database")
				writeSetupToolFixture(t, root, "platform_capability_seed")
			}
			binDir := t.TempDir()
			if layout != "release binaries" {
				writeSetupToolFixture(t, binDir, "go")
			}
			// 只允许执行临时工具桩，绝不运行真实的 Go、迁移或 seed。
			t.Setenv("PATH", binDir)
			t.Setenv("POWERX_SETUP_TEST_EXIT", "0")
			logPath := filepath.Join(t.TempDir(), "calls")
			t.Setenv("POWERX_SETUP_TEST_LOG", logPath)
			configPath := filepath.Join(root, "config dir", "runtime.yaml")
			t.Setenv("POWERX_CONFIG", configPath)

			for _, stage := range []string{"migrate", "seed"} {
				t.Run(stage, func(t *testing.T) {
					require.NoError(t, os.WriteFile(logPath, nil, 0o600))
					command := defaultSetupMigrateCmd()
					if stage == "seed" {
						command = defaultSetupSeedCmd()
					}
					cmd := exec.Command("/bin/sh", "-c", command)
					cmd.Dir = workDir
					output, err := cmd.CombinedOutput()
					require.NoError(t, err, string(output))
					calls, err := os.ReadFile(logPath)
					require.NoError(t, err)
					var expected []string
					if layout == "release binaries" {
						expected = setupToolCall(root, "database", stage, "-config", configPath)
						if stage == "seed" {
							expected = append(expected, setupToolCall(root, "platform_capability_seed", "-config", configPath)...)
						}
					} else {
						expected = setupToolCall(sourceDir, "go", "run", "./cmd/database", stage, "-config", configPath)
						if stage == "seed" {
							expected = append(expected, setupToolCall(sourceDir, "go", "run", "./cmd/platform_capability_seed", "-config", configPath)...)
						}
					}
					require.Equal(t, expected, strings.Split(strings.TrimSuffix(string(calls), "\n"), "\n"))
				})
			}
		})
	}
}

func TestDefaultSetupCommandsRejectMissingTools(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	for _, command := range []string{defaultSetupMigrateCmd(), defaultSetupSeedCmd()} {
		cmd := exec.Command("/bin/sh", "-c", command)
		cmd.Dir = t.TempDir()
		output, err := cmd.CombinedOutput()
		var exitErr *exec.ExitError
		require.ErrorAs(t, err, &exitErr)
		require.Equal(t, 127, exitErr.ExitCode())
		require.Contains(t, string(output), "not found")
	}
}

func TestDefaultSetupSeedStopsWhenDatabaseSeedFails(t *testing.T) {
	root := t.TempDir()
	writeSetupSourceFixture(t, root)
	writeSetupToolFixture(t, root, "database")
	writeSetupToolFixture(t, root, "platform_capability_seed")
	binDir := t.TempDir()
	writeSetupToolFixture(t, binDir, "go")
	t.Setenv("PATH", binDir)
	t.Setenv("POWERX_SETUP_TEST_EXIT", "23")
	logPath := filepath.Join(t.TempDir(), "calls")
	t.Setenv("POWERX_SETUP_TEST_LOG", logPath)
	cmd := exec.Command("/bin/sh", "-c", defaultSetupSeedCmd())
	cmd.Dir = root
	_, err := cmd.CombinedOutput()
	var exitErr *exec.ExitError
	require.ErrorAs(t, err, &exitErr)
	require.Equal(t, 23, exitErr.ExitCode())
	calls, err := os.ReadFile(logPath)
	require.NoError(t, err)
	require.Contains(t, string(calls), "./cmd/database\nseed\n")
	require.NotContains(t, string(calls), "./cmd/platform_capability_seed")
}

func TestRunSetupCommandsUsesBackendSourceWorkDir(t *testing.T) {
	root := t.TempDir()
	backend := filepath.Join(root, "backend")
	writeSetupSourceFixture(t, backend)
	backend, err := filepath.EvalSymlinks(backend)
	require.NoError(t, err)
	t.Chdir(root)
	t.Setenv("POWERX_LINKS_ROOT", filepath.Join(root, "unused links"))
	logPath := filepath.Join(root, "calls")
	t.Setenv("POWERX_SETUP_TEST_LOG", logPath)
	runtimePath := filepath.Join(backend, "etc", "config.yaml")
	// 验证正式命令执行器选择的目录及配置传递；不执行数据库工具。
	err = runSetupCommands(runtimePath, []setupProvisionCommand{{
		stage: "fixture",
		cmd:   `printf '%s\n' "$PWD" "$POWERX_CONFIG" > "$POWERX_SETUP_TEST_LOG"`,
	}})
	require.NoError(t, err)
	calls, err := os.ReadFile(logPath)
	require.NoError(t, err)
	require.Equal(t, backend+"\n"+runtimePath+"\n", string(calls))
}

func writeSetupSourceFixture(t *testing.T, dir string) {
	t.Helper()
	for _, tool := range []string{"database", "platform_capability_seed"} {
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "cmd", tool), 0o755))
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module setup.fixture\n"), 0o600))
}

func writeSetupToolFixture(t *testing.T, dir, name string) {
	t.Helper()
	script := fmt.Sprintf("#!/bin/sh\nprintf '%%s\\n' \"$PWD\" '%s' \"$#\" \"$@\" >> \"$POWERX_SETUP_TEST_LOG\"\nexit \"${POWERX_SETUP_TEST_EXIT:-0}\"\n", name)
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755))
}

func setupToolCall(dir, tool string, args ...string) []string {
	return append([]string{dir, tool, fmt.Sprint(len(args))}, args...)
}
