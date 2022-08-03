package employee

import (
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/gin-gonic/gin"
)

type ParaWXPlatformEmployeeList struct {
	DepartmentID int `form:"departmentID" json:"departmentID" xml:"departmentID"`
}

func ValidateWXPlatformEmployeeList(context *gin.Context) {
	var form ParaWXPlatformEmployeeList

	err := request.ValidatePara(context, &form)
	if err != nil {
		return
	}

	context.Set("departmentID", form.DepartmentID)
	context.Next()
}
