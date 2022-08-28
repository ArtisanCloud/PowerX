package wecom

import (
	"errors"
	databasePowerLib "github.com/ArtisanCloud/PowerLibs/v2/database"
	"github.com/ArtisanCloud/PowerLibs/v2/object"
	"github.com/ArtisanCloud/PowerWeChat/v2/src/work/externalContact/messageTemplate/response"
	"github.com/ArtisanCloud/PowerX/app/models/wx"
	"github.com/ArtisanCloud/PowerX/config/app"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type WXMessageTemplateService struct {
	wxMessageTemplate *wx.WXMessageTemplate
}

func NewWXMessageTemplateService(ctx *gin.Context) (r *WXMessageTemplateService) {
	weComConfig, _ := object.StructToMap(app.G_AppConfigure.Wechat["wecom"])
	if weComConfig["contact_secret"] != nil {
		weComConfig["secret"] = weComConfig["contact_secret"]
	}
	r = &WXMessageTemplateService{
		wxMessageTemplate: wx.NewWXMessageTemplate(nil),
	}
	return r
}

func (srv *WXMessageTemplateService) SyncWXMessageTemplateFromWXPlatform(db *gorm.DB, groupMsg *response.GroupMsg, sender string) (err error) {

	wxMessageTemplate := &wx.WXMessageTemplate{
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

	err = srv.UpsertWXMessageTemplates(db, []*wx.WXMessageTemplate{wxMessageTemplate}, updateFields)
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

func (srv *WXMessageTemplateService) SyncWXMessageTemplateTasksFromWXPlatform(db *gorm.DB, msgID string, limit int, cursor string) (err error) {

	responseGroupMsgTask, err := G_WeComCustomer.App.ExternalContactMessageTemplate.GetGroupMsgTask(msgID, limit, cursor)
	if err != nil {
		return err
	}
	if responseGroupMsgTask.ErrCode != 0 {
		return errors.New(responseGroupMsgTask.ErrMSG)
	}

	if responseGroupMsgTask.NextCursor != "" {
		err = srv.SyncWXMessageTemplateTasksFromWXPlatform(db, msgID, limit, responseGroupMsgTask.NextCursor)
	}

	serviceMessageTemplate := NewWXMessageTemplateService(nil)
	// upsert wx message templates tasks
	for _, rs := range responseGroupMsgTask.TaskList {
		task := wx.NewWXMessageTemplateTask(msgID, rs)
		err = serviceMessageTemplate.UpsertWXMessageTemplateTasks(db, []*wx.WXMessageTemplateTask{task}, nil)
	}

	return err
}

func (srv *WXMessageTemplateService) SyncWXMessageTemplateSendResultsFromWXPlatform(db *gorm.DB, msgID string, userID string, limit int, cursor string) (err error) {

	responseGroupMsgSendResult, err := G_WeComCustomer.App.ExternalContactMessageTemplate.GetGroupMsgSendResult(msgID, userID, limit, cursor)
	if err != nil {
		return err
	}
	if responseGroupMsgSendResult.ErrCode != 0 {
		return errors.New(responseGroupMsgSendResult.ErrMSG)
	}

	if responseGroupMsgSendResult.NextCursor != "" {
		err = srv.SyncWXMessageTemplateSendResultsFromWXPlatform(db, msgID, userID, limit, responseGroupMsgSendResult.NextCursor)
	}

	// upsert wx message templates send results
	serviceMessageTemplate := NewWXMessageTemplateService(nil)
	for _, rs := range responseGroupMsgSendResult.SendList {
		sendResult := wx.NewWXMessageTemplateSendResult(msgID, rs)
		err = serviceMessageTemplate.UpsertWXMessageTemplateSendResults(db, []*wx.WXMessageTemplateSend{sendResult}, nil)
	}
	return err
}

func (srv *WXMessageTemplateService) GetWXMessageTemplateByMsgID(db *gorm.DB, msgID string) (messageTemplate *wx.WXMessageTemplate, err error) {

	messageTemplate = &wx.WXMessageTemplate{}

	condition := &map[string]interface{}{
		"msg_id": msgID,
	}
	err = databasePowerLib.GetFirst(db, condition, messageTemplate, nil)
	if err != nil && errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return messageTemplate, err
}

func (srv *WXMessageTemplateService) UpsertWXMessageTemplates(db *gorm.DB, messageTemplates []*wx.WXMessageTemplate, fieldsToUpdate []string) error {

	return databasePowerLib.UpsertModelsOnUniqueID(db, &wx.WXMessageTemplate{}, wx.WX_MESSAGE_TEMPLATE_UNIQUE_ID, messageTemplates, fieldsToUpdate)
}

func (srv *WXMessageTemplateService) UpsertWXMessageTemplateTasks(db *gorm.DB, tasks []*wx.WXMessageTemplateTask, fieldsToUpdate []string) error {
	return databasePowerLib.UpsertModelsOnUniqueID(db, wx.WXMessageTemplateTask{}, wx.WX_MESSAGE_TEMPLATE_TASK_UNIQUE_ID, tasks, fieldsToUpdate)
}

func (srv *WXMessageTemplateService) UpsertWXMessageTemplateSendResults(db *gorm.DB, tasks []*wx.WXMessageTemplateSend, fieldsToUpdate []string) error {

	return databasePowerLib.UpsertModelsOnUniqueID(db, &wx.WXMessageTemplateSend{}, wx.WX_MESSAGE_TEMPLATE_SEND_RESULT_UNIQUE_ID, tasks, fieldsToUpdate)
}
