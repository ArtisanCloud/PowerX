package customer

import (
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

type masterDataPageRequest struct{ dto.PaginationRequest }

func (h *Handler) GetAccount(c *gin.Context) {
	tenantUUID, ok := requireTenant(c)
	if !ok {
		return
	}
	account, err := h.svc.Get(c.Request.Context(), tenantUUID, c.Param("customer_uuid"))
	if err != nil {
		respondCustomerError(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"payload": account})
}

func (h *Handler) ListAuthIdentities(c *gin.Context) {
	tenantUUID, ok := requireTenant(c)
	if !ok {
		return
	}
	items, err := h.svc.ListAuthIdentities(c.Request.Context(), tenantUUID, c.Param("customer_uuid"))
	if err != nil {
		respondCustomerError(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"payload": gin.H{"items": items}})
}

func (h *Handler) ListMemberships(c *gin.Context) {
	tenantUUID, ok := requireTenant(c)
	if !ok {
		return
	}
	items, err := h.svc.ListMemberships(c.Request.Context(), tenantUUID, c.Param("customer_uuid"))
	if err != nil {
		respondCustomerError(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"payload": gin.H{"items": items}})
}

func (h *Handler) ListLoginEvents(c *gin.Context) {
	tenantUUID, ok := requireTenant(c)
	if !ok {
		return
	}
	var req masterDataPageRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	req.SetDefaultPagination()
	items, total, err := h.svc.ListLoginEvents(c.Request.Context(), tenantUUID, c.Param("customer_uuid"), req.Page, req.PageSize)
	if err != nil {
		respondCustomerError(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"payload": gin.H{"items": items, "pagination": gin.H{"total": total, "page": req.Page, "page_size": req.PageSize}}})
}

func (h *Handler) ListMiniAppEntries(c *gin.Context) {
	tenantUUID, ok := requireTenant(c)
	if !ok {
		return
	}
	var req masterDataPageRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	req.SetDefaultPagination()
	items, total, err := h.svc.ListMiniAppEntries(c.Request.Context(), tenantUUID, req.Page, req.PageSize)
	if err != nil {
		respondCustomerError(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"payload": gin.H{"items": items, "pagination": gin.H{"total": total, "page": req.Page, "page_size": req.PageSize}}})
}
