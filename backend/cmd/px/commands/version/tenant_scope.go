package version

import (
	"net/url"
	"strings"
)

func withTenantScope(path, tenantUUID string) string {
	tid := strings.TrimSpace(tenantUUID)
	if tid == "" {
		return path
	}
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	return path + sep + "as_tenant_uuid=" + url.QueryEscape(tid)
}
