package customer

import (
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/gin-gonic/gin"
)

type ParaUpsertCustomer struct {
	EncryptedData string `form:"encryptedData" json:"encryptedData" xml:"encryptedData" binding:"required"`
	IV            string `form:"iv" json:"iv" xml:"iv" binding:"required"`
}

func ValidateUpsertCustomer(context *gin.Context) {
	var form ParaUpsertCustomer

	err := request.ValidatePara(context, &form)
	if err != nil {
		return
	}

	context.Set("params", &form)
	context.Next()
}
