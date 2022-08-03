package customer

import (
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/gin-gonic/gin"
)

type ParaWXPlatformCustomerList struct {
	UserID string `form:"userID" json:"userID" xml:"userID" binding:"required"`
}

func ValidateWXPlatformCustomerList(context *gin.Context) {
	var form ParaWXPlatformCustomerList

	err := request.ValidatePara(context, &form)
	if err != nil {
		return
	}

	context.Set("userID", form.UserID)
	context.Next()
}
