package config

import (
	"fmt"
	"strings"
)

// legacyDefaultJWTSecret is public and must never be accepted as a signing key,
// including when copied into an existing deployment's configuration.
const legacyDefaultJWTSecret = "K8mN2pQ7rS9tU4vW6xY1zA3bC5dE8fG0"

func validateJWTSecret(secret string) error {
	trimmed := strings.TrimSpace(secret)
	if trimmed == "" {
		return fmt.Errorf("auth.jwt_secret 必须显式配置：请为每个部署生成并持久保存独立随机密钥，或设置 CORE_X_AUTH_JWT_SECRET")
	}
	switch trimmed {
	case legacyDefaultJWTSecret,
		"CHANGE_ME_WITH_A_RANDOM_SECRET_AT_LEAST_32_CHARS",
		"REPLACE_WITH_A_RANDOM_JWT_SECRET_AT_LEAST_32_CHARS":
		return fmt.Errorf("auth.jwt_secret 禁止使用公开默认值或示例占位值：请配置独立随机密钥")
	}
	if len(secret) < 32 {
		return fmt.Errorf("auth.jwt_secret 长度至少32个字符")
	}
	return nil
}
