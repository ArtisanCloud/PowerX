package employee

import (
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/gin-gonic/gin"
)

type ParaEmployeeDetail struct {
	UserID string `form:"userID" json:"userID" xml:"userID" binding:"required"`
}

func ValidateEmployeeDetail(context *gin.Context) {
	var form ParaEmployeeDetail

	err := request.ValidatePara(context, &form)
	if err != nil {
		return
	}

	context.Set("userID", form.UserID)
	context.Next()
}
