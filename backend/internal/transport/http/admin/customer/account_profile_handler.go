package customer

import (
	customersvc "github.com/ArtisanCloud/PowerX/internal/service/customer"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

type updateAccountProfileRequest struct {
	ExpectedProfileVersion string  `json:"expected_profile_version"`
	DisplayName            *string `json:"display_name"`
	Nickname               *string `json:"nickname"`
	PrimaryEmail           *string `json:"primary_email"`
	PrimaryPhone           *string `json:"primary_phone"`
}

func (h *Handler) UpdateAccountProfile(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if _, ok := requireTenant(c); !ok {
		return
	}
	if c.Request.URL.RawQuery != "" {
		dto.RespondErrorFrom(c, dto.NewErrorWithCode(400, "CUSTOMER_PROFILE_REQUEST_INVALID", "资料编辑不接受查询参数或身份覆盖", nil))
		return
	}
	var request updateAccountProfileRequest
	if !bindCustomerAdminJSON(c, &request, 4096, "CUSTOMER_PROFILE_REQUEST_INVALID") {
		return
	}
	result, err := h.svc.UpdateProfileFromAdmin(c.Request.Context(), customersvc.AdminAccountProfileInput{CustomerUUID: c.Param("customer_uuid"), ExpectedProfileVersion: request.ExpectedProfileVersion, DisplayName: request.DisplayName, Nickname: request.Nickname, PrimaryEmail: request.PrimaryEmail, PrimaryPhone: request.PrimaryPhone})
	if err != nil {
		respondCustomerError(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"payload": result})
}
