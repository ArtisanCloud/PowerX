package department

import (
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/gin-gonic/gin"
)

type ParaWXPlatformDepartmentList struct {
	DepartmentID *int `form:"departmentID" json:"departmentID" xml:"departmentID" binding:"required"`
}

func ValidateWXPlatformDepartmentList(context *gin.Context) {
	var form ParaWXPlatformDepartmentList

	err := request.ValidatePara(context, &form)
	if err != nil {
		return
	}

	context.Set("departmentID", form.DepartmentID)
	context.Next()
}
