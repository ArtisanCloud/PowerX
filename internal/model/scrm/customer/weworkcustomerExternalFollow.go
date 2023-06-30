package customer

import (
    "PowerX/internal/model"
)

type WeWorkExternalContactFollow struct {
    model.Model

    ExternalUserId string `gorm:"unique"`
    UserId         string
    Remark         string
    Description    string
    Createtime     int
    Tags           string
    WechatChannels string
    RemarkCorpName string
    RemarkMobiles  string
    OpenUserId     string
    AddWay         int
    State          string
}

//
// Table
//  @Description:
//  @receiver e
//  @return string
//
func (e WeWorkExternalContactFollow) Table() string {
    return `we_work_external_contact_follows`
}
