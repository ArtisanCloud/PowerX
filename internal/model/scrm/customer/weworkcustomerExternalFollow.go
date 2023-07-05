package customer

import (
    "PowerX/internal/model"
    "gorm.io/gorm"
    "gorm.io/gorm/clause"
)

type WeWorkExternalContactFollow struct {
    model.Model

    ExternalUserId string `gorm:"comment:客户ID;column:external_user_id;unique" json:"external_user_id"`
    UserId         string `gorm:"comment:员工ID;column:user_id" json:"user_id"`
    Remark         string `gorm:"comment:备注;column:remark" json:"remark"`
    Description    string `gorm:"comment:描述;column:description" json:"description"`
    Createtime     int    `gorm:"comment:创建时间;column:add_way" json:"createtime"`
    Tags           string `gorm:"comment:TAG;column:add_way" json:"tags"`
    WechatChannels string `gorm:"comment:Channels;column:add_way" json:"wechat_channels"`
    RemarkCorpName string `gorm:"comment:企业备注;column:add_way" json:"remark_corp_name"`
    RemarkMobiles  string `gorm:"comment:电话备注;column:add_way" json:"remark_mobiles"`
    OpenUserId     string `gorm:"comment:开放ID;column:add_way" json:"open_user_id"`
    AddWay         int    `gorm:"comment:AddWay;column:add_way" json:"add_way"`
    State          string `gorm:"comment:State;column:state" json:"state"`
}

//
// Table
//  @Description:
//  @receiver e
//  @return string
//
func (e WeWorkExternalContactFollow) TableName() string {
    return `we_work_external_contact_follows`
}

//
// Query
//  @Description:
//  @receiver e
//  @param db
//  @return follows
//
func (e WeWorkExternalContactFollow) Query(db *gorm.DB) (follows []*WeWorkExternalContactFollow) {

    err := db.Model(e).Find(&follows).Error
    if err != nil {
        panic(err)
    }
    return follows

}

//
// Action
//  @Description:
//  @receiver e
//  @param db
//  @param contacts
//
func (e *WeWorkExternalContactFollow) Action(db *gorm.DB, contacts []*WeWorkExternalContactFollow) {

    err := db.Table(e.TableName()).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "external_user_id"}}, UpdateAll: true}).CreateInBatches(&contacts, 100).Error
    if err != nil {
        panic(err)
    }

}
