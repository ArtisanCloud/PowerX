package reqctx

import (
	"context"

	"gorm.io/gorm"
)

func WithScope(env string, tenantID *uint64) func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		query := db
		if env != "" {
			query = query.Where("env = ?", env)
		}
		if tenantID != nil {
			query = query.Where("tenant_id = ?", *tenantID)
		} else {
			query = query.Where("tenant_id IS NULL")
		}
		return query
	}
}

// ReqDB：从 ctx 读取 env/tenant，并返回带 Scope 的会话（极简用法，避免每处都拼）
// - 若取不到 env/tenant，请谨慎：可返回原 db（或 panic/返回错误，看你的风格）
func ReqDB(ctx context.Context, db *gorm.DB) *gorm.DB {
	e := GetEnv(ctx)
	t := GetTenantID(ctx)
	var tp *uint64
	if t > 0 {
		tp = &t
	}
	return db.Scopes(WithScope(e, tp))
}
