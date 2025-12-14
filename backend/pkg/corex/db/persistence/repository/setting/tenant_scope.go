package setting

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

type TenantScope struct {
	TenantUUID string
}

func (s TenantScope) apply(db *gorm.DB) *gorm.DB {
	tenantUUID := strings.TrimSpace(strings.ToLower(s.TenantUUID))
	if tenantUUID != "" {
		return db.Where("tenant_uuid = ?", tenantUUID)
	}
	return db.Where("(tenant_uuid = '' OR tenant_uuid IS NULL)")
}

func canonicalTenantUUIDStrict(raw string) (string, error) {
	tenantUUID := canonicalTenantUUIDAllowEmpty(raw)
	if tenantUUID == "" {
		return "", fmt.Errorf("tenant uuid is required")
	}
	return tenantUUID, nil
}

func canonicalTenantUUIDAllowEmpty(raw string) string {
	return strings.TrimSpace(strings.ToLower(raw))
}
