package request

import (
	"github.com/gin-gonic/gin"
)

type ParaDelete struct {
	UUIDs []string `form:"uuids" json:"uuids" xml:"uuids" binding:"required"`
}

func ValidateDelete(context *gin.Context) {
	var form ParaDelete

	err := ValidatePara(context, &form)
	if err != nil {
		return
	}

	context.Set("uuids", form.UUIDs)
	context.Next()
}
