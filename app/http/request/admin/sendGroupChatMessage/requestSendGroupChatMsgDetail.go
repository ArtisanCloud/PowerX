package sendGroupChatMsg

import (
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/gin-gonic/gin"
)

type ParaSendGroupChatMsgDetail struct {
	UUID string `form:"uuid" json:"uuid" xml:"uuid" binding:"required"`
}

func ValidateSendGroupChatMsgDetail(context *gin.Context) {
	var form ParaSendGroupChatMsgDetail

	err := request.ValidatePara(context, &form)
	if err != nil {
		return
	}

	context.Set("uuid", form.UUID)
	context.Next()
}
