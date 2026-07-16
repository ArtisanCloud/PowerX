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

func TestDictionaryRepositoryTenantIsolationAndStatusFilter(t *testing.T) {
	db := newDictionaryTestDB(t)
	repo := NewDictionaryRepository(db)
	ctx := context.Background()
	tenantA := uuid.New().String()
	tenantB := uuid.New().String()
	if err := repo.CreateNamespace(ctx, &model.DictionaryNamespace{
		TenantUUID: tenantA, Namespace: "corex.customer.level", Module: "corex.customer",
		NameI18n: datatypes.JSON([]byte(`{"zh-CN":"客户等级"}`)), Status: model.StatusEnabled,
	}); err != nil {
		t.Fatalf("create tenant A namespace: %v", err)
	}
	if err := repo.CreateNamespace(ctx, &model.DictionaryNamespace{
		TenantUUID: tenantB, Namespace: "corex.customer.level", Module: "corex.customer",
		NameI18n: datatypes.JSON([]byte(`{"zh-CN":"客户等级"}`)), Status: model.StatusDisabled,
	}); err != nil {
		t.Fatalf("create tenant B namespace: %v", err)
	}
	rows, total, err := repo.ListNamespaces(ctx, DictionaryNamespaceListOptions{TenantUUID: tenantA, Status: model.StatusEnabled})
	if err != nil {
		t.Fatalf("list namespaces: %v", err)
	}
	if total != 1 || len(rows) != 1 || rows[0].TenantUUID != tenantA {
		t.Fatalf("unexpected tenant isolated rows: total=%d rows=%+v", total, rows)
	}
}

func TestDictionaryRepositoryRejectsDuplicateNamespaceInTenant(t *testing.T) {
	db := newDictionaryTestDB(t)
	repo := NewDictionaryRepository(db)
	ctx := context.Background()
	tenantUUID := uuid.New().String()
	row := &model.DictionaryNamespace{
		TenantUUID: tenantUUID, Namespace: "corex.customer.level", Module: "corex.customer",
		NameI18n: datatypes.JSON([]byte(`{"zh-CN":"客户等级"}`)), Status: model.StatusEnabled,
	}
	if err := repo.CreateNamespace(ctx, row); err != nil {
		t.Fatalf("create namespace: %v", err)
	}
	if err := repo.CreateNamespace(ctx, &model.DictionaryNamespace{
		TenantUUID: tenantUUID, Namespace: "corex.customer.level", Module: "corex.customer",
		NameI18n: datatypes.JSON([]byte(`{"zh-CN":"重复"}`)), Status: model.StatusEnabled,
	}); err == nil {
		t.Fatalf("expected duplicate namespace to fail")
	}
}

func newDictionaryTestDB(t *testing.T) *gorm.DB {
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
