package sendChatMsg

import (
	"errors"
	"github.com/ArtisanCloud/PowerX/app/http"
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/ArtisanCloud/PowerX/app/models"
	"github.com/ArtisanCloud/PowerX/app/service"
	"github.com/ArtisanCloud/PowerX/configs/global"
	globalDatabase "github.com/ArtisanCloud/PowerX/database/global"
	"github.com/gin-gonic/gin"
)

type ParaDeleteSendChatMsg struct {
	UUID string `form:"uuid" json:"uuid" xml:"uuid" binding:"required"`
}

func ValidateDeleteSendChatMsg(context *gin.Context) {
	var form ParaDeleteSendChatMsg

	err := request.ValidatePara(context, &form)
	if err != nil {
		return
	}

	apiResponse := http.NewAPIResponse(context)

	sendChatMsg, err := convertParaDeleteSendChatMsgForDelete(&form)
	if err != nil {
		apiResponse.SetCode(global.API_ERR_CODE_REQUEST_PARAM_ERROR, global.API_RETURN_CODE_ERROR, "", err.Error()).ThrowJSONResponse(context)
	}

	context.Set("sendChatMsg", sendChatMsg)
	context.Next()
}

func convertParaDeleteSendChatMsgForDelete(form *ParaDeleteSendChatMsg) (sendChatMsg *models.SendChatMsg, err error) {

	serviceSendChatMsg := service.NewSendChatMsgService(nil)
	sendChatMsg, err = serviceSendChatMsg.GetSendChatMsgByUUID(globalDatabase.G_DBConnection, form.UUID)

	if err != nil {
		return sendChatMsg, err
	}

	if sendChatMsg == nil {
		return sendChatMsg, errors.New("sendChatMsg is nil")
	}

	return sendChatMsg, nil

}
