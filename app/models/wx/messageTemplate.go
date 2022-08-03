package wx

import (
	"github.com/ArtisanCloud/PowerLibs/v2/database"
	"github.com/ArtisanCloud/PowerLibs/v2/object"
	"github.com/ArtisanCloud/PowerLibs/v2/security"
	"github.com/ArtisanCloud/PowerWeChat/v2/src/work/externalContact/messageTemplate/response"
	"github.com/ArtisanCloud/PowerX/config"
	"gorm.io/datatypes"
)

// TableName overrides the table name used by WXMessageTemplate to `profiles`
func (mdl *WXMessageTemplate) TableName() string {
	return mdl.GetTableName(true)
}

type WXMessageTemplateTask struct {
	*database.PowerCompactModel

	UniqueID      string `gorm:"column:index_task_id;index:,unique" json:"taskID"`
	MsgTemplateID string `gorm:"column:msg_template_id; index" json:"msgTemplateID"`
	UserID        string `gorm:"column:user_id" json:"userID"`
	Status        int    `gorm:"column:status" json:"status"`
	SendTime      int    `gorm:"column:send_time" json:"sendTime"`
}

type WXMessageTemplateSend struct {
	*database.PowerCompactModel

	UniqueID       string `gorm:"column:index_result_id;index:,unique" json:"resultID"`
	MsgTemplateID  string `gorm:"column:msg_template_id; index" json:"msgTemplateID"`
	ExternalUserID string `gorm:"column:external_user_id" json:"externalUserID"`
	ChatID         string `gorm:"column:chat_id" json:"chatID"`
	UserID         string `gorm:"column:user_id" json:"userID"`
	Status         int    `gorm:"column:status" json:"status"`
	SendTime       int    `gorm:"column:send_time" json:"sendTime"`
}

type WXMessageTemplate struct {
	*database.PowerCompactModel

	WXMessageTemplateTasks []*WXMessageTemplateTask `gorm:"ForeignKey:MsgTemplateID;references:MsgID" json:"wxMessageTemplateTasks"`
	WXMessageTemplateSends []*WXMessageTemplateSend `gorm:"ForeignKey:MsgTemplateID;references:MsgID" json:"wxMessageTemplateSends"`

	UniqueID        string         `gorm:"column:index_msg_template_id;index:,unique" json:"taskID"`
	SendChatMsgUUID string         `gorm:"column:send_chat_msg_id" json:"sendChatMsgUUID"`
	MsgID           string         `gorm:"column:msg_id" json:"msgID"`
	ChatType        string         `gorm:"column:chat_type" json:"chat_type"`
	ExternalUserIDs datatypes.JSON `gorm:"column:external_user_ids" json:"externalUserIDs"`
	Sender          string         `gorm:"column:sender" json:"sender"`
	Text            datatypes.JSON `gorm:"column:text" json:"text"`
	Attachments     datatypes.JSON `gorm:"column:attachments" json:"attachments"`
	Creator         string         `gorm:"column:creator" json:"creator"`
	CreateTime      int            `gorm:"column:create_time" json:"createTime"`
	CreateType      int            `gorm:"column:create_type" json:"createType"`
	FailList        datatypes.JSON `gorm:"column:fail_list" json:"failList"`
}

const TABLE_NAME_WX_MESSAGE_TEMPLATE = "wx_message_templates"
const WX_MESSAGE_TEMPLATE_UNIQUE_ID = "index_msg_template_id"
const WX_MESSAGE_TEMPLATE_TASK_UNIQUE_ID = "index_task_id"
const WX_MESSAGE_TEMPLATE_SEND_RESULT_UNIQUE_ID = "index_result_id"

const WX_MESSAGE_TEMPLATE_TASK_STATUS_UNSENT = 0            // 未发送
const WX_MESSAGE_TEMPLATE_TASK_STATUS_SENT = 1              // 已发送
const WX_MESSAGE_TEMPLATE_TASK_STATUS_FRIENDLESS_FAILED = 2 // 因客户不是好友导致发送失败
const WX_MESSAGE_TEMPLATE_TASK_STATUS_CONFLICT_FAILED = 3   // 因客户已经收到其他群发消息导致发送失败

const WX_MESSAGE_TEMPLATE_SEND_RESULT_STATUS_UNSENT = 0 // 未发送
const WX_MESSAGE_TEMPLATE_SEND_RESULT_STATUS_SENT = 2   // 已发送

func NewWXMessageTemplate(mapObject *object.Collection) *WXMessageTemplate {
	if mapObject == nil {
		mapObject = object.NewCollection(&object.HashMap{})
	}

	text, _ := object.JsonEncode(mapObject.Get("text", nil))
	externalUserIDs, _ := object.JsonEncode(mapObject.Get("externalUserIDs", nil))
	attachments, _ := object.JsonEncode(mapObject.Get("attachments", nil))
	failList, _ := object.JsonEncode(mapObject.Get("failList", nil))

	tag := &WXMessageTemplate{
		PowerCompactModel: database.NewPowerCompactModel(),
		MsgID:             mapObject.GetString("msgID", ""),
		ChatType:          mapObject.GetString("chat_type", ""),
		ExternalUserIDs:   datatypes.JSON([]byte(externalUserIDs)),
		Sender:            mapObject.GetString("sender", ""),
		Text:              datatypes.JSON([]byte(text)),
		Attachments:       datatypes.JSON([]byte(attachments)),
		FailList:          datatypes.JSON([]byte(failList)),
	}

	return tag
}

func NewWXMessageTemplateTask(msgID string, taskResult *response.Task) *WXMessageTemplateTask {
	task := &WXMessageTemplateTask{
		PowerCompactModel: database.NewPowerCompactModel(),
		MsgTemplateID:     msgID,
		UserID:            taskResult.UserID,
		Status:            taskResult.Status,
		SendTime:          taskResult.SendTime,
	}

	task.UniqueID = task.GetComposedUniqueID()

	return task
}

func NewWXMessageTemplateSendResult(msgID string, result *response.SendResult) *WXMessageTemplateSend {
	sendResult := &WXMessageTemplateSend{
		PowerCompactModel: database.NewPowerCompactModel(),
		MsgTemplateID:     msgID,
		ExternalUserID:    result.ExternalUserID,
		ChatID:            result.ChatID,
		UserID:            result.UserID,
		Status:            result.Status,
		SendTime:          result.SendTime,
	}

	sendResult.UniqueID = sendResult.GetComposedUniqueID()

	return sendResult
}

func (mdl *WXMessageTemplate) GetTableName(needFull bool) string {
	tableName := TABLE_NAME_WX_MESSAGE_TEMPLATE
	if needFull {
		tableName = config.DatabaseConn.Schemas["option"] + "." + tableName
	}
	return tableName
}

func (mdl *WXMessageTemplate) GetComposedUniqueID() string {
	strID := mdl.SendChatMsgUUID + "-" + mdl.Sender
	hashedID := security.HashStringData(strID)

	return hashedID
}

func (mdl *WXMessageTemplateTask) GetComposedUniqueID() string {
	strID := mdl.MsgTemplateID + "-" + mdl.UserID
	hashedID := security.HashStringData(strID)

	return hashedID
}

func (mdl *WXMessageTemplateSend) GetComposedUniqueID() string {
	strID := mdl.MsgTemplateID + "-" + mdl.UserID + mdl.ExternalUserID + "-" + mdl.ChatID + "-" + mdl.UserID
	hashedID := security.HashStringData(strID)

	return hashedID
}

/**
 *  Relationships
 */

/**
 * Scope Where Conditions
 */
