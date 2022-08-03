package customer

import (
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/gin-gonic/gin"
)

type ParaGetCustomerList struct {
	EmployeeID string `form:"employeeID" json:"employeeID" xml:"employeeID"  binding:"required"`
}

func ValidateGetCustomerList(context *gin.Context) {
	var form ParaGetCustomerList

	err := request.ValidatePara(context, &form)
	if err != nil {
		return
	}

	context.Set("params", form)
	context.Next()
}
