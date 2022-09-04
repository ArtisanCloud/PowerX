package wx

import (
	"database/sql"
	databasePowerLib "github.com/ArtisanCloud/PowerLibs/v2/database"
	"github.com/ArtisanCloud/PowerLibs/v2/object"
	"github.com/ArtisanCloud/PowerX/config"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// TableName overrides the table name used by WXContactWay to `profiles`
func (mdl *WXContactWay) TableName() string {
	return mdl.GetTableName(true)
}

type WXContactWay struct {
	ConfigID      string         `gorm:"column:config_id;index:,unique;" json:"configID"`
	Type          *int           `gorm:"column:type" json:"type"`
	Scene         *int           `gorm:"column:scene" json:"scene"`
	Style         *int           `gorm:"column:style" json:"style"`
	Remark        *string        `gorm:"column:remark" json:"remark"`
	SkipVerify    *bool          `gorm:"column:skip_verify" json:"skipVerify"`
	State         *string        `gorm:"column:state" json:"state"`
	User          datatypes.JSON `gorm:"column:user" json:"user"`
	Party         datatypes.JSON `gorm:"column:party" json:"party"`
	IsTemp        *bool          `gorm:"column:is_temp" json:"isTemp"`
	ExpiresIn     *int           `gorm:"column:expires_in" json:"expiresIn"`
	ChatExpiresIn *int           `gorm:"column:chat_expires_in" json:"chatExpiresIn"`
	UnionID       *string        `gorm:"column:union_id" json:"unionID"`
	Conclusions   datatypes.JSON `gorm:"column:conclusions" json:"conclusions"`
	//ConclusionsContent *string              `gorm:"column:conclusions_content" json:"conclusionsContent"`
}

const TABLE_NAME_WX_CONTACT_WAY = "wx_tags"
const WX_CONTACT_WAY_UNIQUE_ID = "config_id"

const CONTACT_WAY_TYPE_SINGLE = 1
const CONTACT_WAY_TYPE_MULTIPLE = 2

const CONTACT_WAY_SCENE_MINI_PROGRAM = 1
const CONTACT_WAY_SCENE_QR_CODE = 2

const CONTACT_WAY_STYLE_1 = 1
const CONTACT_WAY_STYLE_2 = 2
const CONTACT_WAY_STYLE_3 = 3

func NewWXContactWay(mapObject *object.Collection) *WXContactWay {
	if mapObject == nil {
		mapObject = object.NewCollection(&object.HashMap{})
	}

	conclusions := ""
	conclusionsInterface := mapObject.Get("conclusions", nil)
	if conclusionsInterface != nil {
		conclusions, _ = object.JsonEncode(conclusionsInterface)
	}

	usersInterface := mapObject.GetStringArray("user", nil)
	users, _ := object.JsonEncode(usersInterface)
	partiesInterface := mapObject.GetStringArray("party", nil)
	parties, _ := object.JsonEncode(partiesInterface)

	tag := &WXContactWay{
		ConfigID:      mapObject.GetString("configID", ""),
		Type:          mapObject.GetIntPointer("type", CONTACT_WAY_TYPE_SINGLE),
		Scene:         mapObject.GetIntPointer("scene", CONTACT_WAY_SCENE_QR_CODE),
		Style:         mapObject.GetIntPointer("style", CONTACT_WAY_STYLE_1),
		Remark:        mapObject.GetStringPointer("remark", ""),
		SkipVerify:    mapObject.GetBoolPointer("skipVerify", true),
		State:         mapObject.GetStringPointer("state", ""),
		User:          datatypes.JSON([]byte(users)),
		Party:         datatypes.JSON([]byte(parties)),
		IsTemp:        mapObject.GetBoolPointer("isTemp", false),
		ExpiresIn:     mapObject.GetIntPointer("expiresIn", 7*config.DAY),
		ChatExpiresIn: mapObject.GetIntPointer("chatExpiresIn", 24*config.HOUR),
		UnionID:       mapObject.GetStringPointer("unionID", ""),
		Conclusions:   datatypes.JSON([]byte(conclusions)),
		//ConclusionsContent: mapObject.GetStringPointer("conclusionsContent", ""),
	}

	return tag
}

func (mdl *WXContactWay) GetTableName(needFull bool) string {
	tableName := TABLE_NAME_WX_CONTACT_WAY
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
func (mdl *WXContactWay) WhereWXContactWayName(uuidOrPhone string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("uuid=@value OR mobile=@value", sql.Named("value", uuidOrPhone))
	}
}

func (mdl *WXContactWay) WhereIsActive(db *gorm.DB) func(db *gorm.DB) *gorm.DB {
	//return db.Where("status = ?", "active")
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("active = ?", true)
	}
}
