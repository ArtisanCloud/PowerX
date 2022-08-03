package tag

import (
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/gin-gonic/gin"
)

type ParaWXTagGroupList struct {
	WXDepartmentID int `form:"wxDepartmentID" json:"wxDepartmentID"`
}

func ValidateWXTagGroupList(context *gin.Context) {
	var form ParaWXTagGroupList

	err := request.ValidatePara(context, &form)
	if err != nil {
		return
	}
	context.Set("wxDepartmentID", form.WXDepartmentID)
	context.Next()
}
