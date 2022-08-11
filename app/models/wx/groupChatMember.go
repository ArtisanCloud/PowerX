package wx

import (
	"github.com/ArtisanCloud/PowerLibs/v2/object"
	"github.com/ArtisanCloud/PowerLibs/v2/security"
	"github.com/ArtisanCloud/PowerX/config/database"
	"gorm.io/datatypes"
)

// TableName overrides the table name used by WXGroupChatMember to `profiles`
func (mdl *WXGroupChatMember) TableName() string {
	return mdl.GetTableName(true)
}

type WXGroupChatMember struct {
	WXGroupChat *WXGroupChat `gorm:"foreignKey:WXGroupChatID;references:ChatID" json:"wxGroupChat"`

	UniqueID      string         `gorm:"index:index_user_id;index:index_group_chat_id;index;column:index_wx_group_chat_member_id;unique"`
	WXGroupChatID *string        `gorm:"column:wx_group_chat_id;index:index_group_chat_id" json:"WXGroupChatID"`
	UserID        *string        `gorm:"column:user_id;index:index_user_id" json:"userID"`
	Type          *int           `gorm:"column:type" json:"type"`
	JoinTime      *int           `gorm:"column:join_time" json:"joinTime"`
	JoinScene     *int           `gorm:"column:join_scene" json:"joinScene"`
	Invitor       datatypes.JSON `gorm:"column:invitor" json:"invitor"`
	GroupNickName *string        `gorm:"column:group_nickname" json:"groupNickName"`
	Name          *string        `gorm:"column:name" json:"name"`
	UnionID       *string        `gorm:"column:union_id" json:"unionID"`
}

const TABLE_NAME_WX_GROUP_CHAT_MEMBER = "wx_group_chat_members"
const WX_GROUP_CHAT_MEMBER_UNIQUE_ID = "index_wx_group_chat_member_id"

func NewWXGroupChatMember(mapObject *object.Collection) *WXGroupChatMember {
	if mapObject == nil {
		mapObject = object.NewCollection(&object.HashMap{})
	}

	invitor, _ := object.JsonEncode(mapObject.GetStringArray("invitor", nil))

	chat := &WXGroupChatMember{

		WXGroupChatID: mapObject.GetStringPointer("order", ""),
		UserID:        mapObject.GetStringPointer("wxChatGroupID", ""),

		Type:          mapObject.GetIntPointer("type", -1),
		JoinTime:      mapObject.GetIntPointer("joinTime", -1),
		JoinScene:     mapObject.GetIntPointer("joinScene", -1),
		Invitor:       datatypes.JSON([]byte(invitor)),
		GroupNickName: mapObject.GetStringPointer("groupNickName", ""),
		Name:          mapObject.GetStringPointer("name", ""),
		UnionID:       mapObject.GetStringPointer("unionID", ""),
	}

	return chat
}

func (mdl *WXGroupChatMember) GetTableName(needFull bool) string {
	tableName := TABLE_NAME_WX_GROUP_CHAT_MEMBER
	if needFull {
		tableName = database.G_DBConfig.Schemas["option"] + "." + tableName
	}
	return tableName
}

func (mdl *WXGroupChatMember) GetComposedUniqueID() string {

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
