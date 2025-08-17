package config

import (
	"fmt"
	"strings"
)

// Validate 验证配置的合法性
func (c *Config) Validate() error {
	var errors []string

	// 验证服务器配置
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		errors = append(errors, "server.port 必须在 1-65535 范围内")
	}

	// 验证认证配置
	if strings.TrimSpace(c.Auth.JWTSecret) == "" {
		errors = append(errors, "auth.jwt_secret 不能为空")
	}
	if len(c.Auth.JWTSecret) < 32 {
		errors = append(errors, "auth.jwt_secret 长度至少32个字符")
	}
	if c.Auth.TokenTTLHours <= 0 {
		errors = append(errors, "auth.token_ttl_hours 必须大于0")
	}

	// 验证事件总线配置
	if c.EventBus.Type != "local" && c.EventBus.Type != "redis" {
		errors = append(errors, "event_bus.type 必须是 'local' 或 'redis'")
	}
	if c.EventBus.Type == "redis" && strings.TrimSpace(c.EventBus.RedisAddr) == "" {
		errors = append(errors, "使用redis事件总线时，redis_addr 不能为空")
	}

	// 验证低代码配置
	if c.LowCode.MaxConcurrentFlows <= 0 {
		errors = append(errors, "dynamic_form.max_concurrent_flows 必须大于0")
	}
	if c.LowCode.DefaultTimeoutSec <= 0 {
		errors = append(errors, "dynamic_form.default_timeout_sec 必须大于0")
	}

	// 验证数据库配置
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

	// 验证日志配置
	validLevels := []string{"debug", "info", "warn", "error"}
	levelValid := false
	for _, level := range validLevels {
		if c.LogConfig.Level == level {
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
