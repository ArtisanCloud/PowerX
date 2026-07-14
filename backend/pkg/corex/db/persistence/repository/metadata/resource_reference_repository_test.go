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

func TestResourceTypeRepositoryTenantIsolationAndList(t *testing.T) {
	db := newResourceReferenceRepoTestDB(t)
	repo := NewResourceTypeRepository(db)
	ctx := context.Background()
	tenantA := uuid.New().String()
	tenantB := uuid.New().String()
	if err := repo.Create(ctx, &model.ResourceType{
		TenantUUID: tenantA, ResourceType: "product.sku", Module: "corex.product",
		NameI18n: datatypes.JSON([]byte(`{"zh-CN":"商品"}`)), ValidatorKey: "product_validator", BindingEnabled: true, Status: model.StatusEnabled,
	}); err != nil {
		t.Fatalf("create tenant A resource type: %v", err)
	}
	if err := repo.Create(ctx, &model.ResourceType{
		TenantUUID: tenantB, ResourceType: "product.sku", Module: "corex.product",
		NameI18n: datatypes.JSON([]byte(`{"zh-CN":"商品"}`)), ValidatorKey: "product_validator", BindingEnabled: true, Status: model.StatusEnabled,
	}); err != nil {
		t.Fatalf("create tenant B resource type: %v", err)
	}
	rows, total, err := repo.List(ctx, ResourceTypeListOptions{TenantUUID: tenantA, Module: "corex.product", Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("list resource types: %v", err)
	}
	if total != 1 || len(rows) != 1 || rows[0].TenantUUID != tenantA {
		t.Fatalf("expected tenant-isolated one row, total=%d rows=%+v", total, rows)
	}
}

func TestReferenceRepositoryReplaceAndCount(t *testing.T) {
	db := newResourceReferenceRepoTestDB(t)
	repo := NewReferenceRepository(db)
	ctx := context.Background()
	tenantUUID := uuid.New().String()
	resourceUUID := uuid.New().String()
	firstMetadataUUID := uuid.New().String()
	secondMetadataUUID := uuid.New().String()
	if err := repo.ReplaceForResource(ctx, tenantUUID, "product.sku", resourceUUID, []model.Reference{
		{TenantUUID: tenantUUID, MetadataType: model.MetadataTypeDictionaryItem, MetadataUUID: firstMetadataUUID, ResourceType: "product.sku", ResourceUUID: resourceUUID, FieldName: "status"},
	}); err != nil {
		t.Fatalf("replace first refs: %v", err)
	}
	if err := repo.ReplaceForResource(ctx, tenantUUID, "product.sku", resourceUUID, []model.Reference{
		{TenantUUID: tenantUUID, MetadataType: model.MetadataTypeTaxonomyNode, MetadataUUID: secondMetadataUUID, ResourceType: "product.sku", ResourceUUID: resourceUUID, FieldName: "category"},
	}); err != nil {
		t.Fatalf("replace second refs: %v", err)
	}
	rows, err := repo.ListForResource(ctx, tenantUUID, "product.sku", resourceUUID)
	if err != nil {
		t.Fatalf("list refs: %v", err)
	}
	if len(rows) != 1 || rows[0].MetadataUUID != secondMetadataUUID {
		t.Fatalf("expected replaced refs, got %+v", rows)
	}
	firstCount, err := repo.CountForMetadata(ctx, tenantUUID, model.MetadataTypeDictionaryItem, firstMetadataUUID)
	if err != nil {
		t.Fatalf("count first metadata: %v", err)
	}
	secondCount, err := repo.CountForMetadata(ctx, tenantUUID, model.MetadataTypeTaxonomyNode, secondMetadataUUID)
	if err != nil {
		t.Fatalf("count second metadata: %v", err)
	}
	if firstCount != 0 || secondCount != 1 {
		t.Fatalf("unexpected counts first=%d second=%d", firstCount, secondCount)
	}
}

func newResourceReferenceRepoTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	oldSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = oldSchema })
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.ResourceType{}, &model.Reference{}); err != nil {
		t.Fatalf("migrate resource reference models: %v", err)
	}
	return db
}
