package metadata

import (
	"context"
	"testing"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/metadata"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestTaxonomyRepositoryListNodesPathOrderAndTenantIsolation(t *testing.T) {
	db := newTaxonomyRepoTestDB(t)
	repo := NewTaxonomyRepository(db)
	ctx := context.Background()
	tenantUUID := uuid.New().String()
	otherTenantUUID := uuid.New().String()
	taxonomy := &model.Taxonomy{
		TenantUUID: tenantUUID, Namespace: "corex.product.category", Module: "corex.product",
		NameI18n: datatypes.JSON([]byte(`{"zh-CN":"商品分类"}`)), MaxDepth: 3, Status: model.StatusEnabled,
	}
	if err := repo.CreateTaxonomy(ctx, taxonomy); err != nil {
		t.Fatalf("create taxonomy: %v", err)
	}
	rootUUID := uuid.New()
	root := &model.TaxonomyNode{
		PowerUUIDModel: coremodel.PowerUUIDModel{UUID: rootUUID},
		TenantUUID:     tenantUUID, TaxonomyUUID: taxonomy.UUID.String(), Code: "root",
		LabelI18n: datatypes.JSON([]byte(`{"zh-CN":"根"}`)), Path: "/" + taxonomy.UUID.String() + "/" + rootUUID.String(), Depth: 1, Status: model.StatusEnabled, Version: 1,
	}
	if err := repo.CreateNode(ctx, root); err != nil {
		t.Fatalf("create root: %v", err)
	}
	parent := root.UUID.String()
	childUUID := uuid.New()
	child := &model.TaxonomyNode{
		PowerUUIDModel: coremodel.PowerUUIDModel{UUID: childUUID},
		TenantUUID:     tenantUUID, TaxonomyUUID: taxonomy.UUID.String(), ParentUUID: &parent, Code: "child",
		LabelI18n: datatypes.JSON([]byte(`{"zh-CN":"子"}`)), Path: root.Path + "/" + childUUID.String(), Depth: 2, SortOrder: 5, Status: model.StatusEnabled, Version: 1,
	}
	if err := repo.CreateNode(ctx, child); err != nil {
		t.Fatalf("create child: %v", err)
	}
	other := *child
	other.ID = 0
	other.UUID = uuid.New()
	other.TenantUUID = otherTenantUUID
	other.Path = "/" + taxonomy.UUID.String() + "/" + other.UUID.String()
	if err := repo.CreateNode(ctx, &other); err != nil {
		t.Fatalf("create other tenant node: %v", err)
	}
	rows, err := repo.ListNodes(ctx, TaxonomyNodeListOptions{TenantUUID: tenantUUID, TaxonomyUUID: taxonomy.UUID.String()})
	if err != nil {
		t.Fatalf("list nodes: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected tenant isolated 2 rows, got %d", len(rows))
	}
	if rows[0].Code != "root" || rows[1].Code != "child" || rows[1].Depth != 2 {
		t.Fatalf("unexpected tree order/depth: %+v", rows)
	}
}

func newTaxonomyRepoTestDB(t *testing.T) *gorm.DB {
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
