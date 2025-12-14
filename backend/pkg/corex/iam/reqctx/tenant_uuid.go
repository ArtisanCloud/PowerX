package reqctx

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// CanonicalTenantUUID 校验并标准化 tenant uuid 字符串，返回小写形式。
func CanonicalTenantUUID(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", ErrTenantUUIDMissing
	}
	parsed, err := uuid.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("invalid tenant uuid: %w", err)
	}
	return parsed.String(), nil
}
