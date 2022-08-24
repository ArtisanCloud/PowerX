package models

import (
	"github.com/ArtisanCloud/PowerLibs/v2/database"
	"github.com/ArtisanCloud/PowerLibs/v2/object"
	"github.com/ArtisanCloud/PowerWeChat/v2/src/work/externalContact/contactWay/request"
	requestMessageTemplate "github.com/ArtisanCloud/PowerWeChat/v2/src/work/externalContact/messageTemplate/request"
	"github.com/ArtisanCloud/PowerX/app/models/wx"
	database2 "github.com/ArtisanCloud/PowerX/configs/database"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// TableName overrides the table name
func (mdl *ContactWay) TableName() string {
	return mdl.GetTableName(true)
}

type ContactWay struct {
	*database.PowerModel

	WXTags []*wx.WXTag `gorm:"many2many:public.ac_r_wx_tag_to_object;foreignKey:ConfigID;joinForeignKey:TaggableObjectID;References:ID;JoinReferences:TagID" json:"wxTags"`

	Name                            string         `gorm:"column:name" json:"name"`
	GroupUUID                       string         `gorm:"column:group_uuid" json:"groupUUID"`
	AllowEmployeeChangeOnlineStatus bool           `gorm:"column:allow_employee_change_online_status" json:"allowEmployeeChangeOnlineStatus"`
	RemarkAccount                   string         `gorm:"remark_account" json:"remarkAccount"`
	RemarkAccountPrefix             bool           `gorm:"remark_account_prefix" json:"remarkAccountPrefix"`
	WelcomeMessageType              int8           `gorm:"welcome_message_type" json:"sendWelcomeMessageType"`
	CodeURL                         string         `gorm:"code_url" json:"codeURL"`
	CustomizedCodeImage             string         `gorm:"customized_code_image" json:"customizedCodeImage"`
	Attachments                     datatypes.JSON `gorm:"attachments" json:"attachments"`
	Status                          int8           `gorm:"column:status" json:"status"`

	*wx.WXContactWay
}

const TABLE_NAME_CONTACT_WAY = "contact_ways"

const CONTACT_WAY_WELCOME_MESSAGE_TYPE_DEFAULT = 1
const CONTACT_WAY_WELCOME_MESSAGE_TYPE_CUSTOMIZED = 2
const CONTACT_WAY_WELCOME_MESSAGE_TYPE_NO_SEND = 3

const CONTACT_WAY_TYPE_CHANNEL = 1

func (mdl *ContactWay) GetTableName(needFull bool) string {
	tableName := TABLE_NAME_CONTACT_WAY
	if needFull {
		tableName = database2.G_DBConfig.Schemas["default"] + "." + database2.G_DBConfig.BaseConfig.Prefix + tableName
	}
	return tableName
}

func NewConclusions() *request.Conclusions {

	conclusions := &request.Conclusions{
		Text:        &requestMessageTemplate.TextOfMessage{},
		Image:       &requestMessageTemplate.Image{},
		Link:        &requestMessageTemplate.Link{},
		MiniProgram: &requestMessageTemplate.MiniProgram{},
	}
	return conclusions
}

func NewContactWay(mapObject *object.Collection) *ContactWay {
	if mapObject == nil {
		mapObject = object.NewCollection(&object.HashMap{})
	}

	attachmentsInterface := mapObject.Get("attachment", nil)
	attachment, _ := object.JsonEncode(attachmentsInterface)

	return &ContactWay{
		PowerModel:                      database.NewPowerModel(),
		Name:                            mapObject.GetString("name", ""),
		GroupUUID:                       mapObject.GetString("groupUUID", ""),
		AllowEmployeeChangeOnlineStatus: mapObject.GetBool("allowEmployeeChangeOnlineStatus", false),
		RemarkAccount:                   mapObject.GetString("remarkAccount", ""),
		RemarkAccountPrefix:             mapObject.GetBool("remarkAccountPrefix", true),
		WelcomeMessageType:              mapObject.GetInt8("welcomeMessageType", CONTACT_WAY_WELCOME_MESSAGE_TYPE_DEFAULT),
		CodeURL:                         mapObject.GetString("codeURL", ""),
		CustomizedCodeImage:             mapObject.GetString("customizedCodeImage", ""),
		Attachments:                     datatypes.JSON([]byte(attachment)),
		Status:                          mapObject.GetInt8("status", database.MODEL_STATUS_ACTIVE),
		WXContactWay:                    wx.NewWXContactWay(mapObject),
	}
}

func (mdl *ContactWay) GetForeignRefer() string {
	return "config_id"
}
func (mdl *ContactWay) GetForeignReferValue() string {
	return mdl.ConfigID
}

func (mdl *ContactWay) GetForeignValue() string {
	return mdl.ConfigID
}

func (mdl *ContactWay) LoadWXTags(db *gorm.DB, conditions *map[string]interface{}) ([]*wx.WXTag, error) {
	mdl.WXTags = []*wx.WXTag{}

	err := database.AssociationRelationship(db, conditions, mdl, "WXTags", false).Find(&mdl.WXTags)
	if err != nil {
		return nil, err
	}
	return mdl.WXTags, err
}
