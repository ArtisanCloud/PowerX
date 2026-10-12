package customer

import (
	"errors"
	"net/http"

	customersvc "github.com/ArtisanCloud/PowerX/internal/service/customer"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

type listContactsRequest struct {
	dto.PaginationRequest
	Q      string `form:"q"`
	Status string `form:"status"`
}

type createContactRequest struct {
	DisplayName    string   `json:"display_name" validate:"required,max=128"`
	GivenName      string   `json:"given_name" validate:"omitempty,max=128"`
	FamilyName     string   `json:"family_name" validate:"omitempty,max=128"`
	Email          string   `json:"email" validate:"omitempty,email,max=255"`
	Phone          string   `json:"phone" validate:"omitempty,max=32"`
	Status         string   `json:"status" validate:"omitempty,oneof=active inactive temporary"`
	Roles          []string `json:"roles"`
	Tags           []string `json:"tags"`
	CreationIntent string   `json:"creation_intent" validate:"required,oneof=explicit_create explicit_temporary"`
}

type updateContactRequest struct {
	DisplayName *string   `json:"display_name" validate:"omitempty,max=128"`
	GivenName   *string   `json:"given_name" validate:"omitempty,max=128"`
	FamilyName  *string   `json:"family_name" validate:"omitempty,max=128"`
	Email       *string   `json:"email" validate:"omitempty,email,max=255"`
	Phone       *string   `json:"phone" validate:"omitempty,max=32"`
	Status      *string   `json:"status" validate:"omitempty,oneof=active inactive temporary"`
	Roles       *[]string `json:"roles"`
	Tags        *[]string `json:"tags"`
}

type resolveContactIdentityRequest struct {
	ChannelDictionaryItemUUID string `json:"channel_dictionary_item_uuid" validate:"required,uuid4"`
	ExternalSubject           string `json:"external_subject" validate:"required,max=255"`
}

func (h *Handler) ListContacts(c *gin.Context) {
	tenantUUID, ok := requireTenant(c)
	if !ok {
		return
	}
	if _, _, err := h.contacts.AuthorizeAdmin(c.Request.Context(), "admin_customers_customer_uuid_contacts", "read"); err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	var req listContactsRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	req.SetDefaultPagination()
	page, err := h.contacts.ListByCustomer(c.Request.Context(), customersvc.ListContactsInput{TenantUUID: tenantUUID, CustomerUUID: c.Param("customer_uuid"), Query: req.Q, Status: req.Status, Page: req.Page, PageSize: req.PageSize})
	if err != nil {
		respondContactError(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"payload": gin.H{"items": page.Items, "pagination": gin.H{"total": page.Total, "page": page.Page, "page_size": page.PageSize}}})
}

func (h *Handler) CreateContact(c *gin.Context) {
	tenantUUID, ok := requireTenant(c)
	if !ok {
		return
	}
	if _, _, err := h.contacts.AuthorizeAdmin(c.Request.Context(), "admin_customers_customer_uuid_contacts", "create"); err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	var req createContactRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	contact, err := h.contacts.Create(c.Request.Context(), customersvc.CreateContactInput{TenantUUID: tenantUUID, CustomerUUID: c.Param("customer_uuid"), DisplayName: req.DisplayName, GivenName: req.GivenName, FamilyName: req.FamilyName, Email: req.Email, Phone: req.Phone, Status: req.Status, Roles: req.Roles, Tags: req.Tags, CreationIntent: req.CreationIntent})
	if err != nil {
		respondContactError(c, err)
		return
	}
	dto.ResponseSuccessWithStatus(c, http.StatusCreated, gin.H{"payload": contact})
}

func (h *Handler) GetContact(c *gin.Context) {
	tenantUUID, ok := requireTenant(c)
	if !ok {
		return
	}
	if _, _, err := h.contacts.AuthorizeAdmin(c.Request.Context(), "admin_customers_customer_uuid_contacts_contact_uuid", "read"); err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	contact, err := h.contacts.Get(c.Request.Context(), tenantUUID, c.Param("customer_uuid"), c.Param("contact_uuid"))
	if err != nil {
		respondContactError(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"payload": contact})
}

func (h *Handler) UpdateContact(c *gin.Context) {
	tenantUUID, ok := requireTenant(c)
	if !ok {
		return
	}
	if _, _, err := h.contacts.AuthorizeAdmin(c.Request.Context(), "admin_customers_customer_uuid_contacts_contact_uuid", "update"); err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	var req updateContactRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	contact, err := h.contacts.Update(c.Request.Context(), customersvc.UpdateContactInput{TenantUUID: tenantUUID, CustomerUUID: c.Param("customer_uuid"), ContactUUID: c.Param("contact_uuid"), DisplayName: req.DisplayName, GivenName: req.GivenName, FamilyName: req.FamilyName, Email: req.Email, Phone: req.Phone, Status: req.Status, Roles: req.Roles, Tags: req.Tags})
	if err != nil {
		respondContactError(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"payload": contact})
}

func (h *Handler) ResolveContactIdentity(c *gin.Context) {
	tenantUUID, ok := requireTenant(c)
	if !ok {
		return
	}
	if _, _, err := h.contacts.AuthorizeAdmin(c.Request.Context(), "admin_customers_customer_uuid_contactsresolve_identity", "create"); err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	var req resolveContactIdentityRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	result, err := h.contacts.ResolveIdentity(c.Request.Context(), customersvc.ResolveContactIdentityInput{TenantUUID: tenantUUID, CustomerUUID: c.Param("customer_uuid"), ChannelDictionaryItemUUID: req.ChannelDictionaryItemUUID, ExternalSubject: req.ExternalSubject})
	if err != nil {
		respondContactError(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"payload": result})
}

func (h *Handler) BindContactIdentity(c *gin.Context) {
	tenantUUID, ok := requireTenant(c)
	if !ok {
		return
	}
	if _, _, err := h.contacts.AuthorizeAdmin(c.Request.Context(), "admin_customers_customer_uuid_contacts_contact_uuid_identities", "create"); err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	var req resolveContactIdentityRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	identity, err := h.contacts.BindIdentity(c.Request.Context(), customersvc.BindContactIdentityInput{TenantUUID: tenantUUID, CustomerUUID: c.Param("customer_uuid"), ContactUUID: c.Param("contact_uuid"), ChannelDictionaryItemUUID: req.ChannelDictionaryItemUUID, ExternalSubject: req.ExternalSubject})
	if err != nil {
		respondContactError(c, err)
		return
	}
	dto.ResponseSuccessWithStatus(c, http.StatusCreated, gin.H{"payload": identity})
}

func (h *Handler) ListContactIdentities(c *gin.Context) {
	tenantUUID, ok := requireTenant(c)
	if !ok {
		return
	}
	if _, _, err := h.contacts.AuthorizeAdmin(c.Request.Context(), "admin_customers_customer_uuid_contacts_contact_uuid_identities", "read"); err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	items, err := h.contacts.ListIdentities(c.Request.Context(), tenantUUID, c.Param("customer_uuid"), c.Param("contact_uuid"))
	if err != nil {
		respondContactError(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"payload": gin.H{"items": items}})
}

func respondContactError(c *gin.Context, err error) {
	if dto.CodeOf(err) != "" {
		dto.RespondErrorFrom(c, err)
		return
	}
	switch {
	case errors.Is(err, customersvc.ErrContactInvalidArgument):
		dto.ResponseError(c, http.StatusBadRequest, "contact.invalid_argument", err)
	case errors.Is(err, customersvc.ErrContactNotFound), errors.Is(err, customersvc.ErrContactIdentityNotFound):
		dto.ResponseError(c, http.StatusNotFound, "contact.not_found", err)
	case errors.Is(err, customersvc.ErrContactCustomerMismatch):
		dto.ResponseError(c, http.StatusConflict, "contact.customer_mismatch", err)
	case errors.Is(err, customersvc.ErrContactIdentityConflict):
		dto.ResponseError(c, http.StatusConflict, "contact.identity_conflict", err)
	case errors.Is(err, customersvc.ErrContactChannelDictionaryInvalid):
		dto.ResponseError(c, http.StatusBadRequest, "contact.channel_dictionary_invalid", err)
	case errors.Is(err, customersvc.ErrContactIdentityChannelMigrationRequired):
		dto.ResponseError(c, http.StatusConflict, "contact.identity_channel_migration_required", err)
	case errors.Is(err, customersvc.ErrContactCustomerMembershipInactive):
		dto.ResponseError(c, http.StatusForbidden, "contact.customer_membership_inactive", err)
	default:
		dto.ResponseError(c, http.StatusInternalServerError, "contact.operation_failed", err)
	}
}

func (h *Handler) SetPrimaryContact(c *gin.Context) {
	tenant, ok := requireTenant(c)
	if !ok {
		return
	}
	if _, _, err := h.contacts.AuthorizeAdmin(c.Request.Context(), "admin_customers_customer_uuid_contacts_contact_uuid_primary", "create"); err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	var request struct {
		Expected *string `json:"expected_primary_contact_uuid"`
	}
	if !bindCustomerAdminJSON(c, &request, 4096, "CONTACT_REQUEST_INVALID") {
		return
	}
	if request.Expected == nil {
		dto.RespondErrorFrom(c, dto.NewErrorWithCode(400, "CONTACT_PRIMARY_EXPECTED_REQUIRED", "必须提交已确认的原主要联系人 UUID，未设定时使用空字符串", nil))
		return
	}
	result, err := h.contacts.SetPrimaryContact(c.Request.Context(), customersvc.SetPrimaryContactInput{TenantUUID: tenant, CustomerUUID: c.Param("customer_uuid"), ContactUUID: c.Param("contact_uuid"), ExpectedPrimaryContactUUID: *request.Expected})
	if err != nil {
		respondContactError(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"payload": result})
}
