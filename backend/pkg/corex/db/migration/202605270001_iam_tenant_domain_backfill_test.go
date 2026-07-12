package migration

import (
	"testing"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	modeltenant "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestEnsureIAMTenantDomainBackfillMigration(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:tenant-domain-backfill?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)

	prevSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() {
		coremodel.PowerXSchema = prevSchema
	})

	require.NoError(t, db.AutoMigrate(&modeltenant.Tenant{}))
	require.NoError(t, db.Create(&modeltenant.Tenant{
		Key:    "acme",
		Name:   "Acme",
		Status: modeltenant.TenantStatusActive,
		Type:   modeltenant.TenantTypeEnterprise,
		Plan:   modeltenant.TenantPlanFree,
	}).Error)

	require.NoError(t, EnsureIAMTenantDomainBackfillMigration(db))

	var tenant modeltenant.Tenant
	require.NoError(t, db.Where("key = ?", "acme").First(&tenant).Error)
	require.Equal(t, "acme.tenant.powerx.local", tenant.Domain)
}
