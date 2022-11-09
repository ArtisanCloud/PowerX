package request

import (
	"github.com/gin-gonic/gin"
)

type ParaList struct {
	Page      int    `form:"page" json:"page" xml:"page" `
	PageSize  int    `form:"pageSize" json:"pageSize" xml:"pageSize" `
	RoleID    string `form:"roleID" json:"roleID" xml:"roleID" `
	GroupUUID string `form:"groupUUID" json:"groupUUID" xml:"groupUUID" `
}

func ValidateList(context *gin.Context) {
	var form ParaList

	err := ValidatePara(context, &form)
	if err != nil {
		return
	}

	context.Set("params", form)
	context.Next()
}
