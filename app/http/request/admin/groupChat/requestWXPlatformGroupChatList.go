package groupChat

import (
	"github.com/ArtisanCloud/PowerWeChat/v2/src/kernel/power"
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/gin-gonic/gin"
)

type ParaWXPlatformGroupChatList struct {
	StatusFilter int            `form:"statusFilter" json:"statusFilter" xml:"statusFilter"`
	OwnerFilter  *power.HashMap `form:"ownerFilter" json:"ownerFilter" xml:"ownerFilter"`
	Cursor       string         `form:"cursor" json:"cursor" xml:"cursor"`
	Limit        int            `form:"limit" json:"limit" xml:"limit"`
}

func ValidateWXPlatformGroupChatList(context *gin.Context) {
	var form ParaWXPlatformGroupChatList

	err := request.ValidatePara(context, &form)
	if err != nil {
		return
	}

	context.Set("params", &form)
	context.Next()
}
