package wx

import (
	"github.com/ArtisanCloud/PowerLibs/v2/object"
	"github.com/ArtisanCloud/PowerX/config/database"
)

// TableName overrides the table name used by WXEmployee to `profiles`
func (mdl *WXEmployee) TableName() string {
	return mdl.GetTableName(true)
}

type WXEmployee struct {
	WXAlias           string            `gorm:"column:wx_alias" json:"wxAlias"`
	WXAvatar          string            `gorm:"column:wx_avatar" json:"wxAvatar"`
	WXDepartments     string            `gorm:"column:wx_department" json:"wxDepartment"`
	WXEmail           string            `gorm:"column:wx_email" json:"wxEmail"`
	WXEnable          int               `gorm:"column:wx_enable" json:"wxEnable"`
	WXEnglishName     string            `gorm:"column:wx_englishName" json:"wxEnglishName"`
	WXExtAttr         string            `gorm:"column:wx_extAttr" json:"wxExtAttr"`
	WXExternalProfile string            `gorm:"column:wx_externalProfile" json:"wxExternalProfile"`
	WXGender          string            `gorm:"column:wx_gender" json:"wxGender"`
	WXHideMobile      int               `gorm:"column:wx_hideMobile" json:"wxHideMobile"`
	WXIsLeader        int               `gorm:"column:wx_isLeader" json:"wxIsLeader"`
	WXIsLeaderInDept  string            `gorm:"column:wx_isLeaderInDept" json:"wxIsLeaderInDept"`
	WXMainDepartment  int               `gorm:"column:wx_mainDepartment" json:"wxMainDepartment"`
	WXMobile          string            `gorm:"column:wx_mobile" json:"wxMobile"`
	WXName            string            `gorm:"column:wx_name" json:"wxName"`
	WXOrder           string            `gorm:"column:wx_order" json:"wxOrder"`
	WXPosition        string            `gorm:"column:wx_position" json:"wxPosition"`
	WXQrCode          string            `gorm:"column:wx_qrCode" json:"wxQrCode"`
	WXStatus          int               `gorm:"column:wx_status" json:"wxStatus"`
	WXTelephone       string            `gorm:"column:wx_telephone" json:"wxTelephone"`
	WXThumbAvatar     string            `gorm:"column:wx_thumbAvatar" json:"wxThumbAvatar"`
	WXCorpID          object.NullString `gorm:"index:index_wx_corp_id;column:wx_corp_id" json:"wxCorpID"`
	WXOpenUserID      object.NullString `gorm:"column:wx_open_user_id;" json:"wxOpenUserID"`
	WXUserID          object.NullString `gorm:"index:index_user_id; column:wx_user_id;index:,unique;" json:"wxUserID"`
	WXOpenID          object.NullString `gorm:"index:index_wx_open_id;column:wx_open_id;" json:"wxOpenID"`
}

const TABLE_NAME_EMPLOYEE = "wx_employees"

const WX_EMPLOYEE_STATUS_ACTIVE = 1   // 已激活
const WX_EMPLOYEE_STATUS_BLOCK = 2    // 已禁用
const WX_EMPLOYEE_STATUS_UNACTIVE = 4 // 未激活
const WX_EMPLOYEE_STATUS_QUIT = 5     // 退出企业

func NewWXEmployee(mapObject *object.Collection) *WXEmployee {
	if mapObject == nil {
		mapObject = object.NewCollection(&object.HashMap{})
	}

	employee := &WXEmployee{}

	return employee
}

func (mdl *WXEmployee) GetTableName(needFull bool) string {
	tableName := TABLE_NAME_EMPLOYEE
	if needFull {
		tableName = database.G_DBConfig.Schemas["option"] + "." + tableName
	}
	return tableName
}

/**
 *  Relationships
 */

/**
 * Scope Where Conditions
 */
