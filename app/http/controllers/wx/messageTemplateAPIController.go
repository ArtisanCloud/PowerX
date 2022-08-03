package wx

import (
	"github.com/ArtisanCloud/PowerX/app/http/controllers/api"
	"github.com/ArtisanCloud/PowerX/app/service/wx/wecom"
	"github.com/gin-gonic/gin"
)

type MessageTemplateAPIController struct {
	*api.APIController
	ServiceMessageTemplate *wecom.WXMessageTemplateService
}

func NewMessageTemplateAPIController(context *gin.Context) (ctl *MessageTemplateAPIController) {

	return &MessageTemplateAPIController{
		APIController:          api.NewAPIController(context),
		ServiceMessageTemplate: wecom.NewWXMessageTemplateService(context),
	}
}
