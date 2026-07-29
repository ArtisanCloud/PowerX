package workflow

import (
	"context"
	"testing"
)

type testCatalogProvider struct {
	items []NodeCatalogEnrichment
}

func (p testCatalogProvider) ListNodeCatalogEnrichments(context.Context) ([]NodeCatalogEnrichment, error) {
	return p.items, nil
}

func TestNodeCatalogServiceListsRegisteredAdaptersWithEnrichment(t *testing.T) {
	registry := NewNodeAdapterRegistry()
	if err := RegisterWorkflowNodeAdapters(registry, WorkflowNodeAdapterDeps{}); err != nil {
		t.Fatalf("register adapters: %v", err)
	}
	required := true
	service := NewNodeCatalogService(registry, testCatalogProvider{items: []NodeCatalogEnrichment{
		{
			NodeKind:             " CAPABILITY.INVOKE ",
			RequiredPermissions:  []string{"workflow.capability:invoke"},
			RequiredCapabilities: []string{"com.corex.demo"},
			IdempotencyRequired:  &required,
			SourceStatus:         "missing_dependency",
		},
	}})

	items, err := service.List(context.Background())
	if err != nil {
		t.Fatalf("list catalog: %v", err)
	}
	if len(items) != 12 {
		t.Fatalf("expected 12 items, got %d", len(items))
	}
	var capability NodeCatalogItem
	for _, item := range items {
		if item.NodeKind == "capability.invoke" {
			capability = item
			break
		}
	}
	if capability.NodeKind == "" {
		t.Fatal("capability.invoke missing")
	}
	if capability.StepType != "system" || capability.SourceStatus != "missing_dependency" || !capability.IdempotencyRequired {
		t.Fatalf("unexpected capability catalog item: %#v", capability)
	}
	if len(capability.RequiredCapabilities) != 1 || capability.RequiredCapabilities[0] != "com.corex.demo" {
		t.Fatalf("unexpected required capabilities: %#v", capability.RequiredCapabilities)
	}
}

func TestNodeCatalogServiceGetRejectsUnknownNodeKind(t *testing.T) {
	registry := NewNodeAdapterRegistry()
	if err := registry.Register(NewSkillAdapter(nil)); err != nil {
		t.Fatalf("register skill adapter: %v", err)
	}
	service := NewNodeCatalogService(registry)
	if _, err := service.Get(context.Background(), "knowledge.publish"); err == nil {
		t.Fatal("expected unknown node kind error")
	}
}
