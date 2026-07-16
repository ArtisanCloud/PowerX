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

type resourceReferenceValidator struct{}

func (resourceReferenceValidator) ValidateResource(context.Context, string, string) error {
	return nil
}

func TestResourceReferenceServiceValidatorRules(t *testing.T) {
	db := newResourceReferenceServiceTestDB(t)
	svc, err := NewService(Deps{
		DB: db,
		ValidatorRegistry: NewStaticResourceValidatorRegistry(map[string]ResourceValidator{
			"product_validator": resourceReferenceValidator{},
		}),
	})
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
		BindingEnabled: true,
	}); !errors.Is(err, ErrResourceValidatorMissing) {
		t.Fatalf("expected register enabled type without validator to fail, got %v", err)
	}

	if _, err := svc.RegisterResourceType(ctx, RegisterResourceTypeInput{
		TenantUUID:     tenantUUID,
		ResourceType:   "product.sku",
		Module:         "corex.product",
		NameI18n:       map[string]string{"zh-CN": "商品"},
		BindingEnabled: false,
	}); err != nil {
		t.Fatalf("register disabled resource type: %v", err)
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
	if _, err := svc.ReplaceTagBindings(ctx, ReplaceTagBindingsInput{
		TenantUUID: tenantUUID, ResourceType: "product.sku", ResourceUUID: uuid.New().String(), TagUUIDs: []string{tag.UUID},
	}); !errors.Is(err, ErrResourceBindingDisabled) {
		t.Fatalf("expected disabled binding, got %v", err)
	}

	enabled := true
	validatorKey := "missing_validator"
	resourceType, err := svc.UpdateResourceType(ctx, UpdateResourceTypeInput{
		TenantUUID: tenantUUID, ResourceTypeUUID: mustResourceTypeUUID(t, svc, ctx, tenantUUID, "product.sku"), ValidatorKey: &validatorKey, BindingEnabled: &enabled,
	})
	if err != nil {
		t.Fatalf("update resource type with missing runtime validator key should persist config: %v", err)
	}
	if resourceType.ValidatorStatus != ValidatorStatusMissing {
		t.Fatalf("expected missing validator status, got %s", resourceType.ValidatorStatus)
	}
	if _, err := svc.ReplaceTagBindings(ctx, ReplaceTagBindingsInput{
		TenantUUID: tenantUUID, ResourceType: "product.sku", ResourceUUID: uuid.New().String(), TagUUIDs: []string{tag.UUID},
	}); !errors.Is(err, ErrResourceValidatorMissing) {
		t.Fatalf("expected missing validator, got %v", err)
	}

	validatorKey = "product_validator"
	if _, err := svc.UpdateResourceType(ctx, UpdateResourceTypeInput{
		TenantUUID: tenantUUID, ResourceTypeUUID: mustResourceTypeUUID(t, svc, ctx, tenantUUID, "product.sku"), ValidatorKey: &validatorKey, BindingEnabled: &enabled,
	}); err != nil {
		t.Fatalf("update resource type validator: %v", err)
	}
	bindings, err := svc.ReplaceTagBindings(ctx, ReplaceTagBindingsInput{
		TenantUUID: tenantUUID, ResourceType: "product.sku", ResourceUUID: uuid.New().String(), TagUUIDs: []string{tag.UUID},
	})
	if err != nil {
		t.Fatalf("replace bindings with validator: %v", err)
	}
	if len(bindings) != 1 {
		t.Fatalf("expected one binding, got %+v", bindings)
	}
}

func TestReferenceServiceReplaceRejectsMismatchWithoutPartialWrite(t *testing.T) {
	db := newResourceReferenceServiceTestDB(t)
	svc, err := NewService(Deps{DB: db})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	ctx := context.Background()
	tenantUUID := uuid.New().String()
	resourceUUID := uuid.New().String()
	metadataUUID := uuid.New().String()
	if _, err := svc.ReplaceMetadataReferencesForResource(ctx, ReplaceMetadataReferencesInput{
		TenantUUID: tenantUUID, ResourceType: "product.sku", ResourceUUID: resourceUUID,
		References: []MetadataReferenceInput{{MetadataType: model.MetadataTypeDictionaryItem, MetadataUUID: metadataUUID, ResourceType: "product.sku", ResourceUUID: resourceUUID, FieldName: "status"}},
	}); err != nil {
		t.Fatalf("replace initial refs: %v", err)
	}
	if _, err := svc.ReplaceMetadataReferencesForResource(ctx, ReplaceMetadataReferencesInput{
		TenantUUID: tenantUUID, ResourceType: "product.sku", ResourceUUID: resourceUUID,
		References: []MetadataReferenceInput{{MetadataType: model.MetadataTypeDictionaryItem, MetadataUUID: uuid.New().String(), ResourceType: "order.record", ResourceUUID: uuid.New().String(), FieldName: "status"}},
	}); !errors.Is(err, ErrReferenceResourceMismatch) {
		t.Fatalf("expected reference resource mismatch, got %v", err)
	}
	rows, err := svc.referenceRepo().ListForResource(ctx, tenantUUID, "product.sku", resourceUUID)
	if err != nil {
		t.Fatalf("list refs: %v", err)
	}
	if len(rows) != 1 || rows[0].MetadataUUID != metadataUUID {
		t.Fatalf("expected original refs preserved, got %+v", rows)
	}
}

func mustResourceTypeUUID(t *testing.T, svc *Service, ctx context.Context, tenantUUID, resourceType string) string {
	t.Helper()
	row, err := svc.resourceTypeRepo().GetByResourceType(ctx, tenantUUID, resourceType)
	if err != nil {
		t.Fatalf("get resource type: %v", err)
	}
	return row.UUID.String()
}

func newResourceReferenceServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	oldSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = oldSchema })
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.ResourceType{}, &model.Reference{}, &model.Tag{}, &model.TagBinding{}); err != nil {
		t.Fatalf("migrate resource reference service models: %v", err)
	}
	return db
}
