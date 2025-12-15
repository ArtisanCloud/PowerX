package seed

import (
	dbtenant "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	tenantrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/tenant"
	"gorm.io/gorm"
)

// ensureSystemTenant 返回（或创建）系统租户记录，供需要 UUID/ID 的种子脚本复用。
func ensureSystemTenant(db *gorm.DB) (*dbtenant.Tenant, error) {
	repo := tenantrepo.NewTenantRepository(db)
	return repo.EnsureByKey(seedCtx(), dbtenant.SystemTenantKey, "System", dbtenant.TenantPlanFree, dbtenant.TenantTypeSystem)
}
