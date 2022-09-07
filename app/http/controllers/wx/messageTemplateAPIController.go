package wx

import (
	"github.com/ArtisanCloud/PowerX/app/http/controllers/api"
	"github.com/ArtisanCloud/PowerX/app/service/wx/weCom"
	"github.com/gin-gonic/gin"
)

type MessageTemplateAPIController struct {
	*api.APIController
	ServiceMessageTemplate *weCom.WXMessageTemplateService
}

func NewMessageTemplateAPIController(context *gin.Context) (ctl *MessageTemplateAPIController) {

	return &MessageTemplateAPIController{
		APIController:          api.NewAPIController(context),
		ServiceMessageTemplate: weCom.NewWXMessageTemplateService(context),
	}
}
