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

func TestDictionaryServiceCreateUpdateListAndDeleteConflict(t *testing.T) {
	db := newServiceDictionaryTestDB(t)
	svc, err := NewService(Deps{DB: db})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	ctx := context.Background()
	tenantUUID := uuid.New().String()
	ns, err := svc.CreateDictionaryNamespace(ctx, CreateDictionaryNamespaceInput{
		TenantUUID: tenantUUID,
		Namespace:  "corex.customer.level",
		Module:     "corex.customer",
		NameI18n:   map[string]string{"zh-CN": "客户等级"},
	})
	if err != nil {
		t.Fatalf("create namespace: %v", err)
	}
	item, err := svc.CreateDictionaryItem(ctx, CreateDictionaryItemInput{
		TenantUUID:    tenantUUID,
		NamespaceUUID: ns.UUID,
		Code:          "vip",
		LabelI18n:     map[string]string{"zh-CN": "VIP"},
		SortOrder:     10,
	})
	if err != nil {
		t.Fatalf("create item: %v", err)
	}
	status := model.StatusDisabled
	updated, err := svc.UpdateDictionaryItem(ctx, UpdateDictionaryItemInput{
		TenantUUID: tenantUUID,
		ItemUUID:   item.UUID,
		Status:     &status,
	})
	if err != nil {
		t.Fatalf("update item: %v", err)
	}
	if updated.Status != model.StatusDisabled {
		t.Fatalf("expected disabled status, got %s", updated.Status)
	}
	page, err := svc.ListDictionaryItems(ctx, ListDictionaryItemsInput{TenantUUID: tenantUUID, NamespaceUUID: ns.UUID, Status: model.StatusDisabled})
	if err != nil {
		t.Fatalf("list items: %v", err)
	}
	if page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("unexpected disabled item page: %+v", page)
	}
	if err := db.Create(&model.Reference{
		TenantUUID: tenantUUID, MetadataType: model.MetadataTypeDictionaryItem, MetadataUUID: item.UUID,
		ResourceType: "customer.account", ResourceUUID: uuid.New().String(), FieldName: "level",
	}).Error; err != nil {
		t.Fatalf("create reference: %v", err)
	}
	refs, err := svc.DeleteDictionaryItem(ctx, DeleteDictionaryItemInput{TenantUUID: tenantUUID, ItemUUID: item.UUID})
	if !errors.Is(err, ErrReferenceConflict) {
		t.Fatalf("expected reference conflict, got refs=%+v err=%v", refs, err)
	}
}

func TestDictionaryServiceRejectsMissingRequiredLocale(t *testing.T) {
	db := newServiceDictionaryTestDB(t)
	svc, err := NewService(Deps{DB: db})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	_, err = svc.CreateDictionaryNamespace(context.Background(), CreateDictionaryNamespaceInput{
		TenantUUID: uuid.New().String(),
		Namespace:  "corex.customer.level",
		Module:     "corex.customer",
		NameI18n:   map[string]string{"en-US": "Level"},
	})
	if !errors.Is(err, ErrMissingRequiredLocale) {
		t.Fatalf("expected missing locale error, got %v", err)
	}
}

func TestDictionaryServiceRejectsNamespaceMismatchAndDuplicate(t *testing.T) {
	db := newServiceDictionaryTestDB(t)
	svc, err := NewService(Deps{DB: db})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	ctx := context.Background()
	tenantUUID := uuid.New().String()
	_, err = svc.CreateDictionaryNamespace(ctx, CreateDictionaryNamespaceInput{
		TenantUUID: tenantUUID,
		Namespace:  "corex.sales.level",
		Module:     "corex.customer",
		NameI18n:   map[string]string{"zh-CN": "客户等级"},
	})
	if !errors.Is(err, ErrNamespaceModuleMismatch) {
		t.Fatalf("expected namespace module mismatch, got %v", err)
	}
	if _, err := svc.CreateDictionaryNamespace(ctx, CreateDictionaryNamespaceInput{
		TenantUUID: tenantUUID,
		Namespace:  "corex.customer.level",
		Module:     "corex.customer",
		NameI18n:   map[string]string{"zh-CN": "客户等级"},
	}); err != nil {
		t.Fatalf("create namespace: %v", err)
	}
	_, err = svc.CreateDictionaryNamespace(ctx, CreateDictionaryNamespaceInput{
		TenantUUID: tenantUUID,
		Namespace:  "corex.customer.level",
		Module:     "corex.customer",
		NameI18n:   map[string]string{"zh-CN": "客户等级 2"},
	})
	if !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("expected already exists, got %v", err)
	}
}

func newServiceDictionaryTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	oldSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = oldSchema })
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.DictionaryNamespace{}, &model.DictionaryItem{}, &model.Reference{}); err != nil {
		t.Fatalf("migrate metadata dictionary models: %v", err)
	}
	return db
}

func TestDictionaryExtensionRoundTripAndVersionedUpdate(t *testing.T) {
	db := newServiceDictionaryTestDB(t)
	svc, err := NewService(Deps{DB: db})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	tenant := uuid.NewString()
	ns, err := svc.CreateDictionaryNamespace(ctx, CreateDictionaryNamespaceInput{TenantUUID: tenant, Namespace: "court_mate.exercise.sport", Module: "court_mate.exercise", NameI18n: map[string]string{"zh-CN": "fixture"}})
	if err != nil {
		t.Fatal(err)
	}
	item, err := svc.CreateDictionaryItem(ctx, CreateDictionaryItemInput{TenantUUID: tenant, NamespaceUUID: ns.UUID, Code: "basketball", LabelI18n: map[string]string{"zh-CN": "fixture"}, Metadata: map[string]any{"version": 1, "kind": "term", "dimension": "sport", "parent_uuid": uuid.NewString()}})
	if err != nil {
		t.Fatal(err)
	}
	if item.Metadata["kind"] != "term" || item.Metadata["parent_uuid"] == nil {
		t.Fatal("METADATA_EXTENSION_LOST")
	}
	page, err := svc.ListDictionaryItems(ctx, ListDictionaryItemsInput{TenantUUID: tenant, NamespaceUUID: ns.UUID})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].Metadata["dimension"] != "sport" {
		t.Fatal("METADATA_EXTENSION_READ_LOST")
	}
	expected := int64(1)
	attrs := map[string]any{"version": 2, "kind": "term", "dimension": "sport"}
	updated, err := svc.UpdateDictionaryItem(ctx, UpdateDictionaryItemInput{TenantUUID: tenant, ItemUUID: item.UUID, Metadata: &attrs, ExpectedVersion: &expected})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.UpdateDictionaryItem(ctx, UpdateDictionaryItemInput{TenantUUID: tenant, ItemUUID: item.UUID, Metadata: &attrs})
	if !errors.Is(err, ErrOptimisticConflict) {
		t.Fatal("METADATA_UNCONDITIONAL_WRITE_ACCEPTED")
	}
	if updated.Metadata["version"] != float64(2) {
		t.Fatal("METADATA_VERSION_NOT_ADVANCED")
	}
	_, err = svc.UpdateDictionaryItem(ctx, UpdateDictionaryItemInput{TenantUUID: tenant, ItemUUID: item.UUID, Metadata: &attrs, ExpectedVersion: &expected})
	if !errors.Is(err, ErrOptimisticConflict) {
		t.Fatalf("METADATA_STALE_WRITE_ACCEPTED: %v", err)
	}
}
