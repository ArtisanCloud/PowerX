package service

import (
	"errors"
	"fmt"
	databasePowerLib "github.com/ArtisanCloud/PowerLibs/v2/database"
	"github.com/ArtisanCloud/PowerLibs/v2/object"
	requestMessageTemplate "github.com/ArtisanCloud/PowerWeChat/v2/src/work/externalContact/messageTemplate/request"
	"github.com/ArtisanCloud/PowerWeChat/v2/src/work/externalContact/messageTemplate/response"
	"github.com/ArtisanCloud/PowerX/app/models"
	"github.com/ArtisanCloud/PowerX/app/models/wx"
	"github.com/ArtisanCloud/PowerX/app/service/wx/wecom"
	logger "github.com/ArtisanCloud/PowerX/loggerManager"
	"github.com/gin-gonic/gin"
	"github.com/golang-module/carbon"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type SendGroupChatMsgService struct {
	SendGroupChatMsg *models.SendGroupChatMsg
}

/**
 ** 初始化构造函数
 */
func NewSendGroupChatMsgService(ctx *gin.Context) (r *SendGroupChatMsgService) {
	r = &SendGroupChatMsgService{
		SendGroupChatMsg: models.NewSendGroupChatMsg(nil),
	}
	return r
}

func (srv *SendGroupChatMsgService) SyncSendGroupChatMsgFromWXPlatform(db *gorm.DB, startDatetime *carbon.Carbon, endDatetime *carbon.Carbon, limit int, cursor string) (rs *response.ResponseGetGroupMsgListV2, err error) {

	// get wechat single message templates from wechat platform
	req := &requestMessageTemplate.RequestGetGroupMsgListV2{
		ChatType:  "group",
		StartTime: startDatetime.Timestamp(),
		EndTime:   endDatetime.Timestamp(),
		Limit:     limit,
		Cursor:    cursor,
	}
	rs, err = wecom.G_WeComCustomer.App.ExternalContactMessageTemplate.GetGroupMsgListV2(req)
	if err != nil {
		return rs, err
	}
	if rs.ErrCode != 0 {
		return rs, errors.New(rs.ErrMSG)
	}

	// sync wechat message template from wechat platform
	err = db.Transaction(func(tx *gorm.DB) error {
		serviceWXMessageTemplate := wecom.NewWXMessageTemplateService(nil)
		for _, groupMsg := range rs.GroupMsgList {
			wxMessageTemplate, err := serviceWXMessageTemplate.GetWXMessageTemplateByMsgID(tx, groupMsg.MsgID)
			if err != nil {
				logger.Logger.Error(err.Error())
				continue
			}
			if wxMessageTemplate == nil {
				logger.Logger.Error(errors.New("wechat message template not found").Error())
				continue
			}
			err = serviceWXMessageTemplate.SyncWXMessageTemplateFromWXPlatform(tx, groupMsg, wxMessageTemplate.Sender)
			if err != nil {
				logger.Logger.Error(fmt.Sprintf("message template msg id: %s ", wxMessageTemplate.MsgID) + ", err: " + err.Error())
				return err
			}

		}

		return err
	})

	if req.Cursor != "" {
		rs, err = srv.SyncSendGroupChatMsgFromWXPlatform(db, startDatetime, endDatetime, limit, req.Cursor)
	}

	return rs, err
}

func (srv *SendGroupChatMsgService) DoSendGroupChatMsg(messageTemplate *wx.WXMessageTemplate) (result *response.ResponseAddMessageTemplate, err error) {
	// upload wechat send chat msg
	result, err = srv.CreateSendGroupChatMsgOnWXPlatform(messageTemplate)
	if err != nil {
		return result, err
	}
	if result.ErrCode != 0 {
		return result, errors.New(result.ErrMSG)
	}

	return result, err
}

func (srv *SendGroupChatMsgService) GetQueryList(db *gorm.DB,
	groupChatMsgName string, creatorUserIDs []string,
	filterStartDate *carbon.Carbon, filterEndDate *carbon.Carbon,
	page int, pageSize int,
) (paginator *databasePowerLib.Pagination, err error) {

	sendGroupChatMsgs := []*models.SendGroupChatMsg{}

	db = db.Model(&models.SendGroupChatMsg{}).
		//Debug().
		Preload("WXMessageTemplates.WXMessageTemplateTasks").
		Preload("WXMessageTemplates.WXMessageTemplateSends")

	if groupChatMsgName != "" {
		db = db.Where("group_chat_msg_name like ?", "%"+groupChatMsgName+"%")
	}

	// filter by sender user id
	if len(creatorUserIDs) > 0 {
		//sqlWhereInSender := databasePowerLib.FormatJsonBArrayToWhereInSQL("senders", creatorUserIDs)
		//db = db.Where(sqlWhereInSender)
		db = db.
			Joins("LEFT JOIN wx_message_templates AS wxMessageTemplates ON wxMessageTemplates.send_chat_msg_id = send_group_chat_msgs.uuid").
			Where("wxMessageTemplates.creator IN (?)", creatorUserIDs)
	}

	// filter by start date and end date
	//fmt2.Dump(filterStartDate, filterEndDate)
	if !filterStartDate.IsZero() && !filterEndDate.IsZero() {
		db = db.Where("send_on_time>=? AND send_on_time<=?", filterStartDate, filterEndDate)
	}

	pagination, err := databasePowerLib.GetList(db, nil, &sendGroupChatMsgs, nil, page, pageSize)

	return pagination, err
}

func (srv *SendGroupChatMsgService) GetToDoSendList(db *gorm.DB, filterStartDate *carbon.Carbon, filterEndDate *carbon.Carbon) (sendGroupChatMsgs []*models.SendGroupChatMsg, err error) {

	sendGroupChatMsgs = []*models.SendGroupChatMsg{}

	db = db.Model(&models.SendGroupChatMsg{}).
		//Debug().
		Preload("WXMessageTemplates").
		Where("send_status", models.SEND_CHAT_MESSAGE_SEND_STATUS_UNSENT)

	// filter by start date and end date
	//fmt2.Dump(filterStartDate, filterEndDate)
	if !filterStartDate.IsZero() && !filterEndDate.IsZero() {
		db = db.Where("send_on_time>=? AND send_on_time<=?", filterStartDate, filterEndDate)
	} else {
		return nil, errors.New("filter datetime is required")
	}

	result := db.Find(&sendGroupChatMsgs)

	return sendGroupChatMsgs, result.Error
}

func (srv *SendGroupChatMsgService) UpsertSendGroupChatMsgs(db *gorm.DB, sendGroupChatMsgs []*models.SendGroupChatMsg, fieldsToUpdate []string) error {

	return databasePowerLib.UpsertModelsOnUniqueID(db, &models.SendGroupChatMsg{}, models.SEND_GROUP_CHAT_MSG_UNIQUE_ID, sendGroupChatMsgs, fieldsToUpdate)
}

func (srv *SendGroupChatMsgService) SaveSendGroupChatMsg(db *gorm.DB, sendGroupChatMsg *models.SendGroupChatMsg) (*models.SendGroupChatMsg, error) {

	db = db.Create(sendGroupChatMsg)

	return sendGroupChatMsg, db.Error
}

func (srv *SendGroupChatMsgService) UpdateSendGroupChatMsg(db *gorm.DB, sendGroupChatMsg *models.SendGroupChatMsg, withAssociation bool) (*models.SendGroupChatMsg, error) {
	db = db
	if !withAssociation {
		db = db.Omit(clause.Associations)
	}
	db = db.
		//Debug().
		Updates(sendGroupChatMsg)

	return sendGroupChatMsg, db.Error
}

func (srv *SendGroupChatMsgService) DeleteSendGroupChatMsgsByUUIDs(db *gorm.DB, uuids []string) error {

	db = db.
		//Debug().
		Where("uuid in (?)", uuids).
		Delete(models.SendGroupChatMsg{})

	return db.Error
}

func (srv *SendGroupChatMsgService) DeleteSendGroupChatMsgByUUID(db *gorm.DB, uuid string) error {

	db = db.
		//Debug().
		Where("uuid", uuid).
		Delete(&models.SendGroupChatMsg{})

	return db.Error
}

func (srv *SendGroupChatMsgService) GetSendGroupChatMsgsByUUIDs(db *gorm.DB, uuids []string) (sendGroupChatMsgs []*models.SendGroupChatMsg, err error) {

	sendGroupChatMsgs = []*models.SendGroupChatMsg{}

	db = db.Where("uuid in (?)", uuids)
	result := db.Find(&sendGroupChatMsgs)
	return sendGroupChatMsgs, result.Error
}

func (srv *SendGroupChatMsgService) GetSendGroupChatMsgByUUID(db *gorm.DB, uuid string) (sendGroupChatMsg *models.SendGroupChatMsg, err error) {

	sendGroupChatMsg = &models.SendGroupChatMsg{}

	preloads := []string{
		"WXMessageTemplates.WXMessageTemplateTasks",
		"WXMessageTemplates.WXMessageTemplateSends",
	}

	condition := &map[string]interface{}{
		"uuid": uuid,
	}
	err = databasePowerLib.GetFirst(db, condition, sendGroupChatMsg, preloads)
	if err != nil && errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return sendGroupChatMsg, err
}

func (srv *SendGroupChatMsgService) CreateSendGroupChatMsgOnWXPlatform(msg *wx.WXMessageTemplate) (result *response.ResponseAddMessageTemplate, err error) {

	request, err := wecom.G_WeComEmployee.ConvertAttachmentsToMessageTemplate(msg)
	if err != nil {
		return nil, err
	}
	result, err = wecom.G_WeComCustomer.App.ExternalContactMessageTemplate.AddMsgTemplate(request)
	if err != nil {
		return nil, err
	}
	if result.ErrCode != 0 {
		return result, errors.New(result.ErrMSG)
	}

	return result, nil
}

func (srv *SendGroupChatMsgService) ConvertResponseToMessageTemplate(msg *wx.WXMessageTemplate, responseSendGroupChatMsg *response.ResponseAddMessageTemplate) (*wx.WXMessageTemplate, error) {

	failList, err := object.JsonEncode(responseSendGroupChatMsg.FailList)
	if err != nil {
		return nil, err
	}
	msg.MsgID = responseSendGroupChatMsg.MsgID
	msg.FailList = datatypes.JSON(failList)

	return msg, nil
}

// ---------------------------------------------------------------------------------------------------------------------
