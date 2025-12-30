package middleware

import "expvar"

var (
	tenantHeaderRejectCounter   = expvar.NewInt("tenant_header_reject_total")
	tenantUUIDOnlyRequestCounter = expvar.NewInt("tenant_uuid_only_request_total")
)

func incTenantHeaderReject() {
	tenantHeaderRejectCounter.Add(1)
}

func incTenantUUIDOnlyRequest() {
	tenantUUIDOnlyRequestCounter.Add(1)
}
