package customer

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	capabilityregistry "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
)

const CustomerAccountsAdminManageCapabilityID = "com.corex.customer.accounts.admin_manage"

// CapabilityInvoker exposes customer account management as a service_actor Core
// capability for /tenant/invocations without opening admin HTTP routes to STS.
type CapabilityInvoker struct {
	accounts *AccountService
}

func NewCapabilityInvoker(accounts *AccountService) *CapabilityInvoker {
	return &CapabilityInvoker{accounts: accounts}
}

func (i *CapabilityInvoker) InvokeCoreCapability(ctx context.Context, in capabilityregistry.CoreCapabilityInvokeInput) (map[string]interface{}, error) {
	if i == nil || i.accounts == nil {
		return nil, errors.New("customer capability invoker unavailable")
	}
	if !strings.EqualFold(strings.TrimSpace(in.CapabilityID), CustomerAccountsAdminManageCapabilityID) {
		return nil, capabilityregistry.ErrCoreCapabilityNotHandled
	}
	method := strings.ToUpper(strings.TrimSpace(in.Method))
	endpoint := normalizeCustomerCapabilityEndpoint(in.Endpoint)
	switch {
	case method == http.MethodGet && endpoint == "/api/v1/admin/customers/overview":
		overview, err := i.accounts.Overview(ctx, in.TenantUUID)
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{
			"total":     overview.Total,
			"active":    overview.Active,
			"pending":   overview.Pending,
			"suspended": overview.Suspended,
			"disabled":  overview.Disabled,
		}, nil
	case method == http.MethodGet && endpoint == "/api/v1/admin/customers/accounts":
		items, total, err := i.accounts.List(ctx, ListAccountsInput{
			TenantUUID: in.TenantUUID,
			Query:      strings.TrimSpace(in.Query["q"]),
			Status:     strings.TrimSpace(in.Query["status"]),
			Page:       positiveInt(in.Query["page"], 1),
			PageSize:   positiveInt(in.Query["page_size"], 20),
			SortBy:     strings.TrimSpace(in.Query["sort_by"]),
			SortOrder:  strings.TrimSpace(in.Query["sort_order"]),
		})
		if err != nil {
			return nil, err
		}
		page := positiveInt(in.Query["page"], 1)
		pageSize := positiveInt(in.Query["page_size"], 20)
		if pageSize > 100 {
			pageSize = 100
		}
		return map[string]interface{}{
			"items": items,
			"pagination": map[string]interface{}{
				"total":     total,
				"page":      page,
				"page_size": pageSize,
			},
		}, nil
	case method == http.MethodPost && endpoint == "/api/v1/admin/customers/accounts":
		item, err := i.accounts.Create(ctx, CreateAccountInput{
			TenantUUID:   in.TenantUUID,
			Status:       stringFromBody(in.Body, "status"),
			PrimaryEmail: stringFromBody(in.Body, "primary_email"),
			PrimaryPhone: stringFromBody(in.Body, "primary_phone"),
			DisplayName:  stringFromBody(in.Body, "display_name"),
			Nickname:     stringFromBody(in.Body, "nickname"),
			GivenName:    stringFromBody(in.Body, "given_name"),
			FamilyName:   stringFromBody(in.Body, "family_name"),
			AvatarURL:    stringFromBody(in.Body, "avatar_url"),
			Locale:       stringFromBody(in.Body, "locale"),
			Timezone:     stringFromBody(in.Body, "timezone"),
			MemberSource: stringFromBody(in.Body, "member_source"),
		})
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{"item": item}, nil
	case method == http.MethodPatch && strings.HasPrefix(endpoint, "/api/v1/admin/customers/accounts/") && strings.HasSuffix(endpoint, "/status"):
		customerUUID := customerUUIDFromStatusEndpoint(endpoint)
		status := stringFromBody(in.Body, "status")
		if err := i.accounts.UpdateStatus(ctx, in.TenantUUID, customerUUID, status); err != nil {
			return nil, err
		}
		return map[string]interface{}{
			"customer_uuid": customerUUID,
			"status":        strings.TrimSpace(status),
		}, nil
	default:
		return nil, capabilityregistry.ErrCoreCapabilityNotHandled
	}
}

func normalizeCustomerCapabilityEndpoint(raw string) string {
	endpoint := strings.TrimSpace(raw)
	if endpoint == "" {
		return ""
	}
	if !strings.HasPrefix(endpoint, "/") {
		endpoint = "/" + endpoint
	}
	for strings.Contains(endpoint, "//") {
		endpoint = strings.ReplaceAll(endpoint, "//", "/")
	}
	if len(endpoint) > 1 && strings.HasSuffix(endpoint, "/") {
		endpoint = strings.TrimSuffix(endpoint, "/")
	}
	return endpoint
}

func positiveInt(raw string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func stringFromBody(body map[string]interface{}, key string) string {
	if len(body) == 0 {
		return ""
	}
	return strings.TrimSpace(toString(body[key]))
}

func toString(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return typed
	case nil:
		return ""
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func customerUUIDFromStatusEndpoint(endpoint string) string {
	prefix := "/api/v1/admin/customers/accounts/"
	suffix := "/status"
	if !strings.HasPrefix(endpoint, prefix) || !strings.HasSuffix(endpoint, suffix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(endpoint, prefix), suffix))
}
