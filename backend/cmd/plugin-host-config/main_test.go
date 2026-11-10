package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestPreviewDoesNotWriteOrExposeValues(t *testing.T) {
	root := t.TempDir()
	packageRoot := filepath.Join(root, "installed", "com.powerx.plugins.fixture", "1.0.0")
	require.NoError(t, os.MkdirAll(packageRoot, 0700))
	values := filepath.Join(packageRoot, "host-values.yaml")
	registry := filepath.Join(root, "registry.json")
	prefix := "powerx:fixture-env:plugin:com.powerx.plugins.fixture:"
	doc := map[string]any{"host": map[string]any{"environment_id": "fixture-env", "redis": map[string]any{"key_prefix": prefix}}, "marketplace": map[string]any{"license": map[string]any{"cache": map[string]any{"key_prefix": prefix + "license"}}}, "env": map[string]any{"PRIVATE_TOKEN": "do-not-print-this"}}
	raw, err := yaml.Marshal(doc)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(values, raw, 0600))
	spec := map[string]any{}
	for key, value := range doc {
		if key != "env" {
			spec[key] = value
		}
	}
	reg := manager.NewJSONRegistry(registry)
	require.NoError(t, reg.Put(context.Background(), manager.Descriptor{Manifest: plugin_mgr.Manifest{ID: "com.powerx.plugins.fixture", Version: "1.0.0"}, Paths: plugin_mgr.InstalledPaths{Root: packageRoot, HostValuesFile: values}, HostConfig: &plugin_mgr.HostConfig{ValuesFile: values, Values: map[string]string{"PRIVATE_TOKEN": "do-not-print-this"}, Spec: spec}}, plugin_mgr.StateInstalled))
	require.NoError(t, reg.Save(context.Background()))
	args := []string{"-registry", registry, "-plugin", "com.powerx.plugins.fixture"}
	var out bytes.Buffer
	require.NoError(t, run(args, &out))
	require.NotContains(t, out.String(), "do-not-print-this")
	require.NotContains(t, out.String(), prefix)
	var report manager.LegacyLicenseCachePreview
	require.NoError(t, json.Unmarshal(out.Bytes(), &report))
	require.Equal(t, "legacy_default_candidate", report.Status)
	require.Error(t, run(append(args, "-apply", "-expect", report.PlanDigest), &bytes.Buffer{}))
	after, err := os.ReadFile(values)
	require.NoError(t, err)
	require.Equal(t, raw, after)
}
