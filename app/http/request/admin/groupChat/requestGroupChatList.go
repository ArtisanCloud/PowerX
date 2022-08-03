package groupChat

import (
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/gin-gonic/gin"
)

type ParaGroupChatList struct {
	AdminUserID []string `form:"adminUserID" json:"adminUserID" xml:"adminUserID"`
	Name        string   `form:"name" json:"name" xml:"name"`
	TagIDs      []string `form:"tagIDs" json:"tagIDs" xml:"tagIDs"`
	SortBy      int8     `form:"sortBy" json:"sortBy" xml:"sortBy"`
	Ascend      bool     `form:"ascend" json:"ascend" xml:"ascend"`
	StartDate   string   `form:"startDate" json:"startDate" xml:"startDate"`
	Status      int8     `form:"status" json:"status" xml:"status"`
}

func ValidateGroupChatList(context *gin.Context) {
	var form ParaGroupChatList

	err := request.ValidatePara(context, &form)
	if err != nil {
		return
	}

	context.Set("params", &form)
	context.Next()
}
