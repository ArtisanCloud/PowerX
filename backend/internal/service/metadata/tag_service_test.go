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

type tagTestValidator struct{}

func (tagTestValidator) ValidateResource(context.Context, string, string) error {
	return nil
}

func TestTagServiceCreateUpdateBindingAndDeleteConflict(t *testing.T) {
	db := newServiceTagTestDB(t)
	svc, err := NewService(Deps{DB: db, ValidatorRegistry: NewStaticResourceValidatorRegistry(map[string]ResourceValidator{"product_validator": tagTestValidator{}})})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	ctx := context.Background()
	tenantUUID := uuid.New().String()
	if _, err := svc.RegisterResourceType(ctx, RegisterResourceTypeInput{
		TenantUUID:     tenantUUID,
		ResourceType:   "product.sku",
		Module:         "corex.product",
		NameI18n:       map[string]string{"zh-CN": "商品"},
		ValidatorKey:   "product_validator",
		BindingEnabled: true,
	}); err != nil {
		t.Fatalf("register resource type: %v", err)
	}
	tag, err := svc.CreateTag(ctx, CreateTagInput{
		TenantUUID:   tenantUUID,
		Namespace:    "corex.product",
		ResourceType: "product.sku",
		Code:         "featured",
		LabelI18n:    map[string]string{"zh-CN": "推荐"},
	})
	if err != nil {
		t.Fatalf("create tag: %v", err)
	}
	resourceUUID := uuid.New().String()
	bindings, err := svc.ReplaceTagBindings(ctx, ReplaceTagBindingsInput{
		TenantUUID: tenantUUID, ResourceType: "product.sku", ResourceUUID: resourceUUID, TagUUIDs: []string{tag.UUID},
	})
	if err != nil {
		t.Fatalf("replace bindings: %v", err)
	}
	if len(bindings) != 1 || bindings[0].Tag == nil || bindings[0].Tag.Code != "featured" {
		t.Fatalf("unexpected bindings: %+v", bindings)
	}
	if err := svc.DeleteTag(ctx, DeleteTagInput{TenantUUID: tenantUUID, TagUUID: tag.UUID}); !errors.Is(err, ErrTagBound) {
		t.Fatalf("expected tag bound conflict, got %v", err)
	}
	disabled := model.StatusDisabled
	_, err = svc.UpdateTag(ctx, UpdateTagInput{TenantUUID: tenantUUID, TagUUID: tag.UUID, Status: &disabled})
	if err != nil {
		t.Fatalf("disable tag: %v", err)
	}
	_, err = svc.ReplaceTagBindings(ctx, ReplaceTagBindingsInput{
		TenantUUID: tenantUUID, ResourceType: "product.sku", ResourceUUID: uuid.New().String(), TagUUIDs: []string{tag.UUID},
	})
	if !errors.Is(err, ErrTagDisabled) {
		t.Fatalf("expected disabled tag error, got %v", err)
	}
}

func TestTagServiceMergeMovesBindings(t *testing.T) {
	db := newServiceTagTestDB(t)
	svc, err := NewService(Deps{DB: db, ValidatorRegistry: NewStaticResourceValidatorRegistry(map[string]ResourceValidator{"product_validator": tagTestValidator{}})})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	ctx := context.Background()
	tenantUUID := uuid.New().String()
	if _, err := svc.RegisterResourceType(ctx, RegisterResourceTypeInput{
		TenantUUID:     tenantUUID,
		ResourceType:   "product.sku",
		Module:         "corex.product",
		NameI18n:       map[string]string{"zh-CN": "商品"},
		ValidatorKey:   "product_validator",
		BindingEnabled: true,
	}); err != nil {
		t.Fatalf("register resource type: %v", err)
	}
	source, err := svc.CreateTag(ctx, CreateTagInput{TenantUUID: tenantUUID, Namespace: "corex.product", ResourceType: "product.sku", Code: "old", LabelI18n: map[string]string{"zh-CN": "旧"}})
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	target, err := svc.CreateTag(ctx, CreateTagInput{TenantUUID: tenantUUID, Namespace: "corex.product", ResourceType: "product.sku", Code: "new", LabelI18n: map[string]string{"zh-CN": "新"}})
	if err != nil {
		t.Fatalf("create target: %v", err)
	}
	resourceUUID := uuid.New().String()
	if _, err := svc.ReplaceTagBindings(ctx, ReplaceTagBindingsInput{TenantUUID: tenantUUID, ResourceType: "product.sku", ResourceUUID: resourceUUID, TagUUIDs: []string{source.UUID}}); err != nil {
		t.Fatalf("replace binding: %v", err)
	}
	moved, err := svc.MergeTags(ctx, MergeTagsInput{TenantUUID: tenantUUID, SourceTagUUID: source.UUID, TargetTagUUID: target.UUID})
	if err != nil {
		t.Fatalf("merge tags: %v", err)
	}
	if moved != 1 {
		t.Fatalf("expected moved=1, got %d", moved)
	}
	bindings, err := svc.ListTagBindings(ctx, ListTagBindingsInput{TenantUUID: tenantUUID, ResourceType: "product.sku", ResourceUUID: resourceUUID})
	if err != nil {
		t.Fatalf("list bindings: %v", err)
	}
	if len(bindings) != 1 || bindings[0].TagUUID != target.UUID {
		t.Fatalf("expected binding moved to target, got %+v", bindings)
	}
}

func TestTagServiceRejectsNamespaceMismatchAndDuplicate(t *testing.T) {
	db := newServiceTagTestDB(t)
	svc, err := NewService(Deps{DB: db, ValidatorRegistry: NewStaticResourceValidatorRegistry(map[string]ResourceValidator{"product_validator": tagTestValidator{}})})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	ctx := context.Background()
	tenantUUID := uuid.New().String()
	if _, err := svc.RegisterResourceType(ctx, RegisterResourceTypeInput{
		TenantUUID:     tenantUUID,
		ResourceType:   "product.sku",
		Module:         "corex.product",
		NameI18n:       map[string]string{"zh-CN": "商品"},
		ValidatorKey:   "product_validator",
		BindingEnabled: true,
	}); err != nil {
		t.Fatalf("register resource type: %v", err)
	}
	_, err = svc.CreateTag(ctx, CreateTagInput{
		TenantUUID:   tenantUUID,
		Namespace:    "corex.sales",
		ResourceType: "product.sku",
		Code:         "featured",
		LabelI18n:    map[string]string{"zh-CN": "推荐"},
	})
	if !errors.Is(err, ErrNamespaceModuleMismatch) {
		t.Fatalf("expected namespace module mismatch, got %v", err)
	}
	if _, err := svc.CreateTag(ctx, CreateTagInput{
		TenantUUID:   tenantUUID,
		Namespace:    "corex.product",
		ResourceType: "product.sku",
		Code:         "featured",
		LabelI18n:    map[string]string{"zh-CN": "推荐"},
	}); err != nil {
		t.Fatalf("create tag: %v", err)
	}
	_, err = svc.CreateTag(ctx, CreateTagInput{
		TenantUUID:   tenantUUID,
		Namespace:    "corex.product",
		ResourceType: "product.sku",
		Code:         "featured",
		LabelI18n:    map[string]string{"zh-CN": "推荐 2"},
	})
	if !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("expected already exists, got %v", err)
	}
}

func newServiceTagTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	oldSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = oldSchema })
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.Tag{}, &model.TagBinding{}, &model.ResourceType{}); err != nil {
		t.Fatalf("migrate tag models: %v", err)
	}
	return db
}
