package customer

import (
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/gin-gonic/gin"
)

type ParaCustomerDetail struct {
	ExternalUserID string `form:"externalUserID" json:"externalUserID" xml:"externalUserID" binding:"required"`
}

func ValidateCustomerDetail(context *gin.Context) {
	var form ParaCustomerDetail

	err := request.ValidatePara(context, &form)
	if err != nil {
		return
	}

	context.Set("externalUserID", form.ExternalUserID)
	context.Next()
}
