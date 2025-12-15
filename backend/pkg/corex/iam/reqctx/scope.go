package reqctx

import (
	"context"
	"strings"

	"gorm.io/gorm"
)

func WithScope(env string, tenantUUID *string) func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		query := db
		if env != "" {
			query = query.Where("env = ?", env)
		}
		if tenantUUID != nil && strings.TrimSpace(*tenantUUID) != "" {
			query = query.Where("tenant_uuid = ?", strings.TrimSpace(*tenantUUID))
		} else {
			query = query.Where("(tenant_uuid = '' OR tenant_uuid IS NULL)")
		}
		return query
	}
}

// ReqDB：从 ctx 读取 env/tenant，并返回带 Scope 的会话（极简用法，避免每处都拼）
// - 若取不到 env/tenant，请谨慎：可返回原 db（或 panic/返回错误，看你的风格）
func ReqDB(ctx context.Context, db *gorm.DB) *gorm.DB {
	e := GetEnv(ctx)
	t := strings.TrimSpace(GetTenantUUID(ctx))
	var tenantPointer *string
	if t != "" {
		tenantPointer = &t
	}
	return db.Scopes(WithScope(e, tenantPointer))
}
