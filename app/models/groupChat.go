package models

import (
	"github.com/ArtisanCloud/PowerLibs/v2/database"
	"github.com/ArtisanCloud/PowerLibs/v2/database/tag"
	"github.com/ArtisanCloud/PowerLibs/v2/object"
	"github.com/ArtisanCloud/PowerX/app/models/wx"
	database2 "github.com/ArtisanCloud/PowerX/configs/database"
	"gorm.io/gorm"
)

// TableName overrides the table name
func (mdl *GroupChat) TableName() string {
	return mdl.GetTableName(true)
}

type GroupChat struct {
	*database.PowerCompactModel

	Tags []*tag.Tag `gorm:"many2many:public.ac_r_tag_to_object;foreignKey:ChatID;joinForeignKey:TaggableObjectID;References:index_tag_id;JoinReferences:tag_id" json:"TaggableID"`

	*wx.WXGroupChat
}

const TABLE_NAME_GROUP_CHAT = "group_chats"
const GROUP_CHAT_UNIQUE_ID = "chat_id"

const GROUP_CHAT_WELCOME_MESSAGE_TYPE_DEFAULT = 1
const GROUP_CHAT_WELCOME_MESSAGE_TYPE_CUSTOMIZED = 2
const GROUP_CHAT_WELCOME_MESSAGE_TYPE_NO_SEND = 3

const GROUP_CHAT_TYPE_CHANNEL = 1

func (mdl *GroupChat) GetTableName(needFull bool) string {
	tableName := TABLE_NAME_GROUP_CHAT
	if needFull {
		tableName = database2.G_DBConfig.Schemas["default"] + "." + database2.G_DBConfig.BaseConfig.Prefix + tableName
	}
	return tableName
}

func (mdl *GroupChat) GetID() int32 {
	return mdl.ID
}

func (mdl *GroupChat) GetForeignRefer() string {
	return GROUP_CHAT_UNIQUE_ID
}
func (mdl *GroupChat) GetForeignReferValue() string {
	return *mdl.WXGroupChat.ChatID
}

func NewGroupChat(mapObject *object.Collection) *GroupChat {
	if mapObject == nil {
		mapObject = object.NewCollection(&object.HashMap{})
	}

	return &GroupChat{
		PowerCompactModel: database.NewPowerCompactModel(),
		WXGroupChat:       wx.NewWXGroupChat(mapObject),
	}
}

func (mdl *GroupChat) LoadTags(db *gorm.DB, conditions *map[string]interface{}) ([]*tag.Tag, error) {
	mdl.Tags = []*tag.Tag{}

	err := database.AssociationRelationship(db, conditions, mdl, "Tags", false).Find(&mdl.Tags)
	if err != nil {
		return nil, err
	}
	return mdl.Tags, err
}

func (mdl *GroupChat) LoadWXGroupChatMembers(db *gorm.DB, conditions *map[string]interface{}) ([]*wx.WXGroupChatMember, error) {
	mdl.WXGroupChatMembers = []*wx.WXGroupChatMember{}

	err := database.AssociationRelationship(db, conditions, mdl, "WXGroupChatMembers", false).Find(&mdl.WXGroupChatMembers)
	if err != nil {
		return nil, err
	}
	return mdl.WXGroupChatMembers, err
}

func (mdl *GroupChat) LoadWXGroupChatAdmins(db *gorm.DB, conditions *map[string]interface{}) ([]*wx.WXGroupChatAdmin, error) {
	mdl.WXGroupChatAdmins = []*wx.WXGroupChatAdmin{}

	err := database.AssociationRelationship(db, conditions, mdl, "WXGroupChatAdmins", false).Find(&mdl.WXGroupChatAdmins)
	if err != nil {
		return nil, err
	}
	return mdl.WXGroupChatAdmins, err
}
