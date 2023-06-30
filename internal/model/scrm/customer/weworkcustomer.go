package customer

import (
    "PowerX/internal/model"
)

type WeWorkExternalContacts struct {
    model.Model

    ExternalUserId string `gorm:"unique;not null;" json:"externalUserId"`
    AppId          string `gorm:"index:idx_app_id;column:app_id" json:"appId"`
    CorpId         string `gorm:"index:idx_corp_id;column:corp_id" json:"corpId"`
    OpenId         string `gorm:"index:idx_customer_open_id;column:open_id;" json:"openId"`
    UnionId        string `gorm:"index:idx_union_id;column:union_id" json:"unionId"`

    Name            string `gorm:"column:name" json:"name"`
    Mobile          string `gorm:"column:mobile" json:"mobile"`
    Position        string `gorm:"column:position" json:"position"`
    Avatar          string `gorm:"column:avatar" json:"avatar"`
    CorpName        string `gorm:"column:corp_name" json:"corpName"`
    CorpFullName    string `gorm:"column:corp_full_name" json:"corpFullName"`
    ExternalProfile string `gorm:"column:external_profile" json:"externalProfile"`
    Gender          int    `gorm:"column:gender" json:"gender"`
    WXType          int8   `gorm:"column:wx_type" json:"wxType"`
    Status          int
    Active          bool `gorm:"active"`
}

//
// Table
//  @Description:
//  @receiver e
//  @return string
//
func (e WeWorkExternalContacts) Table() string {
    return `we_work_external_contacts`
}
