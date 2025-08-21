package config

import (
	"fmt"
	"strings"
	"time"
)

// Validate 验证配置的合法性（已对齐新版 AuthConfig）
func (c *Config) Validate() error {
	var errors []string

	// --- Server ---
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		errors = append(errors, "server.port 必须在 1-65535 范围内")
	}

	// --- Auth ---
	if strings.TrimSpace(c.Auth.JWTSecret) == "" {
		errors = append(errors, "auth.jwt_secret 不能为空")
	} else if len(c.Auth.JWTSecret) < 32 {
		errors = append(errors, "auth.jwt_secret 长度至少32个字符")
	}
	if strings.TrimSpace(c.Auth.Issuer) == "" {
		errors = append(errors, "auth.issuer 不能为空")
	}
	if strings.TrimSpace(c.Auth.AudienceUser) == "" {
		errors = append(errors, "auth.audience_user 不能为空")
	}
	if strings.TrimSpace(c.Auth.AudienceCustomer) == "" {
		errors = append(errors, "auth.audience_customer 不能为空")
	}
	if len(c.Auth.Platforms) == 0 {
		errors = append(errors, "auth.platforms 不能为空")
	}
	// TTL 校验：能解析且 >0
	if d, err := time.ParseDuration(strings.TrimSpace(c.Auth.AccessTTLStr)); err != nil || d <= 0 {
		errors = append(errors, "auth.access_ttl 必须是合法的正 Duration（例如 \"15m\"）")
	}
	if d, err := time.ParseDuration(strings.TrimSpace(c.Auth.RefreshTTLStr)); err != nil || d <= 0 {
		errors = append(errors, "auth.refresh_ttl 必须是合法的正 Duration（例如 \"336h\"）")
	}

	// --- Event Bus ---
	if c.EventBus.Type != "local" && c.EventBus.Type != "redis" {
		errors = append(errors, "event_bus.type 必须是 'local' 或 'redis'")
	}
	if c.EventBus.Type == "redis" && strings.TrimSpace(c.EventBus.RedisAddr) == "" {
		errors = append(errors, "使用 redis 事件总线时，event_bus.redis_addr 不能为空")
	}

	// --- LowCode ---
	if c.LowCode.MaxConcurrentFlows <= 0 {
		errors = append(errors, "dynamic_form.max_concurrent_flows 必须大于0")
	}
	if c.LowCode.DefaultTimeoutSec <= 0 {
		errors = append(errors, "dynamic_form.default_timeout_sec 必须大于0")
	}

	// --- Database ---
	if strings.TrimSpace(c.Database.Host) == "" {
		errors = append(errors, "database.host 不能为空")
	}
	if c.Database.Port <= 0 || c.Database.Port > 65535 {
		errors = append(errors, "database.port 必须在 1-65535 范围内")
	}
	if strings.TrimSpace(c.Database.Username) == "" {
		errors = append(errors, "database.username 不能为空")
	}
	if strings.TrimSpace(c.Database.Database) == "" {
		errors = append(errors, "database.database 不能为空")
	}
	if c.Database.MaxIdleConns < 0 {
		errors = append(errors, "database.max_idle_conns 不能为负数")
	}
	if c.Database.MaxOpenConns <= 0 {
		errors = append(errors, "database.max_open_conns 必须大于0")
	}
	if c.Database.ConnMaxLifetimeMinutes <= 0 {
		errors = append(errors, "database.conn_max_lifetime_minutes 必须大于0")
	}

	// --- Logging ---
	validLevels := []string{"debug", "info", "warn", "error"}
	levelValid := false
	for _, lvl := range validLevels {
		if c.LogConfig.Level == lvl {
			levelValid = true
			break
		}
	}
	if !levelValid {
		errors = append(errors, fmt.Sprintf("logging_config.level 必须是以下之一: %s", strings.Join(validLevels, ", ")))
	}

	if len(errors) > 0 {
		return fmt.Errorf("配置验证失败:\n- %s", strings.Join(errors, "\n- "))
	}
	return nil
}
