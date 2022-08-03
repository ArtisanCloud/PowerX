package groupChat

import (
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/gin-gonic/gin"
)

type ParaGroupChatDetail struct {
	ChatID string `form:"chatID" json:"chatID" xml:"chatID"`
}

func ValidateGroupChatDetail(context *gin.Context) {
	var form ParaGroupChatDetail

	err := request.ValidatePara(context, &form)
	if err != nil {
		return
	}

	context.Set("chatID", form.ChatID)
	context.Next()
}
