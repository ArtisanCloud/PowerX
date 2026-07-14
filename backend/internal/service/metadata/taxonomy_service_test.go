package metadata

import (
	"context"
	"errors"
	"testing"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/metadata"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestTaxonomyServiceCreateMoveAndConflictRules(t *testing.T) {
	db := newServiceTaxonomyTestDB(t)
	svc, err := NewService(Deps{DB: db})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	ctx := context.Background()
	tenantUUID := uuid.New().String()
	taxonomy, err := svc.CreateTaxonomy(ctx, CreateTaxonomyInput{
		TenantUUID: tenantUUID,
		Namespace:  "corex.product.category",
		Module:     "corex.product",
		NameI18n:   map[string]string{"zh-CN": "商品分类"},
		MaxDepth:   3,
	})
	if err != nil {
		t.Fatalf("create taxonomy: %v", err)
	}
	root, err := svc.CreateTaxonomyNode(ctx, CreateTaxonomyNodeInput{
		TenantUUID: tenantUUID, TaxonomyUUID: taxonomy.UUID, Code: "root",
		LabelI18n: map[string]string{"zh-CN": "根"},
	})
	if err != nil {
		t.Fatalf("create root: %v", err)
	}
	child, err := svc.CreateTaxonomyNode(ctx, CreateTaxonomyNodeInput{
		TenantUUID: tenantUUID, TaxonomyUUID: taxonomy.UUID, ParentUUID: &root.UUID, Code: "child",
		LabelI18n: map[string]string{"zh-CN": "子"},
	})
	if err != nil {
		t.Fatalf("create child: %v", err)
	}
	grandchild, err := svc.CreateTaxonomyNode(ctx, CreateTaxonomyNodeInput{
		TenantUUID: tenantUUID, TaxonomyUUID: taxonomy.UUID, ParentUUID: &child.UUID, Code: "grandchild",
		LabelI18n: map[string]string{"zh-CN": "孙"},
	})
	if err != nil {
		t.Fatalf("create grandchild: %v", err)
	}
	_, err = svc.MoveTaxonomyNode(ctx, MoveTaxonomyNodeInput{
		TenantUUID: tenantUUID, NodeUUID: root.UUID, TargetParentUUID: &grandchild.UUID, Version: root.Version,
	})
	if !errors.Is(err, ErrCircularMove) {
		t.Fatalf("expected circular move error, got %v", err)
	}
	_, err = svc.UpdateTaxonomyNode(ctx, UpdateTaxonomyNodeInput{
		TenantUUID: tenantUUID, NodeUUID: child.UUID, Version: child.Version + 99,
	})
	if !errors.Is(err, ErrOptimisticConflict) {
		t.Fatalf("expected optimistic conflict, got %v", err)
	}
	nodes, err := svc.ListTaxonomyNodes(ctx, ListTaxonomyNodesInput{TenantUUID: tenantUUID, TaxonomyUUID: taxonomy.UUID})
	if err != nil {
		t.Fatalf("list nodes: %v", err)
	}
	if len(nodes) != 3 {
		t.Fatalf("expected three nodes, got %d", len(nodes))
	}
}

func TestTaxonomyServiceRejectsMaxDepthAndReferenceDelete(t *testing.T) {
	db := newServiceTaxonomyTestDB(t)
	svc, err := NewService(Deps{DB: db})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	ctx := context.Background()
	tenantUUID := uuid.New().String()
	taxonomy, err := svc.CreateTaxonomy(ctx, CreateTaxonomyInput{
		TenantUUID: tenantUUID,
		Namespace:  "corex.product.category",
		Module:     "corex.product",
		NameI18n:   map[string]string{"zh-CN": "商品分类"},
		MaxDepth:   2,
	})
	if err != nil {
		t.Fatalf("create taxonomy: %v", err)
	}
	root, err := svc.CreateTaxonomyNode(ctx, CreateTaxonomyNodeInput{
		TenantUUID: tenantUUID, TaxonomyUUID: taxonomy.UUID, Code: "root",
		LabelI18n: map[string]string{"zh-CN": "根"},
	})
	if err != nil {
		t.Fatalf("create root: %v", err)
	}
	child, err := svc.CreateTaxonomyNode(ctx, CreateTaxonomyNodeInput{
		TenantUUID: tenantUUID, TaxonomyUUID: taxonomy.UUID, ParentUUID: &root.UUID, Code: "child",
		LabelI18n: map[string]string{"zh-CN": "子"},
	})
	if err != nil {
		t.Fatalf("create child: %v", err)
	}
	_, err = svc.CreateTaxonomyNode(ctx, CreateTaxonomyNodeInput{
		TenantUUID: tenantUUID, TaxonomyUUID: taxonomy.UUID, ParentUUID: &child.UUID, Code: "grandchild",
		LabelI18n: map[string]string{"zh-CN": "孙"},
	})
	if !errors.Is(err, ErrInvalidDepth) {
		t.Fatalf("expected max depth error, got %v", err)
	}
	if err := db.Create(&model.Reference{
		TenantUUID: tenantUUID, MetadataType: model.MetadataTypeTaxonomyNode, MetadataUUID: child.UUID,
		ResourceType: "product.sku", ResourceUUID: uuid.New().String(), FieldName: "category",
	}).Error; err != nil {
		t.Fatalf("create reference: %v", err)
	}
	refs, err := svc.DeleteTaxonomyNode(ctx, DeleteTaxonomyNodeInput{TenantUUID: tenantUUID, NodeUUID: child.UUID})
	if !errors.Is(err, ErrReferenceConflict) {
		t.Fatalf("expected reference conflict, got refs=%+v err=%v", refs, err)
	}
}

func newServiceTaxonomyTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	oldSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = oldSchema })
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.Taxonomy{}, &model.TaxonomyNode{}, &model.Reference{}); err != nil {
		t.Fatalf("migrate taxonomy models: %v", err)
	}
	return db
}
