package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDatabaseVersionReadsActualGoBuildRevision(t *testing.T) {
	info := &debug.BuildInfo{GoVersion: "go1.26.7", Settings: []debug.BuildSetting{
		{Key: "vcs.revision", Value: "actual-source-commit"},
		{Key: "vcs.modified", Value: "true"},
	}}
	got := databaseVersionInformation(info)
	require.Equal(t, "actual-source-commit", got.GitCommit)
	require.NotNil(t, got.SourceModified)
	require.True(t, *got.SourceModified)
	require.Contains(t, got.Commands, "prepare-plugin-runtime-credentials")
	unknown := databaseVersionInformation(nil)
	require.Nil(t, unknown.SourceModified)
}

func TestDatabaseInformationRecognizesVersionHelpAndCommands(t *testing.T) {
	var output bytes.Buffer
	handled, err := handleDatabaseInformation([]string{"--version"}, &output)
	require.NoError(t, err)
	require.True(t, handled)
	var info databaseVersion
	require.NoError(t, json.Unmarshal(output.Bytes(), &info))
	require.Equal(t, "database", info.Program)
	require.Contains(t, info.Commands, "prepare-plugin-runtime-credentials")
	for _, cmd := range databaseCommands {
		handled, err := handleDatabaseInformation([]string{cmd}, &output)
		require.NoError(t, err)
		require.False(t, handled)
	}
	handled, err = handleDatabaseInformation([]string{"unknown-command"}, &output)
	require.True(t, handled)
	require.ErrorContains(t, err, "Unknown command")
	handled, err = handleDatabaseInformation([]string{"--version", "migrate"}, &output)
	require.True(t, handled)
	require.ErrorContains(t, err, "no additional arguments")
}

func TestDatabaseVersionDoesNotLoadConfigurationOrConnectDatabase(t *testing.T) {
	if os.Getenv("POWERX_DATABASE_INFO_HELPER") == "1" {
		os.Args = []string{"database", "--version"}
		main()
		os.Exit(0)
	}
	binary, err := os.Executable()
	require.NoError(t, err)
	cmd := exec.Command(binary, "-test.run=^TestDatabaseVersionDoesNotLoadConfigurationOrConnectDatabase$")
	cmd.Dir = t.TempDir()
	cmd.Env = append(os.Environ(), "POWERX_DATABASE_INFO_HELPER=1", "POWERX_CONFIG="+filepath.Join(cmd.Dir, "missing-config.yaml"), "CORE_X_AUTH_JWT_SECRET=unsupported-test-value")
	raw, err := cmd.CombinedOutput()
	require.NoError(t, err, "output=%s", raw)
	var info databaseVersion
	require.NoError(t, json.Unmarshal(raw, &info), "version must return only JSON")
	require.Contains(t, info.Commands, "prepare-plugin-runtime-credentials")
}
