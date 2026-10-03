package customer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"

	capabilityregistry "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	customerrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/customer"
)

const (
	CustomerAccountsServiceReadCapabilityID       = "com.corex.customer.accounts.service_read"
	CustomerAccountsServiceManageCapabilityID     = "com.corex.customer.accounts.service_manage"
	CustomerExternalIdentitiesResolveCapabilityID = "com.corex.customer.external_identities.resolve"

	customerAccountsCoreEndpoint            = "core://customer/accounts"
	customerExternalIdentityResolveEndpoint = "core://customer/external-identities/resolve"
)

// CapabilityInvoker exposes customer account management as a service_actor Core
// capability for /tenant/invocations without opening admin HTTP routes to STS.
type CapabilityInvoker struct {
	accounts *AccountService
	contacts *ContactCoreBinding
}

// ExternalIdentityResolveRequest is the Core-internal payload for a
// plugin-attested external customer subject. provider and membership source are
// deliberately absent: Core derives both from the STS plugin identity.
type ExternalIdentityResolveRequest struct {
	ProviderSubject string `json:"provider_subject"`
	DisplayName     string `json:"display_name"`
}

// ExternalIdentityResolveResult is deliberately minimal: a plugin receives the
// UUIDs it must persist plus a human label, but never unrelated customer PII.
type ExternalIdentityResolveResult struct {
	CustomerUUID       string `json:"customer_uuid"`
	MembershipUUID     string `json:"membership_uuid"`
	Type               string `json:"type"`
	PrimaryContactUUID string `json:"primary_contact_uuid"`
	DisplayName        string `json:"display_name"`
}

func (r ExternalIdentityResolveRequest) toServiceInput(tenantUUID string) ResolveExternalIdentityInput {
	return ResolveExternalIdentityInput{
		TenantUUID: tenantUUID, ProviderSubject: r.ProviderSubject, DisplayName: r.DisplayName,
	}
}

func NewCapabilityInvoker(accounts *AccountService, contacts ...*ContactService) *CapabilityInvoker {
	invoker := &CapabilityInvoker{accounts: accounts}
	if len(contacts) > 0 && contacts[0] != nil {
		invoker.contacts = NewContactCoreBinding(contacts[0])
	}
	return invoker
}

func (i *CapabilityInvoker) InvokeCoreCapability(ctx context.Context, in capabilityregistry.CoreCapabilityInvokeInput) (map[string]interface{}, error) {
	if i == nil || i.accounts == nil {
		return nil, errors.New("customer capability invoker unavailable")
	}
	method := strings.ToUpper(strings.TrimSpace(in.Method))
	endpoint := normalizeCustomerCapabilityEndpoint(in.Endpoint)
	if in.CapabilityID == CustomerExternalIdentitiesReadCapabilityID || in.CapabilityID == CustomerExternalIdentitiesManageCapabilityID {
		if method != "INVOKE" || endpoint != "core://customer/external-identities" {
			return nil, capabilityregistry.ErrCoreCapabilityNotHandled
		}
		raw, err := json.Marshal(in.Body)
		if err != nil {
			return nil, ErrCustomerAccountInvalidArgument
		}
		var request ExternalIdentityRequest
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&request); err != nil {
			return nil, ErrCustomerAccountInvalidArgument
		}
		return i.accounts.ManageExternalIdentity(ctx, in.CapabilityID, request)
	}
	if strings.EqualFold(strings.TrimSpace(in.CapabilityID), CustomerExternalIdentitiesResolveCapabilityID) {
		if method != "INVOKE" || endpoint != customerExternalIdentityResolveEndpoint {
			return nil, capabilityregistry.ErrCoreCapabilityNotHandled
		}
		request, err := decodeExternalIdentityResolveRequest(in.Body)
		if err != nil {
			return nil, err
		}
		item, err := i.accounts.ResolveExternalIdentity(ctx, request.toServiceInput(in.TenantUUID))
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{"item": ExternalIdentityResolveResult{
			CustomerUUID: item.CustomerUUID, MembershipUUID: item.MembershipUUID, Type: item.Type, PrimaryContactUUID: item.PrimaryContactUUID, DisplayName: item.DisplayName,
		}}, nil
	}
	if strings.EqualFold(strings.TrimSpace(in.CapabilityID), CustomerContactsServiceReadCapabilityID) || strings.EqualFold(strings.TrimSpace(in.CapabilityID), CustomerContactsServiceManageCapabilityID) {
		return i.invokeContactCapability(ctx, in, method, endpoint)
	}
	if !strings.EqualFold(strings.TrimSpace(in.CapabilityID), CustomerAccountsServiceReadCapabilityID) && !strings.EqualFold(strings.TrimSpace(in.CapabilityID), CustomerAccountsServiceManageCapabilityID) {
		return nil, capabilityregistry.ErrCoreCapabilityNotHandled
	}
	if method != "INVOKE" || endpoint != customerAccountsCoreEndpoint {
		return nil, capabilityregistry.ErrCoreCapabilityNotHandled
	}
	if in.CapabilityID == CustomerAccountsServiceManageCapabilityID {
		if in.Body["operation"] == "update" {
			request, err := decodeCustomerAccountUpdateRequest(in.Body)
			if err != nil {
				return nil, err
			}
			item, err := i.accounts.Update(ctx, UpdateAccountInput{TenantUUID: in.TenantUUID, CustomerUUID: request.CustomerUUID, AccountProfilePatch: request.AccountProfilePatch})
			if err != nil {
				return nil, err
			}
			return map[string]interface{}{"item": item}, nil
		}
		request, err := decodeCustomerAccountCreateRequest(in.Body)
		if err != nil {
			return nil, err
		}
		var primary *PrimaryContactInput
		if request.PrimaryContact != nil {
			primary = &PrimaryContactInput{DisplayName: request.PrimaryContact.DisplayName, GivenName: request.PrimaryContact.GivenName, FamilyName: request.PrimaryContact.FamilyName, Email: request.PrimaryContact.Email, Phone: request.PrimaryContact.Phone}
		}
		item, err := i.accounts.Create(ctx, CreateAccountInput{TenantUUID: in.TenantUUID, Type: request.Type, PrimaryContact: primary, Status: request.Status, PrimaryEmail: request.PrimaryEmail, PrimaryPhone: request.PrimaryPhone, DisplayName: request.DisplayName, Nickname: request.Nickname, GivenName: request.GivenName, FamilyName: request.FamilyName, AvatarURL: request.AvatarURL, Locale: request.Locale, Timezone: request.Timezone, MemberSource: "plugin"})
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{"item": item}, nil
	}
	request, err := decodeCustomerAccountListRequest(in.Body)
	if err != nil {
		return nil, err
	}
	items, total, err := i.accounts.List(ctx, ListAccountsInput{TenantUUID: in.TenantUUID, Query: request.Query, Status: request.Status, Page: request.Page, PageSize: request.PageSize})
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"items": items, "total": total, "page": request.Page, "page_size": request.PageSize}, nil
}

type customerAccountListRequest struct {
	Operation string `json:"operation"`
	Query     string `json:"q,omitempty"`
	Status    string `json:"status,omitempty"`
	Page      int    `json:"page,omitempty"`
	PageSize  int    `json:"page_size,omitempty"`
}

type customerAccountUpdateRequest struct {
	Operation    string `json:"operation"`
	CustomerUUID string `json:"customer_uuid"`
	AccountProfilePatch
}

func decodeCustomerAccountUpdateRequest(body map[string]interface{}) (customerAccountUpdateRequest, error) {
	var request customerAccountUpdateRequest
	// null 不是“不传”，也不代表清空；只接受字符串值和精确字段名。
	allowed := map[string]bool{"operation": true, "customer_uuid": true, "display_name": true, "nickname": true, "given_name": true, "family_name": true, "primary_email": true, "primary_phone": true, "avatar_url": true, "locale": true, "timezone": true, "status": true}
	for name, value := range body {
		if !allowed[name] {
			return request, ErrCustomerAccountInvalidArgument
		}
		if _, ok := value.(string); !ok {
			return request, ErrCustomerAccountInvalidArgument
		}
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return request, ErrCustomerAccountInvalidArgument
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil || request.Operation != "update" {
		return request, ErrCustomerAccountInvalidArgument
	}
	return request, nil
}

type customerAccountCreateRequest struct {
	Operation      string                 `json:"operation"`
	Type           string                 `json:"type"`
	PrimaryContact *primaryContactRequest `json:"primary_contact,omitempty"`
	Status         string                 `json:"status,omitempty"`
	PrimaryEmail   string                 `json:"primary_email,omitempty"`
	PrimaryPhone   string                 `json:"primary_phone,omitempty"`
	DisplayName    string                 `json:"display_name,omitempty"`
	Nickname       string                 `json:"nickname,omitempty"`
	GivenName      string                 `json:"given_name,omitempty"`
	FamilyName     string                 `json:"family_name,omitempty"`
	AvatarURL      string                 `json:"avatar_url,omitempty"`
	Locale         string                 `json:"locale,omitempty"`
	Timezone       string                 `json:"timezone,omitempty"`
}

type primaryContactRequest struct {
	DisplayName string `json:"display_name"`
	GivenName   string `json:"given_name,omitempty"`
	FamilyName  string `json:"family_name,omitempty"`
	Email       string `json:"email,omitempty"`
	Phone       string `json:"phone,omitempty"`
}

func decodeCustomerAccountCreateRequest(body map[string]interface{}) (customerAccountCreateRequest, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return customerAccountCreateRequest{}, ErrContactInvalidArgument
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var request customerAccountCreateRequest
	if err := decoder.Decode(&request); err != nil || request.Operation != "create" {
		return customerAccountCreateRequest{}, ErrContactInvalidArgument
	}
	return request, nil
}

func decodeCustomerAccountListRequest(body map[string]interface{}) (customerAccountListRequest, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return customerAccountListRequest{}, ErrContactInvalidArgument
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var request customerAccountListRequest
	if err := decoder.Decode(&request); err != nil || request.Operation != "list" {
		return customerAccountListRequest{}, ErrContactInvalidArgument
	}
	if request.Page <= 0 {
		request.Page = 1
	}
	if request.PageSize <= 0 {
		request.PageSize = 20
	}
	if request.PageSize > 100 {
		request.PageSize = 100
	}
	return request, nil
}

type contactCapabilityRequest struct {
	Operation                 string    `json:"operation"`
	CustomerUUID              string    `json:"customer_uuid"`
	ContactUUID               string    `json:"contact_uuid,omitempty"`
	Query                     string    `json:"q,omitempty"`
	Status                    *string   `json:"status,omitempty"`
	Page                      int       `json:"page,omitempty"`
	PageSize                  int       `json:"page_size,omitempty"`
	ChannelDictionaryItemUUID string    `json:"channel_dictionary_item_uuid,omitempty"`
	ExternalSubject           string    `json:"external_subject,omitempty"`
	DisplayName               *string   `json:"display_name,omitempty"`
	GivenName                 *string   `json:"given_name,omitempty"`
	FamilyName                *string   `json:"family_name,omitempty"`
	Email                     *string   `json:"email,omitempty"`
	Phone                     *string   `json:"phone,omitempty"`
	Roles                     *[]string `json:"roles,omitempty"`
	Tags                      *[]string `json:"tags,omitempty"`
	CreationIntent            string    `json:"creation_intent,omitempty"`
}

func (i *CapabilityInvoker) invokeContactCapability(ctx context.Context, in capabilityregistry.CoreCapabilityInvokeInput, method, endpoint string) (map[string]interface{}, error) {
	if i == nil || i.contacts == nil || method != "INVOKE" || endpoint != CustomerContactsCoreEndpoint {
		return nil, capabilityregistry.ErrCoreCapabilityNotHandled
	}
	raw, err := json.Marshal(in.Body)
	if err != nil {
		return nil, ErrContactInvalidArgument
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var req contactCapabilityRequest
	if err := decoder.Decode(&req); err != nil {
		return nil, ErrContactInvalidArgument
	}
	read := map[string]bool{"list_by_customer": true, "get": true, "resolve_identity": true}[req.Operation]
	manage := map[string]bool{"create": true, "update": true, "bind_identity": true}[req.Operation]
	if (!read && !manage) || (read && in.CapabilityID != CustomerContactsServiceReadCapabilityID) || (manage && in.CapabilityID != CustomerContactsServiceManageCapabilityID) {
		return nil, capabilityregistry.ErrCoreCapabilityNotHandled
	}
	switch req.Operation {
	case "list_by_customer":
		status := ""
		if req.Status != nil {
			status = *req.Status
		}
		page, err := i.contacts.ListByCustomer(ctx, in.TenantUUID, ContactCoreListInput{CustomerUUID: req.CustomerUUID, Query: req.Query, Status: status, Page: req.Page, PageSize: req.PageSize})
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{"items": page.Items, "total": page.Total, "page": page.Page, "page_size": page.PageSize}, nil
	case "get":
		item, err := i.contacts.Get(ctx, in.TenantUUID, ContactCoreGetInput{CustomerUUID: req.CustomerUUID, ContactUUID: req.ContactUUID})
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{"item": item}, nil
	case "resolve_identity":
		item, err := i.contacts.ResolveIdentity(ctx, in.TenantUUID, ContactCoreResolveIdentityInput{CustomerUUID: req.CustomerUUID, ChannelDictionaryItemUUID: req.ChannelDictionaryItemUUID, ExternalSubject: req.ExternalSubject})
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{"item": item}, nil
	case "create":
		if req.DisplayName == nil {
			return nil, ErrContactInvalidArgument
		}
		status := ""
		if req.Status != nil {
			status = *req.Status
		}
		item, err := i.contacts.Create(ctx, in.TenantUUID, ContactCoreCreateInput{CustomerUUID: req.CustomerUUID, DisplayName: *req.DisplayName, GivenName: deref(req.GivenName), FamilyName: deref(req.FamilyName), Email: deref(req.Email), Phone: deref(req.Phone), Status: status, Roles: derefSlice(req.Roles), Tags: derefSlice(req.Tags), CreationIntent: req.CreationIntent})
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{"item": item}, nil
	case "update":
		item, err := i.contacts.Update(ctx, in.TenantUUID, ContactCoreUpdateInput{CustomerUUID: req.CustomerUUID, ContactUUID: req.ContactUUID, DisplayName: req.DisplayName, GivenName: req.GivenName, FamilyName: req.FamilyName, Email: req.Email, Phone: req.Phone, Status: req.Status, Roles: req.Roles, Tags: req.Tags})
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{"item": item}, nil
	case "bind_identity":
		item, err := i.contacts.BindIdentity(ctx, in.TenantUUID, ContactCoreBindIdentityInput{CustomerUUID: req.CustomerUUID, ContactUUID: req.ContactUUID, ChannelDictionaryItemUUID: req.ChannelDictionaryItemUUID, ExternalSubject: req.ExternalSubject})
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{"item": item}, nil
	}
	return nil, capabilityregistry.ErrCoreCapabilityNotHandled
}

func deref(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
func derefSlice(value *[]string) []string {
	if value == nil {
		return nil
	}
	return *value
}

func normalizeCustomerCapabilityEndpoint(raw string) string {
	endpoint := strings.TrimSpace(raw)
	if endpoint == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(endpoint), "core://") {
		return strings.TrimSuffix(endpoint, "/")
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

func decodeExternalIdentityResolveRequest(body map[string]interface{}) (ExternalIdentityResolveRequest, error) {
	if len(body) == 0 {
		return ExternalIdentityResolveRequest{}, customerrepo.ErrExternalIdentityRequired
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return ExternalIdentityResolveRequest{}, customerrepo.ErrExternalIdentityRequired
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var request ExternalIdentityResolveRequest
	if err := decoder.Decode(&request); err != nil {
		return ExternalIdentityResolveRequest{}, customerrepo.ErrExternalIdentityRequired
	}
	return request, nil
}
