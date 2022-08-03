package sendChatMsg

import (
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/ArtisanCloud/PowerX/app/models"
	"github.com/ArtisanCloud/PowerX/app/models/wx"
	"github.com/ArtisanCloud/PowerX/app/service"
	"github.com/ArtisanCloud/PowerX/database"
	"github.com/gin-gonic/gin"
)

type ParaUpdateSendChatMsg struct {
	UUID string `form:"uuid" json:"uuid" xml:"uuid"`

	Name                            string `form:"name" json:"name" binding:"required"`
	GroupUUID                       string `form:"groupUUID" json:"groupUUID" binding:"required"`
	AllowEmployeeChangeOnlineStatus bool   `form:"allowEmployeeChangeOnlineStatus" json:"allowEmployeeChangeOnlineStatus" binding:"required"`
	RemarkAccount                   string `form:"remarkAccount" json:"remarkAccount"`
	RemarkAccountPrefix             bool   `form:"remarkAccountPrefix" json:"remarkAccountPrefix"`
	WelcomeMessageType              int8   `form:"welcomeMessageType" json:"welcomeMessageType"`
	Type                            int    `form:"type" json:"type"`
	Scene                           int    `form:"scene" json:"scene"`
	Style                           int    `form:"style" json:"style"`
	Remark                          string `form:"remark" json:"remark"`
	SkipVerify                      bool   `form:"skipVerify" json:"skipVerify"`
	//User                            []string              `form:"user" json:"user"`
	//Party                           []int                 `form:"party" json:"party"`
	//Conclusions                     *request2.Conclusions `form:"conclusions" json:"conclusions"`
	//WXTagIDs                        []string              `form:"wxTagIDs" json:"wxTagIDs"`
	//IsTemp        bool                  `form:"isTemp" json:"isTemp"`
	//ExpiresIn     int                   `form:"expiresIn" json:"expiresIn"`
	//ChatExpiresIn int                   `form:"chatExpiresIn" json:"chatExpiresIn"`
	//UnionID       string                `form:"unionID" json:"unionID"`
}

func ValidateUpdateSendChatMsg(context *gin.Context) {
	var form ParaUpdateSendChatMsg

	err := request.ValidatePara(context, &form)
	if err != nil {
		return
	}

	sendChatMsg, updateTags, err := convertParaToSendChatMsgForUpdate(form)
	if err != nil {
		return
	}

	context.Set("sendChatMsg", sendChatMsg)
	context.Set("updateTags", updateTags)
	context.Next()
}

func convertParaToSendChatMsgForUpdate(form ParaUpdateSendChatMsg) (sendChatMsg *models.SendChatMsg, updateTags []*wx.WXTag, err error) {

	sendChatMsgService := service.NewSendChatMsgService(nil)
	sendChatMsg, err = sendChatMsgService.GetSendChatMsgByUUID(database.DBConnection, form.UUID)
	if err != nil {
		return nil, nil, err
	}

	//users, err := object.JsonEncode(form.User)
	//if err != nil {
	//	return nil, nil, err
	//}
	//parties, err := object.JsonEncode(form.Party)
	//if err != nil {
	//	return nil, nil, err
	//}
	//conclusions, err := object.JsonEncode(form.Conclusions)
	//if err != nil {
	//	return nil, nil, err
	//}
	//
	//sendChatMsg.Name = form.Name
	//sendChatMsg.GroupUUID = form.GroupUUID
	//sendChatMsg.AllowEmployeeChangeOnlineStatus = form.AllowEmployeeChangeOnlineStatus
	//sendChatMsg.RemarkAccount = form.RemarkAccount
	//sendChatMsg.RemarkAccountPrefix = form.RemarkAccountPrefix
	//sendChatMsg.WelcomeMessageType = form.WelcomeMessageType
	//
	//sendChatMsg.WXSendChatMsg.Type = &form.Type
	//sendChatMsg.WXSendChatMsg.Scene = &form.Scene
	//sendChatMsg.WXSendChatMsg.Style = &form.Style
	//sendChatMsg.WXSendChatMsg.Remark = &form.Remark
	//sendChatMsg.WXSendChatMsg.SkipVerify = &form.SkipVerify
	//sendChatMsg.WXSendChatMsg.User = datatypes.JSON([]byte(users))
	//sendChatMsg.WXSendChatMsg.Party = datatypes.JSON([]byte(parties))
	//sendChatMsg.WXSendChatMsg.Conclusions = datatypes.JSON([]byte(conclusions))

	return sendChatMsg, updateTags, nil
}
