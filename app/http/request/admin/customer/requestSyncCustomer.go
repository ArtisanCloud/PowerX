package customer

import (
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/gin-gonic/gin"
)

type ParaSyncCustomer struct {
	EmployeeUserIDs []string `form:"employeeUserIDs" json:"employeeUserIDs"`
}

func ValidateSyncCustomer(context *gin.Context) {
	var form ParaSyncCustomer

	err := request.ValidatePara(context, &form)
	if err != nil {
		return
	}
	context.Set("employeeUserIDs", form.EmployeeUserIDs)
	context.Next()
}
