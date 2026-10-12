package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// LoadForUpgrade 不搜索其他配置、不加载开发 .env、不保存或生成密钥。
// 环境覆盖沿用正式 loadFromEnv；摘要同时冻结原文件和实际生效配置。
func LoadForUpgrade(path, dataDir string) (*Config, string, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return nil, "", fmt.Errorf("UPGRADE_CONFIG_UNREADABLE")
	}
	if info.Mode().Perm()&0077 != 0 {
		return nil, "", fmt.Errorf("UPGRADE_CONFIG_PERMISSIONS")
	}
	body, err := os.ReadFile(path)
	if err != nil || len(strings.TrimSpace(string(body))) == 0 {
		return nil, "", fmt.Errorf("UPGRADE_CONFIG_UNREADABLE")
	}
	var raw Config
	if err := yaml.Unmarshal(body, &raw); err != nil {
		return nil, "", fmt.Errorf("UPGRADE_CONFIG_INVALID")
	}
	if raw.Install.Status != "installed" {
		return nil, "", fmt.Errorf("UPGRADE_INSTANCE_NOT_INSTALLED")
	}
	bound := func(value, environment string) bool {
		return strings.TrimSpace(value) != "" || strings.TrimSpace(os.Getenv(environment)) != ""
	}
	if raw.Database.Driver == "" || (raw.Database.DSN == "" && (!bound(raw.Database.Host, "CORE_X_DB_HOST") || !bound(raw.Database.Database, "CORE_X_DB_DATABASE") || !bound(raw.Database.UserName, "CORE_X_DB_USERNAME") || !bound(raw.Database.Password, "CORE_X_DB_PASSWORD"))) {
		return nil, "", fmt.Errorf("UPGRADE_DATABASE_BINDING_INVALID")
	}
	if raw.Auth.JWTSecret == "" && os.Getenv("POWERX_AUTH_JWT_SECRET") == "" {
		return nil, "", fmt.Errorf("UPGRADE_SECRET_INVALID")
	}
	if raw.Server.SecretKey == "" || raw.Deployment.Env == "" {
		return nil, "", fmt.Errorf("UPGRADE_CONFIG_REQUIRED_FIELD")
	}
	cfg := GetDefaults()
	if err := yaml.Unmarshal(body, cfg); err != nil {
		return nil, "", fmt.Errorf("UPGRADE_CONFIG_INVALID")
	}
	if err := loadFromEnv(cfg); err != nil {
		return nil, "", fmt.Errorf("UPGRADE_ENVIRONMENT_INVALID")
	}
	if err := cfg.Validate(); err != nil {
		return nil, "", fmt.Errorf("UPGRADE_CONFIG_INVALID")
	}
	if cfg.Install.Status != "installed" || cfg.Deployment.Env != raw.Deployment.Env {
		return nil, "", fmt.Errorf("UPGRADE_DEPLOYMENT_CHANGED")
	}
	if _, err := cfg.Server.ParseKey(); err != nil {
		return nil, "", fmt.Errorf("UPGRADE_SECRET_INVALID")
	}
	if cfg.Database.Driver != "postgres" {
		return nil, "", fmt.Errorf("UPGRADE_DATABASE_UNSUPPORTED")
	}
	for _, name := range []string{"", "uploads", "plugins/installed", "runtime"} {
		info, err := os.Stat(filepath.Join(dataDir, name))
		if err != nil || !info.IsDir() {
			return nil, "", fmt.Errorf("UPGRADE_DATA_MOUNT_INCOMPLETE")
		}
		f, err := os.Open(filepath.Join(dataDir, name))
		if err != nil {
			return nil, "", fmt.Errorf("UPGRADE_DATA_MOUNT_UNREADABLE")
		}
		_ = f.Close()
	}
	if cfg.Storage.DefaultDriver == "local" {
		info, err := os.Stat(cfg.Storage.Local.BasePath)
		if !filepath.IsAbs(cfg.Storage.Local.BasePath) || err != nil || !info.IsDir() {
			return nil, "", fmt.Errorf("UPGRADE_STORAGE_PATH_UNAVAILABLE")
		}
	}
	effective, err := json.Marshal(cfg)
	if err != nil {
		return nil, "", fmt.Errorf("UPGRADE_CONFIG_INVALID")
	}
	hash := sha256.New()
	_, _ = hash.Write(body)
	_, _ = hash.Write(effective)
	return cfg, hex.EncodeToString(hash.Sum(nil)), nil
}

func upgradeDataDirectory() string {
	if value := os.Getenv("POWERX_UPGRADE_DATA_DIR"); value != "" {
		return value
	}
	return "/data"
}
