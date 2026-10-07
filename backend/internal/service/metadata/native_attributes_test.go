package metadata

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"testing"
)

func TestNativeTagMetadataCAS(t *testing.T) {
	db := newServiceTagTestDB(t)
	svc, err := NewService(Deps{DB: db})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	tenant := uuid.NewString()
	_, err = svc.RegisterResourceType(ctx, RegisterResourceTypeInput{TenantUUID: tenant, ResourceType: "court_mate.exercise", Module: "court_mate.exercise", NameI18n: map[string]string{"zh-CN": "fixture"}, BindingEnabled: false})
	if err != nil {
		t.Fatal(err)
	}
	tag, err := svc.CreateTag(ctx, CreateTagInput{TenantUUID: tenant, Namespace: "court_mate.exercise.goal", ResourceType: "court_mate.exercise", Code: "fixture", LabelI18n: map[string]string{"zh-CN": "fixture"}, Metadata: map[string]any{"version": 1}})
	if err != nil {
		t.Fatal(err)
	}
	expected := int64(1)
	attrs := map[string]any{"version": 2, "aliases": map[string]any{"en": "fixture_alias"}}
	updated, err := svc.UpdateTag(ctx, UpdateTagInput{TenantUUID: tenant, TagUUID: tag.UUID, ExpectedVersion: &expected, Metadata: &attrs})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Metadata["version"] != float64(2) {
		t.Fatalf("attributes missing: %#v", updated)
	}
	_, err = svc.UpdateTag(ctx, UpdateTagInput{TenantUUID: tenant, TagUUID: tag.UUID, ExpectedVersion: &expected, Metadata: &attrs})
	if !errors.Is(err, ErrOptimisticConflict) {
		t.Fatalf("stale update accepted: %v", err)
	}
	_, err = svc.UpdateTag(ctx, UpdateTagInput{TenantUUID: tenant, TagUUID: tag.UUID, Metadata: &attrs})
	if !errors.Is(err, ErrOptimisticConflict) {
		t.Fatalf("missing version accepted: %v", err)
	}
}

func TestNativeNodeAtomicMoveWithAttributes(t *testing.T) {
	db := newServiceTaxonomyTestDB(t)
	svc, err := NewService(Deps{DB: db})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	tenant := uuid.NewString()
	tree, err := svc.CreateTaxonomy(ctx, CreateTaxonomyInput{TenantUUID: tenant, Namespace: "court_mate.exercise.sport", Module: "court_mate.exercise", NameI18n: map[string]string{"zh-CN": "fixture"}, MaxDepth: 4})
	if err != nil {
		t.Fatal(err)
	}
	root, err := svc.CreateTaxonomyNode(ctx, CreateTaxonomyNodeInput{TenantUUID: tenant, TaxonomyUUID: tree.UUID, Code: "root", LabelI18n: map[string]string{"zh-CN": "fixture"}, Metadata: map[string]any{"version": 1}})
	if err != nil {
		t.Fatal(err)
	}
	child, err := svc.CreateTaxonomyNode(ctx, CreateTaxonomyNodeInput{TenantUUID: tenant, TaxonomyUUID: tree.UUID, ParentUUID: &root.UUID, Code: "child", LabelI18n: map[string]string{"zh-CN": "fixture"}, Metadata: map[string]any{"version": 1}})
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := svc.CreateTaxonomyNode(ctx, CreateTaxonomyNodeInput{TenantUUID: tenant, TaxonomyUUID: tree.UUID, ParentUUID: &child.UUID, Code: "leaf", LabelI18n: map[string]string{"zh-CN": "fixture"}})
	if err != nil {
		t.Fatal(err)
	}
	attrs := map[string]any{"version": 2, "aliases": map[string]any{"en": "fixture_alias"}}
	updated, err := svc.UpdateTaxonomyNode(ctx, UpdateTaxonomyNodeInput{TenantUUID: tenant, NodeUUID: child.UUID, Version: 1, MoveParent: true, Metadata: &attrs})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Version != 2 || updated.Depth != 1 || updated.ParentUUID != nil || updated.Metadata["version"] != float64(2) {
		t.Fatalf("non-atomic update: %#v", updated)
	}
	nodes, err := svc.ListTaxonomyNodes(ctx, ListTaxonomyNodesInput{TenantUUID: tenant, TaxonomyUUID: tree.UUID})
	if err != nil {
		t.Fatal(err)
	}
	for _, node := range nodes {
		if node.UUID == leaf.UUID && node.Depth != 2 {
			t.Fatalf("descendant path not updated: %#v", node)
		}
	}
	_, err = svc.UpdateTaxonomyNode(ctx, UpdateTaxonomyNodeInput{TenantUUID: tenant, NodeUUID: child.UUID, Version: 2, MoveParent: true, ParentUUID: &leaf.UUID})
	if !errors.Is(err, ErrCircularMove) {
		t.Fatalf("cycle accepted: %v", err)
	}
}
