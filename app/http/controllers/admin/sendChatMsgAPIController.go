package admin

import (
	"errors"
	"github.com/ArtisanCloud/PowerLibs/v2/object"
	"github.com/ArtisanCloud/PowerX/app/http/controllers/api"
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/ArtisanCloud/PowerX/app/models"
	modelWX "github.com/ArtisanCloud/PowerX/app/models/wx"
	"github.com/ArtisanCloud/PowerX/app/service"
	serviceWX "github.com/ArtisanCloud/PowerX/app/service/wx/wecom"
	"github.com/ArtisanCloud/PowerX/config"
	globalDatabase "github.com/ArtisanCloud/PowerX/database/global"
	logger "github.com/ArtisanCloud/PowerX/loggerManager"
	"github.com/gin-gonic/gin"
	"github.com/golang-module/carbon"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type SendChatMsgAPIController struct {
	*api.APIController
	ServiceSendChatMsg *service.SendChatMsgService
}

func NewSendChatMsgAPIController(context *gin.Context) (ctl *SendChatMsgAPIController) {

	return &SendChatMsgAPIController{
		APIController:      api.NewAPIController(context),
		ServiceSendChatMsg: service.NewSendChatMsgService(context),
	}
}

func APISendChatMsgSync(context *gin.Context) {
	ctl := NewSendChatMsgAPIController(context)

	defer api.RecoverResponse(context, "api.admin.sendChatMsg.sync")

	startDatetimeInterface, _ := context.Get("startDatetime")
	startDatetime := startDatetimeInterface.(*carbon.Carbon)
	endDatetimeInterface, _ := context.Get("endDatetime")
	endDatetime := endDatetimeInterface.(*carbon.Carbon)
	limitInterface, _ := context.Get("limit")
	limit := limitInterface.(int)

	rs, err := ctl.ServiceSendChatMsg.SyncSendChatMsgFromWXPlatform(globalDatabase.G_DBConnection, startDatetime, endDatetime, limit, "")
	if err != nil {
		ctl.RS.SetCode(config.API_ERR_CODE_FAIL_TO_SYNC_SEND_CHAT_MSG_ON_WX_PLATFORM, config.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}
	if rs.ErrCode != 0 {
		ctl.RS.SetCode(config.API_ERR_CODE_FAIL_TO_SYNC_SEND_CHAT_MSG_ON_WX_PLATFORM, config.API_RETURN_CODE_ERROR, "", errors.New(rs.ErrMSG).Error())
		panic(ctl.RS)
		return
	}

	ctl.RS.Success(context, err)
}

func APIGetSendChatMsgList(context *gin.Context) {
	ctl := NewSendChatMsgAPIController(context)

	params, _ := context.Get("params")
	para := params.(request.ParaList)
	creatorUserIDsInterface, _ := context.Get("creatorUserIDs")
	creatorUserIDs := creatorUserIDsInterface.([]string)
	filterStartDateInterface, _ := context.Get("filterStartDate")
	filterStartDate := filterStartDateInterface.(*carbon.Carbon)
	filterEndDateInterface, _ := context.Get("filterEndDate")
	filterEndDate := filterEndDateInterface.(*carbon.Carbon)

	defer api.RecoverResponse(context, "api.admin.sendChatMsg.list")

	arrayList, err := ctl.ServiceSendChatMsg.GetQueryList(globalDatabase.G_DBConnection, creatorUserIDs, filterStartDate, filterEndDate, para.Page, para.PageSize)
	if err != nil {
		ctl.RS.SetCode(config.API_ERR_CODE_FAIL_TO_GET_SEND_CHAT_MSG_LIST, config.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	ctl.RS.Success(context, arrayList)
}

func APIGetSendChatMsgDetail(context *gin.Context) {
	ctl := NewSendChatMsgAPIController(context)

	uuidInterface, _ := context.Get("uuid")
	uuid := uuidInterface.(string)

	defer api.RecoverResponse(context, "api.admin.sendChatMsg.detail")

	sendChatMsg, err := ctl.ServiceSendChatMsg.GetSendChatMsgByUUID(globalDatabase.G_DBConnection, uuid)
	if err != nil {
		ctl.RS.SetCode(config.API_ERR_CODE_FAIL_TO_GET_SEND_CHAT_MSG_DETAIL, config.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}
	if sendChatMsg == nil {
		ctl.RS.SetCode(config.API_ERR_CODE_FAIL_TO_GET_SEND_CHAT_MSG_DETAIL, config.API_RETURN_CODE_ERROR, "", "")
		panic(ctl.RS)
		return
	}

	ctl.RS.Success(context, sendChatMsg)
}

func APIEstimateSendChatCustomersCount(context *gin.Context) {
	ctl := NewSendChatMsgAPIController(context)

	params, _ := context.Get("sendChatMsg")
	sendChatMsg := params.(*models.SendChatMsg)

	defer api.RecoverResponse(context, "api.admin.sendChatMsg.estimateExternalUsers")

	var err error
	arrayAllExternalUserIDs := []string{}
	for _, messageTemplate := range sendChatMsg.WXMessageTemplates {
		arrayExternalUserIDs := []string{}
		err = object.JsonDecode(messageTemplate.ExternalUserIDs, &arrayExternalUserIDs)
		if err != nil {
			if err != nil {
				ctl.RS.SetCode(config.API_ERR_CODE_FAIL_TO_ESTIMATE_SEND_CHAT_MSG_CUSTOMERS_COUNT, config.API_RETURN_CODE_ERROR, "", err.Error())
				panic(ctl.RS)
				return
			}
		}
		arrayAllExternalUserIDs = append(arrayAllExternalUserIDs, arrayExternalUserIDs...)

	}

	ctl.RS.Success(context, len(arrayAllExternalUserIDs))
}

func APICreateSendChatMsg(context *gin.Context) {
	ctl := NewSendChatMsgAPIController(context)

	params, _ := context.Get("sendChatMsg")
	sendChatMsg := params.(*models.SendChatMsg)

	defer api.RecoverResponse(context, "api.admin.sendChatMsg.upsert")

	var err error

	err = globalDatabase.G_DBConnection.Transaction(func(tx *gorm.DB) error {
		// insert send chat msg
		err = ctl.ServiceSendChatMsg.UpsertSendChatMsgs(tx.Omit(clause.Associations), []*models.SendChatMsg{sendChatMsg}, nil)
		if err != nil {
			ctl.RS.SetCode(config.API_ERR_CODE_FAIL_TO_CREATE_SEND_CHAT_MSG, config.API_RETURN_CODE_ERROR, "", err.Error())
			return err
		}

		serviceWXMessageTemplate := serviceWX.NewWXMessageTemplateService(nil)
		for _, messageTemplate := range sendChatMsg.WXMessageTemplates {
			// upload wechat send chat msg
			if sendChatMsg.SendImmediately {
				result, err := ctl.ServiceSendChatMsg.DoSendChatMsg(messageTemplate)
				messageTemplate, err = ctl.ServiceSendChatMsg.ConvertResponseToMessageTemplate(messageTemplate, result)
				logger.Logger.Info("create message template unique id:", messageTemplate.UniqueID)
				if err != nil {
					ctl.RS.SetCode(config.API_ERR_CODE_FAIL_TO_DO_SEND_CHAT_MSG_ON_WX_PLATFORM, config.API_RETURN_CODE_ERROR, "", err.Error())
					return err
				}
			} else {
				// 我们现在使用的是crontab的定时器，按时触发DoSendChatMsg接口
				// crontab to: {location}/admin/api/sendChatMessage/doSend
				//
				// 如果二开团队有自己的触发延时事件机制，可以在这里的block实现
				// ...

			}

			// save the wechat message template
			err = serviceWXMessageTemplate.UpsertWXMessageTemplates(tx.Omit(clause.Associations), []*modelWX.WXMessageTemplate{messageTemplate}, nil)
			if err != nil {
				return err
			}

			// save send chat message send status to "sent"
			sendChatMsg.SendStatus = models.SEND_CHAT_MESSAGE_SEND_STATUS_SENT
			_, err = ctl.ServiceSendChatMsg.UpdateSendChatMsg(tx, sendChatMsg, false)
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

	ctl.RS.Success(context, sendChatMsg)

}

func APIDoSendChatMsgs(context *gin.Context) {
	ctl := NewSendChatMsgAPIController(context)

	defer api.RecoverResponse(context, "api.admin.sendChatMsg.doSend")

	var err error

	now := carbon.Now().SetSecond(0).
		SetMicrosecond(0).
		SetMillisecond(0)
	endDatetime := now.AddMinute()

	toSendList, err := ctl.ServiceSendChatMsg.GetToDoSendList(globalDatabase.G_DBConnection, &now, &endDatetime)
	if err != nil {
		ctl.RS.SetCode(config.API_ERR_CODE_FAIL_TO_GET_SEND_CHAT_MSG_LIST, config.API_RETURN_CODE_ERROR, "", err.Error())
		panic(ctl.RS)
		return
	}

	serviceWXMessageTemplate := serviceWX.NewWXMessageTemplateService(context)
	for _, sendChatMsg := range toSendList {

		if sendChatMsg.WXMessageTemplates == nil {
			continue
		}

		for _, wxMessageTemplate := range sendChatMsg.WXMessageTemplates {

			err = globalDatabase.G_DBConnection.Transaction(func(tx *gorm.DB) error {

				// send chat msg
				result, err := ctl.ServiceSendChatMsg.DoSendChatMsg(wxMessageTemplate)
				if err != nil {
					logger.Logger.Error(err.Error())
					return err
				}
				if result.ErrCode != 0 {
					logger.Logger.Error(result.ErrMSG)
					return err
				}
				wxMessageTemplate, err = ctl.ServiceSendChatMsg.ConvertResponseToMessageTemplate(wxMessageTemplate, result)
				if err != nil {
					return err
				}

				// save the wechat message template
				err = serviceWXMessageTemplate.UpsertWXMessageTemplates(tx.Omit(clause.Associations), []*modelWX.WXMessageTemplate{wxMessageTemplate}, nil)
				if err != nil {
					return err
				}

				// save send chat message send status to "sent"
				sendChatMsg.SendStatus = models.SEND_CHAT_MESSAGE_SEND_STATUS_SENT
				_, err = ctl.ServiceSendChatMsg.UpdateSendChatMsg(tx, sendChatMsg, false)
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
