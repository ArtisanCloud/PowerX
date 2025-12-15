package authorization

import (
	"strings"

	eventfabricmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
)

func tenantUUIDFromGrant(grant *eventfabricmodel.AuthorizationGrant) string {
	if grant == nil {
		return ""
	}
	return canonicalTenantKey(grant.TenantUUID)
}

func tenantUUIDFromTicket(ticket *eventfabricmodel.AuthorizationApprovalTicket) string {
	if ticket == nil {
		return ""
	}
	return canonicalTenantKey(ticket.TenantUUID)
}

func tenantUUIDFromTemplate(tmpl *eventfabricmodel.AuthorizationGrantTemplate) string {
	if tmpl == nil {
		return ""
	}
	if tmpl.TenantUUID == nil {
		return ""
	}
	return canonicalTenantKey(*tmpl.TenantUUID)
}

func tenantUUIDFromSnapshot(snapshot *GrantSnapshot) string {
	if snapshot == nil {
		return ""
	}
	return canonicalTenantKey(snapshot.TenantUUID)
}

func canonicalTenantKey(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	return strings.ToLower(trimmed)
}
