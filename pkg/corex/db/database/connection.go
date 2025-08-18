package database

import (
	"fmt"
	"net/url"
	"time"

	corexdb "github.com/ArtisanCloud/PowerX/pkg/corex/db"

	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormLogger "gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

// pkg/corex/db/database/connection.go

// Connect 连接到数据库并返回 GORM DB 实例
// 支持：Postgres / MySQL
// 优先使用 cfg.DSN；否则按 Driver + 零散字段拼接。
func Connect(cfg corexdb.DatabaseConfig) (*gorm.DB, error) {
	driver := cfg.Driver
	if driver == "" {
		driver = "postgres" // 默认 PG
	}

	// GORM 基本配置
	gcfg := &gorm.Config{
		NamingStrategy: schema.NamingStrategy{
			TablePrefix:   cfg.TablePrefix,
			SingularTable: true,
		},
		//Logger: gormLogger.Default.LogMode(gormLogger.Info),
		Logger: gormLogger.Default.LogMode(gormLogger.Warn), // 原来是 Info
		// Logger: gormLogger.Default.LogMode(gormLogger.Silent), // 完全静默
	}

	// 选择 DSN
	dsn := cfg.DSN
	var (
		gdb *gorm.DB
		err error
	)

	if dsn == "" {
		switch driver {
		case "postgres", "pg", "Postgres", "PostgreSQL":
			// 例：host=... port=... user=... password=... dbname=... sslmode=disable TimeZone=Asia/Shanghai
			if cfg.SSLMode == "" {
				cfg.SSLMode = "disable"
			}
			if cfg.Timezone == "" {
				cfg.Timezone = "UTC"
			}
			dsn = fmt.Sprintf(
				"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s TimeZone=%s",
				cfg.Host, cfg.Port, cfg.Username, cfg.Password, cfg.Database, cfg.SSLMode, cfg.Timezone,
			)
			gdb, err = gorm.Open(postgres.Open(dsn), gcfg)

		case "mysql", "MySQL":
			// 例：user:pass@tcp(host:3306)/dbname?parseTime=true&loc=Asia%2FTokyo&charset=utf8mb4
			loc := cfg.Timezone
			if loc == "" {
				loc = "Local"
			}
			dsn = fmt.Sprintf(
				"%s:%s@tcp(%s:%d)/%s?parseTime=true&loc=%s&charset=utf8mb4",
				cfg.Username, cfg.Password, cfg.Host, cfg.Port, cfg.Database, url.QueryEscape(loc),
			)
			gdb, err = gorm.Open(mysql.Open(dsn), gcfg)

		default:
			return nil, fmt.Errorf("不支持的数据库驱动: %s", driver)
		}
	} else {
		// 直接使用显式 DSN
		switch driver {
		case "postgres", "pg", "Postgres", "PostgreSQL":
			gdb, err = gorm.Open(postgres.Open(dsn), gcfg)
		case "mysql", "MySQL":
			gdb, err = gorm.Open(mysql.Open(dsn), gcfg)
		default:
			return nil, fmt.Errorf("不支持的数据库驱动: %s (使用 DSN)", driver)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("连接数据库失败: %w", err)
	}

	// 连接池设置（缺省给个合理默认）
	sqlDB, err := gdb.DB()
	if err != nil {
		return nil, fmt.Errorf("获取底层 SQL DB 失败: %w", err)
	}
	if cfg.MaxIdleConns <= 0 {
		cfg.MaxIdleConns = 10
	}
	if cfg.MaxOpenConns <= 0 {
		cfg.MaxOpenConns = 100
	}
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetConnMaxLifetime(1 * time.Hour)

	return gdb, nil
}

// 兼容老调用：GetDB(*cfg) -> Connect(*cfg)
func GetDB(cfg *corexdb.DatabaseConfig) (*gorm.DB, error) {
	if cfg == nil {
		return nil, fmt.Errorf("nil database config")
	}
	return Connect(*cfg)
}
