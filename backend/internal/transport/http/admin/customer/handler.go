package customer

import (
	"errors"
	"net/http"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	customersvc "github.com/ArtisanCloud/PowerX/internal/service/customer"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type Handler struct {
	svc *customersvc.AccountService
}

func NewHandler(deps *shared.Deps) *Handler {
	return &Handler{svc: customersvc.NewAccountService(deps.DB)}
}

type listAccountsRequest struct {
	dto.PaginationRequest
	Q      string `form:"q"`
	Status string `form:"status"`
}

type createAccountRequest struct {
	Status       string `json:"status"`
	PrimaryEmail string `json:"primary_email"`
	PrimaryPhone string `json:"primary_phone"`
	DisplayName  string `json:"display_name"`
	Nickname     string `json:"nickname"`
	GivenName    string `json:"given_name"`
	FamilyName   string `json:"family_name"`
	AvatarURL    string `json:"avatar_url"`
	Locale       string `json:"locale"`
	Timezone     string `json:"timezone"`
	MemberSource string `json:"member_source"`
}

type updateStatusRequest struct {
	Status string `json:"status" validate:"required"`
}

func (h *Handler) Overview(c *gin.Context) {
	tenantUUID, ok := requireTenant(c)
	if !ok {
		return
	}
	overview, err := h.svc.Overview(c.Request.Context(), tenantUUID)
	if err != nil {
		respondCustomerError(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"payload": overview})
}

func (h *Handler) ListAccounts(c *gin.Context) {
	tenantUUID, ok := requireTenant(c)
	if !ok {
		return
	}
	var req listAccountsRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	req.SetDefaultPagination()
	items, total, err := h.svc.List(c.Request.Context(), customersvc.ListAccountsInput{
		TenantUUID: tenantUUID,
		Query:      req.Q,
		Status:     req.Status,
		Page:       req.Page,
		PageSize:   req.PageSize,
		SortBy:     req.SortBy,
		SortOrder:  req.SortOrder,
	})
	if err != nil {
		respondCustomerError(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{
		"payload": gin.H{
			"items": items,
			"pagination": gin.H{
				"total":     total,
				"page":      req.Page,
				"page_size": req.PageSize,
			},
		},
	})
}

func (h *Handler) CreateAccount(c *gin.Context) {
	tenantUUID, ok := requireTenant(c)
	if !ok {
		return
	}
	var req createAccountRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	item, err := h.svc.Create(c.Request.Context(), customersvc.CreateAccountInput{
		TenantUUID:   tenantUUID,
		Status:       req.Status,
		PrimaryEmail: req.PrimaryEmail,
		PrimaryPhone: req.PrimaryPhone,
		DisplayName:  req.DisplayName,
		Nickname:     req.Nickname,
		GivenName:    req.GivenName,
		FamilyName:   req.FamilyName,
		AvatarURL:    req.AvatarURL,
		Locale:       req.Locale,
		Timezone:     req.Timezone,
		MemberSource: req.MemberSource,
	})
	if err != nil {
		respondCustomerError(c, err)
		return
	}
	dto.ResponseSuccessWithStatus(c, http.StatusCreated, gin.H{"payload": item})
}

func (h *Handler) UpdateStatus(c *gin.Context) {
	tenantUUID, ok := requireTenant(c)
	if !ok {
		return
	}
	var req updateStatusRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	if err := h.svc.UpdateStatus(c.Request.Context(), tenantUUID, c.Param("customer_uuid"), req.Status); err != nil {
		respondCustomerError(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"payload": gin.H{"customer_uuid": strings.TrimSpace(c.Param("customer_uuid")), "status": strings.TrimSpace(req.Status)}})
}

func requireTenant(c *gin.Context) (string, bool) {
	tenantUUID := strings.TrimSpace(reqctx.GetTenantUUID(c.Request.Context()))
	if tenantUUID == "" {
		dto.ResponseError(c, http.StatusBadRequest, "tenant.uuid_required", nil)
		return "", false
	}
	return tenantUUID, true
}

func respondCustomerError(c *gin.Context, err error) {
	if err == nil {
		return
	}
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		dto.ResponseError(c, http.StatusNotFound, "customer.not_found", err)
	case strings.Contains(err.Error(), "invalid_status"):
		dto.ResponseError(c, http.StatusBadRequest, "customer.invalid_status", err)
	case strings.Contains(err.Error(), "identity_required"):
		dto.ResponseError(c, http.StatusBadRequest, "customer.identity_required", err)
	default:
		dto.ResponseError(c, http.StatusInternalServerError, "customer.operation_failed", err)
	}
}
