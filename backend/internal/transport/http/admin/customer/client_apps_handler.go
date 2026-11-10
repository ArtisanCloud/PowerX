package customer

import (
	"strconv"

	customersvc "github.com/ArtisanCloud/PowerX/internal/service/customer"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

func (h *Handler) ClientAppsCatalog(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	data, err := h.clientApps.Catalog(c.Request.Context())
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"payload": data})
}
func (h *Handler) ListClientApps(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	items, total, err := h.clientApps.List(c.Request.Context(), c.Query("kind"), c.Query("provider"), c.Query("q"), page, size)
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 20
	}
	dto.ResponseSuccess(c, gin.H{"payload": gin.H{"items": items, "pagination": gin.H{"total": total, "page": page, "page_size": size}}})
}
func (h *Handler) GetClientApp(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	item, err := h.clientApps.Get(c.Request.Context(), c.Param("app_uuid"))
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"payload": item})
}
func (h *Handler) SaveClientApp(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if c.Request.URL.RawQuery != "" {
		dto.RespondErrorFrom(c, dto.NewErrorWithCode(400, "CLIENT_APP_INVALID_ARGUMENT", "CLIENT_APP_INVALID_ARGUMENT", nil))
		return
	}
	var input customersvc.ClientApplicationInput
	if !bindCustomerAdminJSON(c, &input, 32<<10, "CLIENT_APP_INVALID_ARGUMENT") {
		return
	}
	item, err := h.clientApps.Save(c.Request.Context(), c.Param("app_uuid"), input)
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"payload": item})
}
func (h *Handler) ImportClientApp(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var input struct {
		Platform string `json:"platform"`
		AppID    string `json:"app_id"`
	}
	if c.Request.URL.RawQuery != "" {
		dto.RespondErrorFrom(c, dto.NewErrorWithCode(400, "CLIENT_APP_INVALID_ARGUMENT", "CLIENT_APP_INVALID_ARGUMENT", nil))
		return
	}
	if !bindCustomerAdminJSON(c, &input, 4096, "CLIENT_APP_INVALID_ARGUMENT") {
		return
	}
	item, err := h.clientApps.ImportLegacy(c.Request.Context(), input.Platform, input.AppID)
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"payload": item})
}
