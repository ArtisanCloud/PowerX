package wx

import (
	"github.com/ArtisanCloud/PowerLibs/v2/object"
	"github.com/ArtisanCloud/PowerLibs/v2/security"
	"github.com/ArtisanCloud/PowerX/configs/database"
)

// TableName overrides the table name used by WXGroupChatAdmin to `profiles`
func (mdl *WXGroupChatAdmin) TableName() string {
	return mdl.GetTableName(true)
}

type WXGroupChatAdmin struct {
	WXGroupChat *WXGroupChat `gorm:"foreignKey:WXGroupChatID;references:ChatID" json:"wxGroupChat"`

	UniqueID      string  `gorm:"index:index_user_id;index:index_group_chat_id;index;column:index_wx_group_chat_admin_id;unique"`
	WXGroupChatID *string `gorm:"column:wx_group_chat_id;index:index_group_chat_id" json:"WXGroupChatID"`
	UserID        *string `gorm:"column:user_id;index:index_user_id" json:"userID"`
}

const TABLE_NAME_WX_GROUP_CHAT_ADMIN = "wx_group_chat_admins"
const WX_GROUP_CHAT_ADMIN_UNIQUE_ID = "index_wx_group_chat_admin_id"

func NewWXGroupChatAdmin(mapObject *object.Collection) *WXGroupChatAdmin {
	if mapObject == nil {
		mapObject = object.NewCollection(&object.HashMap{})
	}

	chat := &WXGroupChatAdmin{

		WXGroupChatID: mapObject.GetStringPointer("order", ""),
		UserID:        mapObject.GetStringPointer("wxChatGroupID", ""),
	}

	return chat
}

func (mdl *WXGroupChatAdmin) GetTableName(needFull bool) string {
	tableName := TABLE_NAME_WX_GROUP_CHAT_ADMIN
	if needFull {
		tableName = database.G_DBConfig.Schemas["option"] + "." + tableName
	}
	return tableName
}

func (mdl *WXGroupChatAdmin) GetComposedUniqueID() string {

	strKey := *mdl.WXGroupChatID + "-" + *mdl.UserID
	hashKey := security.HashStringData(strKey)

	return hashKey
}

/**
 *  Relationships
 */

/**
 * Scope Where Conditions
 */
