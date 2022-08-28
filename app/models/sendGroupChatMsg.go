package models

import (
	databasePowerLib "github.com/ArtisanCloud/PowerLibs/v2/database"
	"github.com/ArtisanCloud/PowerLibs/v2/object"
	"github.com/ArtisanCloud/PowerX/app/models/wx"
	databaseConfig "github.com/ArtisanCloud/PowerX/config"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"time"
)

// TableName overrides the table name
func (mdl *SendGroupChatMsg) TableName() string {
	return mdl.GetTableName(true)
}

type SendGroupChatMsg struct {
	*databasePowerLib.PowerModel

	WXMessageTemplates []*wx.WXMessageTemplate `gorm:"ForeignKey:SendChatMsgUUID;references:UUID" json:"wxMessageTemplates"`

	GroupChatMsgName string         `gorm:"column:group_chat_msg_name" json:"groupChatMsgName"`
	Senders          datatypes.JSON `gorm:"column:senders" json:"senders"`
	SendImmediately  bool           `gorm:"column:send_immediately" json:"sendImmediately"`
	SendOnTime       time.Time      `gorm:"column:send_on_time" json:"sendOnTime"`
	SendStatus       int8           `gorm:"column:send_status" json:"sendStatus"`
}

const TABLE_NAME_SEND_GROUP_CHAT_MSG = "send_group_chat_msgs"
const SEND_GROUP_CHAT_MSG_UNIQUE_ID = "uuid"

const SEND_GROUP_CHAT_MESSAGE_SEND_STATUS_UNSENT = 0 // 为发送
const SEND_GROUP_CHAT_MESSAGE_SEND_STATUS_SENT = 1   // 已发送

const SEND_GROUP_CHAT_MSG_TYPE_CHANNEL = 1

func (mdl *SendGroupChatMsg) GetTableName(needFull bool) string {
	tableName := TABLE_NAME_SEND_GROUP_CHAT_MSG
	if needFull {
		tableName = databasePowerLib.GetTableFullName(databaseConfig.G_DBConfig.Schemas.Default, databaseConfig.G_DBConfig.Prefix, tableName)
	}
	return tableName
}

func (mdl *SendGroupChatMsg) GetID() int32 {
	return mdl.ID
}

func (mdl *SendGroupChatMsg) GetForeignRefer() string {
	return SEND_GROUP_CHAT_MSG_UNIQUE_ID
}
func (mdl *SendGroupChatMsg) GetForeignReferValue() string {
	return mdl.UUID
}

func NewSendGroupChatMsg(mapObject *object.Collection) *SendGroupChatMsg {
	if mapObject == nil {
		mapObject = object.NewCollection(&object.HashMap{})
	}

	Senders, _ := object.JsonEncode(mapObject.GetStringArray("Senders", nil))

	return &SendGroupChatMsg{
		PowerModel: databasePowerLib.NewPowerModel(),

		GroupChatMsgName: mapObject.GetString("groupChatMsgName", ""),
		Senders:          datatypes.JSON(Senders),
		SendImmediately:  mapObject.GetBool("sendImmediately", true),
		SendOnTime:       mapObject.GetDateTime("sendOnTime", time.Now().Add(1*time.Hour)),
	}
}

func (mdl *SendGroupChatMsg) LoadWXMessageTemplates(db *gorm.DB, conditions *map[string]interface{}) ([]*wx.WXMessageTemplate, error) {
	mdl.WXMessageTemplates = []*wx.WXMessageTemplate{}

	err := databasePowerLib.AssociationRelationship(db, conditions, mdl, "WXMessageTemplates", false).Find(&mdl.WXMessageTemplates)
	if err != nil {
		return nil, err
	}
	return mdl.WXMessageTemplates, err
}
