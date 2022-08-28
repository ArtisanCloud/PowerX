package admin

import (
	"errors"
	"github.com/ArtisanCloud/PowerLibs/v2/object"
	"github.com/ArtisanCloud/PowerX/app/http/controllers/api"
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/ArtisanCloud/PowerX/app/models"
	modelWX "github.com/ArtisanCloud/PowerX/app/models/wx"
	"github.com/ArtisanCloud/PowerX/app/service"
	"github.com/ArtisanCloud/PowerX/app/service/wx/wecom"
	"github.com/ArtisanCloud/PowerX/config/global"
	globalDatabase "github.com/ArtisanCloud/PowerX/database/global"
	logger "github.com/ArtisanCloud/PowerX/loggerManager"
	"github.com/gin-gonic/gin"
	"github.com/golang-module/carbon"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type SendGroupChatMsgAPIController struct {
	*api.APIController
	ServiceSendGroupChatMsg *service.SendGroupChatMsgService
}

func NewSendGroupChatMsgAPIController(context *gin.Context) (ctl *SendGroupChatMsgAPIController) {

	return &SendGroupChatMsgAPIController{
		APIController:           api.NewAPIController(context),
		ServiceSendGroupChatMsg: service.NewSendGroupChatMsgService(context),
	}
}

func APISendGroupChatMsgSync(context *gin.Context) {
	ctl := NewSendGroupChatMsgAPIController(context)

	defer api.RecoverResponse(context, "api.admin.sendGroupChatMsg.sync")

	startDatetimeInterface, _ := context.Get("startDatetime")
	startDatetime := startDatetimeInterface.(*carbon.Carbon)
	endDatetimeInterface, _ := context.Get("endDatetime")
	endDatetime := endDatetimeInterface.(*carbon.Carbon)
	limitInterface, _ := context.Get("limit")
	limit := limitInterface.(int)

	rs, err := ctl.ServiceSendGroupChatMsg.SyncSendGroupChatMsgFromWXPlatform(globalDatabase.G_DBConnection, startDatetime, endDatetime, limit, "")
	if err != nil {
		ctl.RS.SetCode(global.API_ERR_CODE_FAIL_TO_SYNC_SEND_CHAT_MSG_ON_WX_PLATFORM, global.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}
	if rs.ErrCode != 0 {
		ctl.RS.SetCode(global.API_ERR_CODE_FAIL_TO_SYNC_SEND_CHAT_MSG_ON_WX_PLATFORM, global.API_RETURN_CODE_ERROR, "", errors.New(rs.ErrMSG).Error())
		panic(ctl.RS)
		return
	}

	ctl.RS.Success(context, err)
}

func APIGetSendGroupChatMsgList(context *gin.Context) {
	ctl := NewSendGroupChatMsgAPIController(context)

	params, _ := context.Get("params")
	para := params.(request.ParaList)
	groupChatMsgNameInterface, _ := context.Get("groupChatMsgName")
	groupChatMsgName := groupChatMsgNameInterface.(string)
	creatorUserIDsInterface, _ := context.Get("creatorUserIDs")
	creatorUserIDs := creatorUserIDsInterface.([]string)
	filterStartDateInterface, _ := context.Get("filterStartDate")
	filterStartDate := filterStartDateInterface.(*carbon.Carbon)
	filterEndDateInterface, _ := context.Get("filterEndDate")
	filterEndDate := filterEndDateInterface.(*carbon.Carbon)

	defer api.RecoverResponse(context, "api.admin.sendGroupChatMsg.list")

	arrayList, err := ctl.ServiceSendGroupChatMsg.GetQueryList(globalDatabase.G_DBConnection,
		groupChatMsgName, creatorUserIDs,
		filterStartDate, filterEndDate,
		para.Page, para.PageSize,
	)
	if err != nil {
		ctl.RS.SetCode(global.API_ERR_CODE_FAIL_TO_GET_SEND_GROUP_CHAT_MSG_LIST, global.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	ctl.RS.Success(context, arrayList)
}

func APIGetSendGroupChatMsgDetail(context *gin.Context) {
	ctl := NewSendGroupChatMsgAPIController(context)

	uuidInterface, _ := context.Get("uuid")
	uuid := uuidInterface.(string)

	defer api.RecoverResponse(context, "api.admin.sendGroupChatMsg.detail")

	sendGroupChatMsg, err := ctl.ServiceSendGroupChatMsg.GetSendGroupChatMsgByUUID(globalDatabase.G_DBConnection, uuid)
	if err != nil {
		ctl.RS.SetCode(global.API_ERR_CODE_FAIL_TO_GET_SEND_GROUP_CHAT_MSG_DETAIL, global.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}
	if sendGroupChatMsg == nil {
		ctl.RS.SetCode(global.API_ERR_CODE_FAIL_TO_GET_SEND_GROUP_CHAT_MSG_DETAIL, global.API_RETURN_CODE_ERROR, "", "")
		panic(ctl.RS)
		return
	}

	ctl.RS.Success(context, sendGroupChatMsg)
}

func APIEstimateSendGroupChatCustomersCount(context *gin.Context) {
	ctl := NewSendGroupChatMsgAPIController(context)

	params, _ := context.Get("sendGroupChatMsg")
	sendGroupChatMsg := params.(*models.SendGroupChatMsg)

	defer api.RecoverResponse(context, "api.admin.sendGroupChatMsg.estimateExternalUsers")

	var err error
	arrayAllExternalUserIDs := []string{}
	for _, messageTemplate := range sendGroupChatMsg.WXMessageTemplates {
		arrayExternalUserIDs := []string{}
		err = object.JsonDecode(messageTemplate.ExternalUserIDs, &arrayExternalUserIDs)
		if err != nil {
			if err != nil {
				ctl.RS.SetCode(global.API_ERR_CODE_FAIL_TO_ESTIMATE_SEND_GROUP_CHAT_MSG_CUSTOMERS_COUNT, global.API_RETURN_CODE_ERROR, "", err.Error())
				panic(ctl.RS)
				return
			}
		}
		arrayAllExternalUserIDs = append(arrayAllExternalUserIDs, arrayExternalUserIDs...)

	}

	ctl.RS.Success(context, len(arrayAllExternalUserIDs))
}

func APICreateSendGroupChatMsg(context *gin.Context) {
	ctl := NewSendGroupChatMsgAPIController(context)

	params, _ := context.Get("sendGroupChatMsg")
	sendGroupChatMsg := params.(*models.SendGroupChatMsg)

	defer api.RecoverResponse(context, "api.admin.sendGroupChatMsg.upsert")

	var err error

	err = globalDatabase.G_DBConnection.Transaction(func(tx *gorm.DB) error {
		// insert send chat msg
		err = ctl.ServiceSendGroupChatMsg.UpsertSendGroupChatMsgs(tx.Omit(clause.Associations), []*models.SendGroupChatMsg{sendGroupChatMsg}, nil)
		if err != nil {
			ctl.RS.SetCode(global.API_ERR_CODE_FAIL_TO_CREATE_SEND_GROUP_CHAT_MSG, global.API_RETURN_CODE_ERROR, "", err.Error())
			return err
		}

		serviceWXMessageTemplate := wecom.NewWXMessageTemplateService(nil)
		for _, messageTemplate := range sendGroupChatMsg.WXMessageTemplates {

			if sendGroupChatMsg.SendImmediately {
				result, err := ctl.ServiceSendGroupChatMsg.DoSendGroupChatMsg(messageTemplate)
				if err != nil {
					ctl.RS.SetCode(global.API_ERR_CODE_FAIL_TO_DO_SEND_GROUP_CHAT_MSG, global.API_RETURN_CODE_ERROR, "", err.Error())
					return err
				}
				messageTemplate, err = ctl.ServiceSendGroupChatMsg.ConvertResponseToMessageTemplate(messageTemplate, result)
				logger.Logger.Info("create message template unique id:" + messageTemplate.UniqueID)

			} else {

				// 我们现在使用的是crontab的定时器，按时触发DoSendGroupChatMsg接口
				// crontab to: {location}/admin/api/sendGroupChatMessage/doSend
				//
				// 如果二开团队有自己的触发延时事件机制，可以在这里的block实现
			}

			err = serviceWXMessageTemplate.UpsertWXMessageTemplates(tx.Omit(clause.Associations), []*modelWX.WXMessageTemplate{messageTemplate}, nil)
			if err != nil {
				return err
			}

			sendGroupChatMsg.SendStatus = models.SEND_GROUP_CHAT_MESSAGE_SEND_STATUS_SENT
			_, err = ctl.ServiceSendGroupChatMsg.UpdateSendGroupChatMsg(tx, sendGroupChatMsg, false)
			if err != nil {
				return err
			}
		}

		return err
	})

	if err != nil {
		panic(ctl.RS)
		return
	}

	ctl.RS.Success(context, sendGroupChatMsg)

}

func APIDoSendGroupChatMsgs(context *gin.Context) {
	ctl := NewSendGroupChatMsgAPIController(context)

	defer api.RecoverResponse(context, "api.admin.sendGroupChatMsg.doSend")

	var err error

	now := carbon.Now().SetSecond(0).
		SetMicrosecond(0).
		SetMillisecond(0)
	endDatetime := now.AddMinute()

	toSendList, err := ctl.ServiceSendGroupChatMsg.GetToDoSendList(globalDatabase.G_DBConnection, &now, &endDatetime)
	if err != nil {
		ctl.RS.SetCode(global.API_ERR_CODE_FAIL_TO_GET_SEND_GROUP_CHAT_MSG_LIST, global.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	serviceWXMessageTemplate := wecom.NewWXMessageTemplateService(context)
	for _, sendGroupChatMsg := range toSendList {

		if sendGroupChatMsg.WXMessageTemplates == nil {
			continue
		}

		for _, wxMessageTemplate := range sendGroupChatMsg.WXMessageTemplates {

			err = globalDatabase.G_DBConnection.Transaction(func(tx *gorm.DB) error {

				// send chat msg
				result, err := ctl.ServiceSendGroupChatMsg.DoSendGroupChatMsg(wxMessageTemplate)
				if err != nil {
					logger.Logger.Error(err.Error())
					return err
				}
				if result.ErrCode != 0 {
					logger.Logger.Error(result.ErrMSG)
					return err
				}
				wxMessageTemplate, err = ctl.ServiceSendGroupChatMsg.ConvertResponseToMessageTemplate(wxMessageTemplate, result)
				if err != nil {
					return err
				}

				// save the wx message template
				err = serviceWXMessageTemplate.UpsertWXMessageTemplates(tx.Omit(clause.Associations), []*modelWX.WXMessageTemplate{wxMessageTemplate}, nil)
				if err != nil {
					return err
				}

				// save send chat message send status to "sent"
				sendGroupChatMsg.SendStatus = models.SEND_CHAT_MESSAGE_SEND_STATUS_SENT
				_, err = ctl.ServiceSendGroupChatMsg.UpdateSendGroupChatMsg(tx, sendGroupChatMsg, false)
				if err != nil {
					return err
				}

				return err
			})

			if err != nil {
				logger.Logger.Error(err.Error())
				continue
			}

		}

	}

	return
}
