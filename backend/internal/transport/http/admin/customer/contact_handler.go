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
	Status         string   `json:"status" validate:"omitempty,oneof=active inactive temporary"`
	Roles          []string `json:"roles"`
	Tags           []string `json:"tags"`
	CreationIntent string   `json:"creation_intent" validate:"required,oneof=explicit_create explicit_temporary"`
}

type updateContactRequest struct {
	DisplayName *string   `json:"display_name" validate:"omitempty,max=128"`
	GivenName   *string   `json:"given_name" validate:"omitempty,max=128"`
	FamilyName  *string   `json:"family_name" validate:"omitempty,max=128"`
	Status      *string   `json:"status" validate:"omitempty,oneof=active inactive temporary"`
	Roles       *[]string `json:"roles"`
	Tags        *[]string `json:"tags"`
}

type resolveContactIdentityRequest struct {
	Channel         string `json:"channel" validate:"required,max=64"`
	ExternalSubject string `json:"external_subject" validate:"required,max=255"`
}

func (h *Handler) ListContacts(c *gin.Context) {
	tenantUUID, ok := requireTenant(c)
	if !ok {
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
	var req createContactRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	contact, err := h.contacts.Create(c.Request.Context(), customersvc.CreateContactInput{TenantUUID: tenantUUID, CustomerUUID: c.Param("customer_uuid"), DisplayName: req.DisplayName, GivenName: req.GivenName, FamilyName: req.FamilyName, Status: req.Status, Roles: req.Roles, Tags: req.Tags, CreationIntent: req.CreationIntent})
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
	var req updateContactRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	contact, err := h.contacts.Update(c.Request.Context(), customersvc.UpdateContactInput{TenantUUID: tenantUUID, CustomerUUID: c.Param("customer_uuid"), ContactUUID: c.Param("contact_uuid"), DisplayName: req.DisplayName, GivenName: req.GivenName, FamilyName: req.FamilyName, Status: req.Status, Roles: req.Roles, Tags: req.Tags})
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
	var req resolveContactIdentityRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	result, err := h.contacts.ResolveIdentity(c.Request.Context(), customersvc.ResolveContactIdentityInput{TenantUUID: tenantUUID, CustomerUUID: c.Param("customer_uuid"), Channel: req.Channel, ExternalSubject: req.ExternalSubject})
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
	var req resolveContactIdentityRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	identity, err := h.contacts.BindIdentity(c.Request.Context(), customersvc.BindContactIdentityInput{TenantUUID: tenantUUID, CustomerUUID: c.Param("customer_uuid"), ContactUUID: c.Param("contact_uuid"), Channel: req.Channel, ExternalSubject: req.ExternalSubject})
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
	items, err := h.contacts.ListIdentities(c.Request.Context(), tenantUUID, c.Param("customer_uuid"), c.Param("contact_uuid"))
	if err != nil {
		respondContactError(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"payload": gin.H{"items": items}})
}

func respondContactError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, customersvc.ErrContactInvalidArgument):
		dto.ResponseError(c, http.StatusBadRequest, "contact.invalid_argument", err)
	case errors.Is(err, customersvc.ErrContactNotFound), errors.Is(err, customersvc.ErrContactIdentityNotFound):
		dto.ResponseError(c, http.StatusNotFound, "contact.not_found", err)
	case errors.Is(err, customersvc.ErrContactCustomerMismatch):
		dto.ResponseError(c, http.StatusConflict, "contact.customer_mismatch", err)
	case errors.Is(err, customersvc.ErrContactIdentityConflict):
		dto.ResponseError(c, http.StatusConflict, "contact.identity_conflict", err)
	case errors.Is(err, customersvc.ErrContactCustomerMembershipInactive):
		dto.ResponseError(c, http.StatusForbidden, "contact.customer_membership_inactive", err)
	default:
		dto.ResponseError(c, http.StatusInternalServerError, "contact.operation_failed", err)
	}
}
