package sendChatMsg

import (
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/gin-gonic/gin"
)

type ParaSendChatMsgDetail struct {
	UUID string `form:"uuid" json:"uuid" xml:"uuid" binding:"required"`
}

func ValidateSendChatMsgDetail(context *gin.Context) {
	var form ParaSendChatMsgDetail

	err := request.ValidatePara(context, &form)
	if err != nil {
		return
	}

	context.Set("uuid", form.UUID)
	context.Next()
}
