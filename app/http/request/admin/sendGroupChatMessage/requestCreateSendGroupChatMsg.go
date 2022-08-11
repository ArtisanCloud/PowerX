package sendGroupChatMsg

import (
	"github.com/ArtisanCloud/PowerLibs/v2/database"
	"github.com/ArtisanCloud/PowerLibs/v2/object"
	requestMessageTemplate "github.com/ArtisanCloud/PowerWeChat/v2/src/work/externalContact/messageTemplate/request"
	"github.com/ArtisanCloud/PowerX/app/http"
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/ArtisanCloud/PowerX/app/models"
	"github.com/ArtisanCloud/PowerX/app/models/wx"
	"github.com/ArtisanCloud/PowerX/app/service"
	"github.com/ArtisanCloud/PowerX/config/global"
	"github.com/gin-gonic/gin"
	"github.com/golang-module/carbon"
	"gorm.io/datatypes"
)

type ParaCreateSendGroupChat struct {
	GroupChatMsgName string                               `form:"groupChatMsgName" json:"groupChatMsgName" binding:"required"`
	Senders          []string                             `form:"senders" json:"senders" binding:"required,min=1"`
	SendImmediately  *bool                                `form:"sendImmediately" json:"sendImmediately" binding:"required"`
	SendOnTime       string                               `form:"sendOnTime" json:"sendOnTime"`
	Text             requestMessageTemplate.TextOfMessage `form:"text" json:"text"`
	Attachments      []*object.HashMap                    `form:"attachments" json:"attachments" binding:"required"`
}

func ValidateCreateSendGroupChatMsg(context *gin.Context) {
	var form ParaCreateSendGroupChat
	apiResponse := http.NewAPIResponse(context)

	err := request.ValidatePara(context, &form)
	if err != nil {
		return
	}

	sendGroupChatMsg, err := convertParaToSendGroupChatForCreate(context, form)
	if err != nil {
		apiResponse.SetCode(global.API_ERR_CODE_REQUEST_PARAM_ERROR, global.API_RETURN_CODE_ERROR, "", err.Error()).
			ThrowJSONResponse(context)
		return
	}

	context.Set("sendGroupChatMsg", sendGroupChatMsg)
	context.Next()
}

func convertParaToSendGroupChatForCreate(context *gin.Context, form ParaCreateSendGroupChat) (sendGroupChatMsg *models.SendGroupChatMsg, err error) {

	// Get senders
	senders, err := object.JsonEncode(form.Senders)
	if err != nil {
		return nil, err
	}

	sendOnTime := carbon.Now().Time
	if !*form.SendImmediately {
		sendOnTime = carbon.Parse(form.SendOnTime).Carbon2Time()
	}

	sendGroupChatMsg = &models.SendGroupChatMsg{
		PowerModel:       database.NewPowerModel(),
		GroupChatMsgName: form.GroupChatMsgName,
		Senders:          datatypes.JSON([]byte(senders)),
		SendImmediately:  *form.SendImmediately,
		SendOnTime:       sendOnTime,
	}

	attachments, _ := object.JsonEncode(form.Attachments)
	text, _ := object.JsonEncode(form.Text)
	for _, sender := range form.Senders {

		messageTemplate := &wx.WXMessageTemplate{
			PowerCompactModel: database.NewPowerCompactModel(),
			SendChatMsgUUID:   sendGroupChatMsg.GetForeignReferValue(),
			ChatType:          "group",
			Sender:            sender,
			Text:              datatypes.JSON([]byte(text)),
			Attachments:       datatypes.JSON([]byte(attachments)),
			Creator:           service.GetAuthEmployee(context).WXUserID.String,
		}
		messageTemplate.UniqueID = messageTemplate.GetComposedUniqueID()
		sendGroupChatMsg.WXMessageTemplates = append(sendGroupChatMsg.WXMessageTemplates, messageTemplate)
	}

	return sendGroupChatMsg, nil
}
