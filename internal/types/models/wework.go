package models

import "github.com/ArtisanCloud/PowerLibs/v3/object"

type WeWorkExternalContact struct {
	*Model
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
