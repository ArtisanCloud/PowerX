package wx

import (
	databasePowerLib "github.com/ArtisanCloud/PowerLibs/v2/database"
	"github.com/ArtisanCloud/PowerLibs/v2/object"
	"github.com/ArtisanCloud/PowerX/config"
)

// TableName overrides the table name used by WXGroupChat to `profiles`
func (mdl *WXGroupChat) TableName() string {
	return mdl.GetTableName(true)
}

type WXGroupChat struct {
	WXGroupChatMembers []*WXGroupChatMember `gorm:"foreignKey:WXGroupChatID;references:ChatID" json:"wxGroupChatMembers"`
	WXGroupChatAdmins  []*WXGroupChatAdmin  `gorm:"foreignKey:WXGroupChatID;references:ChatID" json:"wxGroupChatAdmins"`

	ChatID     *string `gorm:"column:chat_id;index:,unique;" json:"chatID"`
	Status     int8    `gorm:"column:status;" json:"status"`
	Name       *string `gorm:"column:name" json:"name"`
	Owner      *string `gorm:"column:owner" json:"owner"`
	CreateTime *int    `gorm:"column:create_time" json:"createTime"`
	Notice     *string `gorm:"column:notice" json:"notice"`
}

const TABLE_NAME_WX_GROUP_CHAT = "wx_group_chats"
const WX_GROUP_CHAT_UNIQUE_ID = "chat_id"

const WX_GROUP_CHAT_STATUS_NORNAL int8 = 0      // - 跟进人正常
const WX_GROUP_CHAT_STATUS_RESIGNED int8 = 1    // - 跟进人离职
const WX_GROUP_CHAT_STATUS_transfering int8 = 2 // - 离职继承中
const WX_GROUP_CHAT_STATUS_transferred int8 = 3 // - 离职继承完成

func NewWXGroupChat(mapObject *object.Collection) *WXGroupChat {
	if mapObject == nil {
		mapObject = object.NewCollection(&object.HashMap{})
	}

	chat := &WXGroupChat{

		ChatID:     mapObject.GetStringPointer("chatID", ""),
		Name:       mapObject.GetStringPointer("name", ""),
		Owner:      mapObject.GetStringPointer("owner", ""),
		CreateTime: mapObject.GetIntPointer("createTime", -1),
		Notice:     mapObject.GetStringPointer("notice", ""),
	}

	return chat
}

func (mdl *WXGroupChat) GetTableName(needFull bool) string {
	tableName := TABLE_NAME_WX_GROUP_CHAT
	if needFull {
		tableName = databasePowerLib.GetTableFullName(config.G_DBConfig.Schemas.Default, config.G_DBConfig.Prefix, tableName)
	}
	return tableName
}

/**
 *  Relationships
 */

/**
 * Scope Where Conditions
 */
