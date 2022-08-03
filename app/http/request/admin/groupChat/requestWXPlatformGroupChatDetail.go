package groupChat

import (
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/gin-gonic/gin"
)

type ParaWXPlatformGroupChatDetail struct {
	ChatID   string `form:"chatID" json:"chatID" xml:"chatID" binding:"required"`
	NeedName int    `form:"needName" json:"needName" xml:"needName" binding:"required"`
}

func ValidateWXPlatformGroupChatDetail(context *gin.Context) {
	var form ParaWXPlatformGroupChatDetail

	err := request.ValidatePara(context, &form)
	if err != nil {
		return
	}

	context.Set("params", &form)
	context.Next()
}
