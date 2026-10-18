package config

import (
	"fmt"
	"os"
	"strings"
)

// LoadForMigration 只供数据库迁移/状态命令使用，不生成密钥、不校验登录服务配置。
// 升级协议仍使用完整已安装配置校验，不能借迁移入口绕过升级预检。
func LoadForMigration(path string) (*Config, error) {
	if os.Getenv("POWERX_UPGRADE_MODE") == "1" {
		cfg, _, err := LoadForUpgrade(path, upgradeDataDirectory())
		return cfg, err
	}
	if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("迁移需要明确且存在的 config.yaml，请检查 -config 或 POWERX_CONFIG")
	}
	return loadConfiguration(path, true)
}

func (c *Config) ValidateMigration() error {
	if err := c.ValidateDeploymentIdentity(); err != nil {
		return err
	}
	db := c.Database
	switch db.Driver {
	case "", "postgres", "pg", "Postgres", "PostgreSQL", "mysql", "MySQL":
	default:
		return fmt.Errorf("database.driver 必须是 postgres 或 mysql")
	}
	if strings.TrimSpace(db.DSN) == "" {
		if strings.TrimSpace(db.Host) == "" || strings.TrimSpace(db.UserName) == "" || strings.TrimSpace(db.Database) == "" {
			return fmt.Errorf("迁移需要 database.dsn 或完整的 database.host/username/database")
		}
		if db.Port <= 0 || db.Port > 65535 {
			return fmt.Errorf("database.port 必须在 1-65535 范围内")
		}
	}
	if db.MaxIdleConns < 0 || db.MaxOpenConns <= 0 || db.ConnMaxLifetimeMinutes <= 0 {
		return fmt.Errorf("数据库连接池配置无效")
	}
	return nil
}
