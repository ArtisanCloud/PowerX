package service

import (
	"errors"
	"fmt"
	databasePowerLib "github.com/ArtisanCloud/PowerLibs/v2/database"
	"github.com/ArtisanCloud/PowerLibs/v2/object"
	requestMessageTemplate "github.com/ArtisanCloud/PowerWeChat/v2/src/work/externalContact/messageTemplate/request"
	"github.com/ArtisanCloud/PowerWeChat/v2/src/work/externalContact/messageTemplate/response"
	"github.com/ArtisanCloud/PowerX/app/models"
	modelWX "github.com/ArtisanCloud/PowerX/app/models/wx"
	"github.com/ArtisanCloud/PowerX/app/service/wx/weCom"
	logger "github.com/ArtisanCloud/PowerX/loggerManager"
	"github.com/gin-gonic/gin"
	"github.com/golang-module/carbon"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type SendChatMsgService struct {
	SendChatMsg *models.SendChatMsg
}

/**
 ** 初始化构造函数
 */
func NewSendChatMsgService(ctx *gin.Context) (r *SendChatMsgService) {
	r = &SendChatMsgService{
		SendChatMsg: models.NewSendChatMsg(nil),
	}
	return r
}

func (srv *SendChatMsgService) SyncSendChatMsgFromWXPlatform(db *gorm.DB, startDatetime *carbon.Carbon, endDatetime *carbon.Carbon, limit int, cursor string) (rs *response.ResponseGetGroupMsgListV2, err error) {

	// get wechat single message templates from wechat platform
	req := &requestMessageTemplate.RequestGetGroupMsgListV2{
		ChatType:  "single",
		StartTime: startDatetime.Timestamp(),
		EndTime:   endDatetime.Timestamp(),
		Limit:     limit,
		Cursor:    cursor,
	}
	rs, err = weCom.G_WeComCustomer.App.ExternalContactMessageTemplate.GetGroupMsgListV2(req)
	if err != nil {
		return rs, err
	}
	if rs.ErrCode != 0 {
		return rs, errors.New(rs.ErrMSG)
	}

	// sync wechat message template from wechat platform
	err = db.Transaction(func(tx *gorm.DB) error {
		serviceWXMessageTemplate := weCom.NewWXMessageTemplateService(nil)
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
		rs, err = srv.SyncSendChatMsgFromWXPlatform(db, startDatetime, endDatetime, limit, req.Cursor)
	}

	return rs, err
}

func (srv *SendChatMsgService) SyncWXMessageTemplateFromWXPlatform(db *gorm.DB, groupMsg *response.GroupMsg, sender string) (err error) {
	serviceWXMessageTemplate := weCom.NewWXMessageTemplateService(nil)

	wxMessageTemplate := &modelWX.WXMessageTemplate{
		MsgID:      groupMsg.MsgID,
		Creator:    groupMsg.Creator,
		CreateTime: groupMsg.CreateTime,
		CreateType: groupMsg.CreateType,
	}
	updateFields := []string{
		"create_time",
		"create_type",
	}
	if groupMsg.Creator != "" {
		updateFields = append(updateFields, "creator")
	}

	err = serviceWXMessageTemplate.UpsertWXMessageTemplates(db, []*modelWX.WXMessageTemplate{wxMessageTemplate}, updateFields)
	if err != nil {
		return err
	}

	err = srv.SyncWXMessageTemplateTasksFromWXPlatform(db, groupMsg.MsgID, 100, "")
	if err != nil {
		return err
	}

	err = srv.SyncWXMessageTemplateSendResultsFromWXPlatform(db, groupMsg.MsgID, sender, 100, "")
	if err != nil {
		return err
	}

	return err
}

func (srv *SendChatMsgService) SyncWXMessageTemplateTasksFromWXPlatform(db *gorm.DB, msgID string, limit int, cursor string) (err error) {

	responseGroupMsgTask, err := weCom.G_WeComCustomer.App.ExternalContactMessageTemplate.GetGroupMsgTask(msgID, limit, cursor)
	if err != nil {
		return err
	}
	if responseGroupMsgTask.ErrCode != 0 {
		return errors.New(responseGroupMsgTask.ErrMSG)
	}

	if responseGroupMsgTask.NextCursor != "" {
		err = srv.SyncWXMessageTemplateTasksFromWXPlatform(db, msgID, limit, responseGroupMsgTask.NextCursor)
	}

	serviceMessageTemplate := weCom.NewWXMessageTemplateService(nil)
	// upsert wechat message templates tasks
	for _, rs := range responseGroupMsgTask.TaskList {
		task := modelWX.NewWXMessageTemplateTask(msgID, rs)
		err = serviceMessageTemplate.UpsertWXMessageTemplateTasks(db, []*modelWX.WXMessageTemplateTask{task}, nil)
	}

	return err
}

func (srv *SendChatMsgService) SyncWXMessageTemplateSendResultsFromWXPlatform(db *gorm.DB, msgID string, userID string, limit int, cursor string) (err error) {

	responseGroupMsgSendResult, err := weCom.G_WeComCustomer.App.ExternalContactMessageTemplate.GetGroupMsgSendResult(msgID, userID, limit, cursor)
	if err != nil {
		return err
	}
	if responseGroupMsgSendResult.ErrCode != 0 {
		return errors.New(responseGroupMsgSendResult.ErrMSG)
	}

	if responseGroupMsgSendResult.NextCursor != "" {
		err = srv.SyncWXMessageTemplateSendResultsFromWXPlatform(db, msgID, userID, limit, responseGroupMsgSendResult.NextCursor)
	}

	// upsert wechat message templates send results
	serviceMessageTemplate := weCom.NewWXMessageTemplateService(nil)
	for _, rs := range responseGroupMsgSendResult.SendList {
		sendResult := modelWX.NewWXMessageTemplateSendResult(msgID, rs)
		err = serviceMessageTemplate.UpsertWXMessageTemplateSendResults(db, []*modelWX.WXMessageTemplateSend{sendResult}, nil)
	}
	return err
}

func (srv *SendChatMsgService) DoSendChatMsg(messageTemplate *modelWX.WXMessageTemplate) (result *response.ResponseAddMessageTemplate, err error) {
	// upload wechat send chat msg
	result, err = srv.CreateSendChatMsgOnWXPlatform(messageTemplate)
	if err != nil {
		return result, err
	}
	if result.ErrCode != 0 {
		return result, errors.New(result.ErrMSG)
	}

	return result, err
}

func (srv *SendChatMsgService) GetQueryList(db *gorm.DB,
	creatorUserIDs []string,
	filterStartDate *carbon.Carbon, filterEndDate *carbon.Carbon,
	page int, pageSize int,
) (paginator *databasePowerLib.Pagination, err error) {

	sendChatMsgs := []*models.SendChatMsg{}

	db = db.Model(&models.SendChatMsg{}).
		//Debug().
		Preload("WXMessageTemplates.WXMessageTemplateTasks").
		Preload("WXMessageTemplates.WXMessageTemplateSends")

	// filter by sender user id
	if len(creatorUserIDs) > 0 {
		//sqlWhereInSender := databasePowerLib.FormatJsonBArrayToWhereInSQL("senders", creatorUserIDs)
		//db = db.Where(sqlWhereInSender)
		db = db.
			Joins("LEFT JOIN wx_message_templates AS wxMessageTemplates ON wxMessageTemplates.send_chat_msg_id = send_chat_msgs.uuid").
			Where("wxMessageTemplates.creator IN (?)", creatorUserIDs)
	}

	// filter by start date and end date
	//fmt2.Dump(filterStartDate, filterEndDate)
	if !filterStartDate.IsZero() && !filterEndDate.IsZero() {
		db = db.Where("send_on_time>=? AND send_on_time<=?", filterStartDate, filterEndDate)
	}

	pagination, err := databasePowerLib.GetList(db, nil, &sendChatMsgs, nil, page, pageSize)

	return pagination, err
}

func (srv *SendChatMsgService) GetToDoSendList(db *gorm.DB, filterStartDate *carbon.Carbon, filterEndDate *carbon.Carbon) (sendChatMsgs []*models.SendChatMsg, err error) {

	sendChatMsgs = []*models.SendChatMsg{}

	db = db.Model(&models.SendChatMsg{}).
		Debug().
		Preload("WXMessageTemplates").
		Where("send_status", models.SEND_CHAT_MESSAGE_SEND_STATUS_UNSENT)

	// filter by start date and end date
	//fmt2.Dump(filterStartDate, filterEndDate)
	if !filterStartDate.IsZero() && !filterEndDate.IsZero() {
		db = db.Where("send_on_time>=? AND send_on_time<=?", filterStartDate, filterEndDate)
	} else {
		return nil, errors.New("filter datetime is required")
	}

	result := db.Find(&sendChatMsgs)

	return sendChatMsgs, result.Error
}

func (srv *SendChatMsgService) UpsertSendChatMsgs(db *gorm.DB, sendChatMsgs []*models.SendChatMsg, fieldsToUpdate []string) error {

	return databasePowerLib.UpsertModelsOnUniqueID(db, &models.SendChatMsg{}, models.SEND_CHAT_MSG_UNIQUE_ID, sendChatMsgs, fieldsToUpdate)
}

func (srv *SendChatMsgService) SaveSendChatMsg(db *gorm.DB, sendChatMsg *models.SendChatMsg) (*models.SendChatMsg, error) {

	db = db.Create(sendChatMsg)

	return sendChatMsg, db.Error
}

func (srv *SendChatMsgService) UpdateSendChatMsg(db *gorm.DB, sendChatMsg *models.SendChatMsg, withAssociation bool) (*models.SendChatMsg, error) {
	db = db
	if !withAssociation {
		db = db.Omit(clause.Associations)
	}
	db = db.
		//Debug().
		Updates(sendChatMsg)

	return sendChatMsg, db.Error
}

func (srv *SendChatMsgService) DeleteSendChatMsgsByUUIDs(db *gorm.DB, uuids []string) error {

	db = db.
		//Debug().
		Where("uuid in (?)", uuids).
		Delete(models.SendChatMsg{})

	return db.Error
}

func (srv *SendChatMsgService) DeleteSendChatMsgByUUID(db *gorm.DB, uuid string) error {

	db = db.
		//Debug().
		Where("uuid", uuid).
		Delete(&models.SendChatMsg{})

	return db.Error
}

func (srv *SendChatMsgService) GetSendChatMsgsByUUIDs(db *gorm.DB, uuids []string) (sendChatMsgs []*models.SendChatMsg, err error) {

	sendChatMsgs = []*models.SendChatMsg{}

	db = db.Where("uuid in (?)", uuids)
	result := db.Find(&sendChatMsgs)
	return sendChatMsgs, result.Error
}

func (srv *SendChatMsgService) GetSendChatMsgByUUID(db *gorm.DB, uuid string) (sendChatMsg *models.SendChatMsg, err error) {

	sendChatMsg = &models.SendChatMsg{}

	preloads := []string{
		"WXMessageTemplates.WXMessageTemplateTasks",
		"WXMessageTemplates.WXMessageTemplateSends",
	}

	condition := &map[string]interface{}{
		"uuid": uuid,
	}
	err = databasePowerLib.GetFirst(db, condition, sendChatMsg, preloads)
	if err != nil && errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return sendChatMsg, err
}

func (srv *SendChatMsgService) CreateSendChatMsgOnWXPlatform(msg *modelWX.WXMessageTemplate) (result *response.ResponseAddMessageTemplate, err error) {

	request, err := weCom.G_WeComEmployee.ConvertAttachmentsToMessageTemplate(msg)
	if err != nil {
		return nil, err
	}

	result, err = weCom.G_WeComCustomer.App.ExternalContactMessageTemplate.AddMsgTemplate(request)
	if err != nil {
		return nil, err
	}
	if result.ErrCode != 0 {
		return result, errors.New(result.ErrMSG)
	}

	return result, nil
}

func (srv *SendChatMsgService) ConvertResponseToMessageTemplate(msg *modelWX.WXMessageTemplate, responseSendChatMsg *response.ResponseAddMessageTemplate) (*modelWX.WXMessageTemplate, error) {

	failList, err := object.JsonEncode(responseSendChatMsg.FailList)
	if err != nil {
		return nil, err
	}
	msg.MsgID = responseSendChatMsg.MsgID
	msg.FailList = datatypes.JSON(failList)

	return msg, nil
}

// ---------------------------------------------------------------------------------------------------------------------
