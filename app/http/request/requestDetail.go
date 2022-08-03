package request

import (
	"github.com/gin-gonic/gin"
)

type ParaDetail struct {
	UUID string `form:"uuid" json:"uuid" xml:"uuid" binding:"required"`
}

func ValidateDetail(context *gin.Context) {
	var form ParaDetail

	err := ValidatePara(context, &form)
	if err != nil {
		return
	}

	context.Set("params", form)
	context.Next()
}
