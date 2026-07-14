package metadata_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	metasvc "github.com/ArtisanCloud/PowerX/internal/service/metadata"
	"github.com/google/uuid"
)

type recordingMetadataInvoker struct {
	calls []metadataCall
}

type metadataCall struct {
	capabilityID string
	payload      map[string]any
}

func (r *recordingMetadataInvoker) InvokeMetadataCapability(_ context.Context, capabilityID string, payload map[string]any) (map[string]any, error) {
	r.calls = append(r.calls, metadataCall{capabilityID: capabilityID, payload: payload})
	return map[string]any{"payload": map[string]any{"ok": true}}, nil
}

func TestPluginMetadataClientDelegatedCapabilityPaths(t *testing.T) {
	invoker := &recordingMetadataInvoker{}
	client, err := metasvc.NewPluginMetadataClient(metasvc.PluginMetadataClientOptions{
		Mode:    "delegated",
		Invoker: invoker,
	})
	if err != nil {
		t.Fatalf("new delegated client: %v", err)
	}
	ctx := context.Background()
	if _, err := client.ResolveResourceType(ctx, "metadata.demo_resource"); err != nil {
		t.Fatalf("resolve resource type: %v", err)
	}
	if _, err := client.ListDictionaryItems(ctx, uuid.New().String(), map[string]any{"locale": "zh-CN"}); err != nil {
		t.Fatalf("list dictionary items: %v", err)
	}
	if _, err := client.ListTaxonomyNodes(ctx, uuid.New().String(), map[string]any{"locale": "zh-CN"}); err != nil {
		t.Fatalf("list taxonomy nodes: %v", err)
	}
	if _, err := client.ListTags(ctx, map[string]any{"resource_type": "metadata.demo_resource"}); err != nil {
		t.Fatalf("list tags: %v", err)
	}
	if _, err := client.ReplaceTagBindings(ctx, "metadata.demo_resource", uuid.New().String(), []string{uuid.New().String()}); err != nil {
		t.Fatalf("replace tag bindings: %v", err)
	}
	want := []string{
		metasvc.CapabilityMetadataResourceTypeRead,
		metasvc.CapabilityMetadataDictionaryRead,
		metasvc.CapabilityMetadataTaxonomyRead,
		metasvc.CapabilityMetadataTagRead,
		metasvc.CapabilityMetadataTagManage,
	}
	if len(invoker.calls) != len(want) {
		t.Fatalf("expected %d calls, got %+v", len(want), invoker.calls)
	}
	for i := range want {
		if invoker.calls[i].capabilityID != want[i] {
			t.Fatalf("call %d capability mismatch: got %s want %s", i, invoker.calls[i].capabilityID, want[i])
		}
		if invoker.calls[i].payload["method"] == "" || invoker.calls[i].payload["endpoint"] == "" {
			t.Fatalf("call %d missing REST payload: %+v", i, invoker.calls[i].payload)
		}
	}
}

func TestPluginMetadataClientLocalRequiresCanonicalSeed(t *testing.T) {
	missingPath := filepath.Join(t.TempDir(), "missing.yaml")
	if _, err := metasvc.NewPluginMetadataClient(metasvc.PluginMetadataClientOptions{Mode: "local", SeedPath: missingPath}); err == nil {
		t.Fatalf("expected missing local seed to fail")
	}

	emptyPath := writePluginSeedFile(t, `
version: 1
module: corex.metadata
dictionaries: []
taxonomies: []
resource_types: []
tags: []
`)
	if _, err := metasvc.NewPluginMetadataClient(metasvc.PluginMetadataClientOptions{Mode: "local", SeedPath: emptyPath}); err == nil {
		t.Fatalf("expected empty canonical seed to fail")
	}

	validPath := writePluginSeedFile(t, `
version: 1
module: corex.metadata
dictionaries:
  - namespace: corex.metadata.status
    name_i18n:
      zh-CN: 状态
    items:
      - code: enabled
        label_i18n:
          zh-CN: 启用
resource_types:
  - resource_type: metadata.demo_resource
    name_i18n:
      zh-CN: 演示资源
tags:
  - namespace: corex.metadata.demo
    resource_type: metadata.demo_resource
    code: sample
    label_i18n:
      zh-CN: 示例
`)
	client, err := metasvc.NewPluginMetadataClient(metasvc.PluginMetadataClientOptions{Mode: "local", SeedPath: validPath})
	if err != nil {
		t.Fatalf("new local client: %v", err)
	}
	seed, err := client.LocalSeed()
	if err != nil {
		t.Fatalf("local seed: %v", err)
	}
	if len(seed.ResourceTypes) != 1 || len(seed.Dictionaries) != 1 || len(seed.Tags) != 1 {
		t.Fatalf("unexpected local seed: %+v", seed)
	}
}

func writePluginSeedFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "seed.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write seed: %v", err)
	}
	return path
}
