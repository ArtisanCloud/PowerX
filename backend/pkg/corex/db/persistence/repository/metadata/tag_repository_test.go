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

func TestTagRepositoryReplaceBindingsUsageAndDeleteConflict(t *testing.T) {
	db := newTagRepoTestDB(t)
	tagRepo := NewTagRepository(db)
	bindingRepo := NewTagBindingRepository(db)
	ctx := context.Background()
	tenantUUID := uuid.New().String()
	tag := &model.Tag{
		TenantUUID: tenantUUID, Namespace: "corex.product", ResourceType: "product.sku", Code: "featured",
		LabelI18n: datatypes.JSON([]byte(`{"zh-CN":"推荐"}`)), Source: model.SourceAdmin, Status: model.StatusEnabled,
	}
	if err := tagRepo.CreateTag(ctx, tag); err != nil {
		t.Fatalf("create tag: %v", err)
	}
	resourceUUID := uuid.New().String()
	if _, err := bindingRepo.ReplaceByResource(ctx, tenantUUID, "product.sku", resourceUUID, "", []string{tag.UUID.String()}); err != nil {
		t.Fatalf("replace bindings: %v", err)
	}
	if err := tagRepo.RecountUsage(ctx, tenantUUID, tag.UUID.String()); err != nil {
		t.Fatalf("recount usage: %v", err)
	}
	got, err := tagRepo.GetTag(ctx, tenantUUID, tag.UUID.String())
	if err != nil {
		t.Fatalf("get tag: %v", err)
	}
	if got.UsageCount != 1 {
		t.Fatalf("expected usage_count=1, got %d", got.UsageCount)
	}
	total, err := tagRepo.CountBindings(ctx, tenantUUID, tag.UUID.String())
	if err != nil {
		t.Fatalf("count bindings: %v", err)
	}
	if total != 1 {
		t.Fatalf("expected one binding, got %d", total)
	}
}

func TestTagRepositoryMergeTags(t *testing.T) {
	db := newTagRepoTestDB(t)
	tagRepo := NewTagRepository(db)
	bindingRepo := NewTagBindingRepository(db)
	ctx := context.Background()
	tenantUUID := uuid.New().String()
	source := &model.Tag{TenantUUID: tenantUUID, Namespace: "corex.product", ResourceType: "product.sku", Code: "old", LabelI18n: datatypes.JSON([]byte(`{"zh-CN":"旧"}`)), Source: model.SourceAdmin, Status: model.StatusEnabled}
	target := &model.Tag{TenantUUID: tenantUUID, Namespace: "corex.product", ResourceType: "product.sku", Code: "new", LabelI18n: datatypes.JSON([]byte(`{"zh-CN":"新"}`)), Source: model.SourceAdmin, Status: model.StatusEnabled}
	if err := tagRepo.CreateTag(ctx, source); err != nil {
		t.Fatalf("create source: %v", err)
	}
	if err := tagRepo.CreateTag(ctx, target); err != nil {
		t.Fatalf("create target: %v", err)
	}
	resourceUUID := uuid.New().String()
	if _, err := bindingRepo.ReplaceByResource(ctx, tenantUUID, "product.sku", resourceUUID, "", []string{source.UUID.String()}); err != nil {
		t.Fatalf("replace source binding: %v", err)
	}
	moved, err := tagRepo.MergeTags(ctx, tenantUUID, source.UUID.String(), target.UUID.String())
	if err != nil {
		t.Fatalf("merge tags: %v", err)
	}
	if moved != 1 {
		t.Fatalf("expected moved=1, got %d", moved)
	}
	archived, err := tagRepo.GetTag(ctx, tenantUUID, source.UUID.String())
	if err != nil {
		t.Fatalf("get source: %v", err)
	}
	if archived.Status != model.StatusArchived {
		t.Fatalf("expected source archived, got %s", archived.Status)
	}
}

func newTagRepoTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	oldSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = oldSchema })
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.Tag{}, &model.TagBinding{}); err != nil {
		t.Fatalf("migrate tag models: %v", err)
	}
	return db
}
