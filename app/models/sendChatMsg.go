package models

import (
	"github.com/ArtisanCloud/PowerLibs/v2/database"
	"github.com/ArtisanCloud/PowerLibs/v2/object"
	"github.com/ArtisanCloud/PowerX/app/models/wx"
	globalConfig "github.com/ArtisanCloud/PowerX/configs/database"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"time"
)

// TableName overrides the table name
func (mdl *SendChatMsg) TableName() string {
	return mdl.GetTableName(true)
}

type FilterCustomers struct {
	ToFilterCustomers      bool           `gorm:"column:to_filter_customers" json:"toFilterCustomers"`
	FilterGender           int8           `gorm:"column:filter_gender" json:"filterGender"`
	FilterChatIDs          datatypes.JSON `gorm:"column:filter_chat_ids" json:"filterChatIDs"`
	FilterStartDate        time.Time      `gorm:"column:filter_start_date" json:"filterStartDate"`
	FilterEndDate          time.Time      `gorm:"column:filter_end_date" json:"filterEndDate"`
	FilterWXTagIDs         datatypes.JSON `gorm:"column:filter_wx_tag_ids" json:"filterWXTagIDs"`
	FilterTagIDs           datatypes.JSON `gorm:"column:filter_tag_ids" json:"filterTagIDs"`
	FilterExcludedWXTagIDs datatypes.JSON `gorm:"column:filter_excluded_wx_tag_ids" json:"filterExcludedWXTagIDs"`
}

type SendChatMsg struct {
	*database.PowerModel

	WXMessageTemplates []*wx.WXMessageTemplate `gorm:"ForeignKey:SendChatMsgUUID;references:UUID" json:"wxMessageTemplates"`

	ByAllEmployees  bool           `gorm:"column:by_all_employees" json:"byAllEmployees"`
	Senders         datatypes.JSON `gorm:"column:senders" json:"senders"`
	SendImmediately bool           `gorm:"column:send_immediately" json:"sendImmediately"`
	SendOnTime      time.Time      `gorm:"column:send_on_time" json:"sendOnTime"`
	SendStatus      int8           `gorm:"column:send_status" json:"sendStatus"`
	*FilterCustomers
}

const TABLE_NAME_SEND_CHAT_MSG = "send_chat_msgs"
const SEND_CHAT_MSG_UNIQUE_ID = "uuid"

const SEND_CHAT_MESSAGE_TEMPLATE_GENDER_TYPE_ALL = 0
const SEND_CHAT_MESSAGE_TEMPLATE_GENDER_TYPE_MALE = 1
const SEND_CHAT_MESSAGE_TEMPLATE_GENDER_TYPE_FEMALE = 2
const SEND_CHAT_MESSAGE_TEMPLATE_GENDER_TYPE_UNKNOW = 3

const SEND_CHAT_MESSAGE_SEND_STATUS_UNSENT = 0 // 为发送
const SEND_CHAT_MESSAGE_SEND_STATUS_SENT = 1   // 已发送

const SEND_CHAT_MSG_TYPE_CHANNEL = 1

func (mdl *SendChatMsg) GetTableName(needFull bool) string {
	tableName := TABLE_NAME_SEND_CHAT_MSG
	if needFull {
		tableName = globalConfig.G_DBConfig.Schemas["option"] + "." + tableName
	}
	return tableName
}

func (mdl *SendChatMsg) GetID() int32 {
	return mdl.ID
}

func (mdl *SendChatMsg) GetForeignRefer() string {
	return SEND_CHAT_MSG_UNIQUE_ID
}
func (mdl *SendChatMsg) GetForeignReferValue() string {
	return mdl.PowerModel.UUID
}

func NewSendChatMsg(mapObject *object.Collection) *SendChatMsg {
	if mapObject == nil {
		mapObject = object.NewCollection(&object.HashMap{})
	}

	Senders, _ := object.JsonEncode(mapObject.GetStringArray("Senders", nil))
	filterChatIDs, _ := object.JsonEncode(mapObject.GetStringArray("FilterChatIDs", nil))
	filterWXTagIDs, _ := object.JsonEncode(mapObject.GetStringArray("filterWXTagIDs", nil))
	filterTagIDs, _ := object.JsonEncode(mapObject.GetStringArray("filterTagIDs", nil))
	filterExcludedWXTagIDs, _ := object.JsonEncode(mapObject.GetStringArray("filterExcludedTagIDs", nil))

	return &SendChatMsg{
		PowerModel: database.NewPowerModel(),

		Senders: datatypes.JSON(Senders),
		FilterCustomers: &FilterCustomers{
			ToFilterCustomers:      mapObject.GetBool("toFilterCustomers", false),
			FilterGender:           mapObject.GetInt8("filterGender", SEND_CHAT_MESSAGE_TEMPLATE_GENDER_TYPE_ALL),
			FilterChatIDs:          datatypes.JSON([]byte(filterChatIDs)),
			FilterStartDate:        mapObject.GetDateTime("filterStartDate", time.Now()),
			FilterEndDate:          mapObject.GetDateTime("filterEndDate", time.Now()),
			FilterWXTagIDs:         datatypes.JSON(filterWXTagIDs),
			FilterTagIDs:           datatypes.JSON(filterTagIDs),
			FilterExcludedWXTagIDs: datatypes.JSON(filterExcludedWXTagIDs),
		},
		SendImmediately: mapObject.GetBool("sendImmediately", true),
		SendOnTime:      mapObject.GetDateTime("sendOnTime", time.Now().Add(1*time.Hour)),
	}
}

func (mdl *SendChatMsg) LoadWXMessageTemplates(db *gorm.DB, conditions *map[string]interface{}) ([]*wx.WXMessageTemplate, error) {
	mdl.WXMessageTemplates = []*wx.WXMessageTemplate{}

	err := database.AssociationRelationship(db, conditions, mdl, "WXMessageTemplates", false).Find(&mdl.WXMessageTemplates)
	if err != nil {
		return nil, err
	}
	return mdl.WXMessageTemplates, err
}
