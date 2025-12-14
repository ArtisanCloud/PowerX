package agentlifecyclecontract

import "net/http"

const defaultTenantUUID = "tenant-001"

func applyTenantHeaders(req *http.Request, tenantUUID string) {
	if req == nil {
		return
	}
	if tenantUUID == "" {
		tenantUUID = defaultTenantUUID
	}
	if req.Header.Get("Authorization") == "" {
		req.Header.Set("Authorization", "Bearer token")
	}
	req.Header.Set("X-Tenant-UUID", tenantUUID)
}
