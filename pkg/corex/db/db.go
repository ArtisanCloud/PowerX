package db

import (
	"context"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type Config struct {
	DSN                string
	MaxOpen, MaxIdle   int
	ConnMaxLifetimeSec int
}

func Open(cfg Config) (*gorm.DB, error) {
	gdb, err := gorm.Open(postgres.Open(cfg.DSN), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	sqlDB, _ := gdb.DB()
	if cfg.MaxOpen > 0 {
		sqlDB.SetMaxOpenConns(cfg.MaxOpen)
	}
	if cfg.MaxIdle > 0 {
		sqlDB.SetMaxIdleConns(cfg.MaxIdle)
	}
	// 省略 ConnMaxLifetime 设置…
	return gdb, nil
}

type tenantKey struct{}

func WithTenant(ctx context.Context, gdb *gorm.DB, tenantID string) *gorm.DB {
	gdb.Exec("set app.tenant_id = ?", tenantID)
	return gdb.WithContext(context.WithValue(ctx, tenantKey{}, tenantID))
}
func TenantFrom(ctx context.Context) (string, bool) {
	v := ctx.Value(tenantKey{})
	if v == nil {
		return "", false
	}
	return v.(string), true
}
