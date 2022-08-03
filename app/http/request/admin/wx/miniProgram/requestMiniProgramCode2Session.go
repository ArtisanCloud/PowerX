package miniProgram

import (
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/gin-gonic/gin"
)

type ParaMiniProgramCode2Session struct {
	Code string `form:"code" json:"code" xml:"code" binding:"required"`
}

func ValidateMiniProgramCode2Session(context *gin.Context) {
	var form ParaMiniProgramCode2Session

	err := request.ValidatePara(context, &form)
	if err != nil {
		return
	}

	context.Set("para", &form)
	context.Next()
}
