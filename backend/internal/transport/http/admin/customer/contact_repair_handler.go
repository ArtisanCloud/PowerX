package customer

import (
	customersvc "github.com/ArtisanCloud/PowerX/internal/service/customer"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

func (h *Handler) repairAvailable(c *gin.Context) bool {
	c.Header("Cache-Control", "no-store")
	if h.repairs == nil {
		dto.RespondErrorFrom(c, dto.NewErrorWithCode(503, "CONTACT_REPAIR_UNAVAILABLE", "补录服务未加载", nil))
		return false
	}
	if c.Request.URL.RawQuery != "" {
		dto.RespondErrorFrom(c, dto.NewErrorWithCode(400, "CONTACT_REPAIR_REQUEST_INVALID", "补录不接受身份覆盖参数", nil))
		return false
	}
	return true
}
func (h *Handler) PreviewContactRepair(c *gin.Context) {
	if !h.repairAvailable(c) {
		return
	}
	var input customersvc.ContactRepairInput
	if !bindCustomerAdminJSON(c, &input, 65536, "CONTACT_REPAIR_REQUEST_INVALID") {
		return
	}
	result, err := h.repairs.Preview(c.Request.Context(), input)
	if err != nil {
		respondContactError(c, err)
		return
	}
	dto.ResponseSuccessWithStatus(c, 201, gin.H{"payload": result})
}
func (h *Handler) GetContactRepair(c *gin.Context) {
	if !h.repairAvailable(c) {
		return
	}
	result, err := h.repairs.Get(c.Request.Context(), c.Param("repair_uuid"))
	if err != nil {
		respondContactError(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"payload": result})
}
func (h *Handler) CommitContactRepair(c *gin.Context) {
	if !h.repairAvailable(c) {
		return
	}
	var request customerImportCommitRequest
	if !bindCustomerAdminJSON(c, &request, 4096, "CONTACT_REPAIR_REQUEST_INVALID") {
		return
	}
	result, err := h.repairs.Commit(c.Request.Context(), c.Param("repair_uuid"), request.Checksum, request.Confirm)
	if err != nil {
		respondContactError(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"payload": result})
}
