package wx

import (
	"database/sql"
	"github.com/ArtisanCloud/PowerLibs/v2/object"
	"github.com/ArtisanCloud/PowerX/configs/database"
	"gorm.io/gorm"
)

// TableName overrides the table name used by WXCustomer to `profiles`
func (mdl *WXCustomer) TableName() string {
	return mdl.GetTableName(true)
}

type WXCustomer struct {
	CorpID         object.NullString `gorm:"index:index_corp_id;column:corp_id" json:"corpID"`
	AppID          object.NullString `gorm:"index:index_app_id;column:app_id" json:"appID"`
	ExternalUserID object.NullString `gorm:"index:index_external_user_id;column:external_user_id;not null;index:,unique" json:"externalUserID"`
	OpenID         object.NullString `gorm:"index:index_customer_open_id;column:open_id;index:,unique" json:"openID"`
	UnionID        object.NullString `gorm:"index:index_union_id;column:union_id" json:"unionID"`

	Name            string `gorm:"column:name" json:"name"`
	Mobile          string `gorm:"column:mobile" json:"mobile"`
	Position        string `gorm:"column:position" json:"position"`
	Avatar          string `gorm:"column:avatar" json:"avatar"`
	CorpName        string `gorm:"column:corp_name" json:"corpName"`
	CorpFullName    string `gorm:"column:corp_full_name" json:"corpFullName"`
	ExternalProfile string `gorm:"column:external_profile" json:"externalProfile"`
	Gender          int    `gorm:"column:gender" json:"gender"`
	WXType          int8   `gorm:"column:wx_type" json:"wxType"`
}

const TABLE_NAME_WX_CUSTOMER = "wx_customers"

const WX_TYPE_WEWORK = 1
const WX_TYPE_MINI_PROGRAM = 2
const WX_OFFICIAL_ACCOUNT = 3

const WX_CUSTOMER_GENDER_MALE = 1
const WX_CUSTOMER_GENDER_FEMALE = 2

func NewWXCustomer(mapObject *object.Collection) *WXCustomer {
	if mapObject == nil {
		mapObject = object.NewCollection(&object.HashMap{})
	}

	// dealing with unique ids
	openID := object.NewNullString("", false)
	strOpenID := mapObject.GetString("openID", "")
	if strOpenID != "" {
		openID = object.NewNullString(strOpenID, true)
	} else {
		return nil
	}

	externalUserID := object.NewNullString("", false)
	strExternalUserID := mapObject.GetString("external_contact.external_user_id", "")

	corpID := object.NewNullString("", false)
	strCorpID := mapObject.GetString("corpID", "")

	appID := object.NewNullString("", false)
	strAppID := mapObject.GetString("appID", "")

	// https://artisancloud.feishu.cn/docs/doccng48rV06m9XvtqIQyCbkD8b
	// wechat work external customer
	if strExternalUserID != "" && strCorpID != "" {
		externalUserID = object.NewNullString(strExternalUserID, true)
		corpID = object.NewNullString(strCorpID, true)
	} else
	// official or mini program customer
	if strAppID != "" {
		appID = object.NewNullString(strAppID, true)

	} else {
		// invalid wx customer
		return nil
	}

	strUnionID := mapObject.GetString("external_contact.unionid", "")
	unionID := object.NewNullString("", false)
	if strUnionID != "" {
		unionID = object.NewNullString(strUnionID, true)
	}

	customer := &WXCustomer{
		CorpID:         corpID,
		AppID:          appID,
		ExternalUserID: externalUserID,
		OpenID:         openID,
		UnionID:        unionID,
		Name:           mapObject.GetString("external_contact.name", ""),
		//Mobile:          mapObject.GetString("external_contact.mobile", ""),
		Position:        mapObject.GetString("external_contact.position", ""),
		Avatar:          mapObject.GetString("external_contact.avatar", ""),
		CorpName:        mapObject.GetString("external_contact.corp_name", ""),
		CorpFullName:    mapObject.GetString("external_contact.corp_full_name", ""),
		Gender:          int(mapObject.GetFloat64("external_contact.gender", 0)),
		WXType:          mapObject.GetInt8("WXType", WX_TYPE_WEWORK),
		ExternalProfile: mapObject.GetString("externalProfile", ""),
	}

	//customer.WXIndexID = customer.GetComposedUniqueWXID()

	return customer
}

func (mdl *WXCustomer) GetTableName(needFull bool) string {
	tableName := TABLE_NAME_WX_CUSTOMER
	if needFull {
		tableName = database.G_DBConfig.Schemas["option"] + "." + tableName
	}
	return tableName
}

func (mdl *WXCustomer) GetComposedUniqueWXID() object.NullString {
	if mdl.OpenID.String != "" || mdl.ExternalUserID.String != "" {
		strUniqueID := mdl.OpenID.String + "-" + mdl.ExternalUserID.String
		return object.NewNullString(strUniqueID, true)
	} else {
		return object.NewNullString("", false)
	}
}

/**
 *  Relationships
 */

/**
 * Scope Where Conditions
 */
func (mdl *WXCustomer) WhereWXCustomerName(uuidOrPhone string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("uuid=@value OR mobile=@value", sql.Named("value", uuidOrPhone))
	}
}

func (mdl *WXCustomer) WhereMobile(phone string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("mobile=@value", sql.Named("value", phone))
	}
}

func (mdl *WXCustomer) WhereIsActive(db *gorm.DB) func(db *gorm.DB) *gorm.DB {
	//return db.Where("status = ?", "active")
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("active = ?", true)
	}
}
