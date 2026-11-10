package productinfo

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRuntimeParserSharedGoldenAndStrictSchema(t *testing.T) {
	raw, err := os.ReadFile("../../../contracts/deployment-fingerprint/v1/golden-manifests.json")
	require.NoError(t, err)
	var fixtures struct {
		Vectors []struct {
			Name     string
			Manifest json.RawMessage
			BuildID  string `json:"build_id"`
		}
	}
	require.NoError(t, json.Unmarshal(raw, &fixtures))
	for _, v := range fixtures.Vectors {
		t.Run(v.Name, func(t *testing.T) {
			m, err := ParseManifest(v.Manifest)
			require.NoError(t, err)
			require.Equal(t, v.BuildID, m.BuildID)
		})
	}
	schema, err := os.ReadFile("../../../contracts/deployment-fingerprint/v1/protocol.schema.json")
	require.NoError(t, err)
	require.Equal(t, schema, manifestSchema)
	var body map[string]any
	require.NoError(t, json.Unmarshal(fixtures.Vectors[0].Manifest, &body))
	body["artifacts"].([]any)[0].(map[string]any)["size"] = nil
	raw, err = json.Marshal(body)
	require.NoError(t, err)
	_, err = ParseManifest(raw)
	require.Error(t, err)
}

func TestSidecarSelfCheckAndFailureStates(t *testing.T) {
	file := filepath.Join(t.TempDir(), "powerx")
	require.NoError(t, os.WriteFile(file, []byte("final executable"), 0600))
	require.Equal(t, "development", LoadAt(file, true).State)
	require.Equal(t, "missing", LoadAt(file, false).State)
	m, err := GenerateBackend(file, "1.2.3", "revision", "2026-10-23T00:00:00Z", runtime.GOOS, runtime.GOARCH)
	require.NoError(t, err)
	raw, err := json.Marshal(m)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(file+".build.json", raw, 0600))
	info := LoadAt(file, true)
	require.Equal(t, "available", info.State)
	require.Equal(t, m.BuildID, info.BuildInfo.BuildID)
	require.NoError(t, os.WriteFile(file, []byte("modified executable"), 0600))
	require.Equal(t, "digest_mismatch", LoadAt(file, true).State)
	require.NoError(t, os.WriteFile(file+".build.json", []byte(`{"schema":null}`), 0600))
	require.Equal(t, "invalid", LoadAt(file, true).State)
}

func TestDevelopmentFingerprintUsesActualBinaryWithoutSidecar(t *testing.T) {
	file := filepath.Join(t.TempDir(), "app")
	require.NoError(t, os.WriteFile(file, []byte("development executable"), 0600))
	first := LoadAt(file, true)
	require.Equal(t, "development", first.State)
	require.NotNil(t, first.BuildInfo)
	expected, err := GenerateBackend(file, "development", SourceCommit, "", runtime.GOOS, runtime.GOARCH)
	require.NoError(t, err)
	require.Equal(t, expected.BuildID, first.BuildInfo.BuildID)
	require.Equal(t, first, LoadAt(file, true), "the same executable must keep the same build fingerprint")
	_, err = os.Stat(file + ".build.json")
	require.True(t, os.IsNotExist(err), "startup must not create or modify a release sidecar")
	require.Equal(t, "missing", LoadAt(file, false).State, "a release missing its sidecar cannot fall back to a development claim")
	require.NoError(t, os.WriteFile(file, []byte("updated development executable"), 0600))
	updated := LoadAt(file, true)
	require.NotEqual(t, first.BuildInfo.BuildID, updated.BuildInfo.BuildID)
	require.NoError(t, os.WriteFile(file+".build.json", []byte(`{"schema":null}`), 0600))
	require.Equal(t, "invalid", LoadAt(file, true).State, "an invalid sidecar must not be hidden by automatic hashing")
}
