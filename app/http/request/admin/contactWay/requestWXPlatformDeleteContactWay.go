package contactWay

import (
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/gin-gonic/gin"
)

type ParaWXPlatformDeleteContactWay struct {
	ConfigID string `form:"configID" json:"configID" xml:"configID" binding:"required"`
}

func ValidateWXPlatformDeleteContactWay(context *gin.Context) {
	var form ParaDeleteContactWay

	err := request.ValidatePara(context, &form)
	if err != nil {
		return
	}

	context.Set("configID", form.ConfigID)
	context.Next()
}
