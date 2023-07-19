package scene

import (
    "PowerX/internal/model"
    "gorm.io/gorm"
    "gorm.io/gorm/clause"
)

type SceneQrcode struct {
    model.Model
    QId                string `gorm:"comment:唯一标识;unique;column:qid" json:"qid"`
    Name               string `gorm:"comment:活码名称;column:name" json:"name"`
    Desc               string `gorm:"comment:描述;column:desc" json:"desc"`
    Owner              string `gorm:"comment:所属人(userId逗号隔开);column:owner" json:"owner"`
    RealQrcodeLink     string `gorm:"comment:真实二维码图片;column:real_qrcode_link" json:"real_qrcode_link"`
    Platform           int    `gorm:"comment:平台:1:企业微信;column:platform" json:"platform"`
    Classify           int    `gorm:"comment:分类:1:群活码,2:客户活码,3:渠道活码;column:classify" json:"classify"`
    SceneLink          string `gorm:"comment:场景落地页;column:scene_link" json:"scene_link"`
    SafeThresholdValue int    `gorm:"comment:安全阈值/渠道码生效;column:safe_threshold_value" json:"safe_threshold_value"`
    ExpiryDate         int64  `gorm:"comment:有效期截止日;column:expiry_date" json:"expiry_date"`
    IsAutoActive       bool   `gorm:"comment:是否自动打开二维码/保留字段;column:is_auth_active" json:"is_auth_active"`
    Cpa                int    `gorm:"comment:打开次数;column:cpa" json:"cpa"`

    //
    ActiveQrcodeLink string `gorm:"comment:活码图,方便后续嵌入媒资文章;column:active_qrcode_link" json:"active_qrcode_link"`
    State            int    `gorm:"comment:状态1:启用 2:禁用 3:删除;column:state" json:"state"`
}

//
// Table
//  @Description:
//  @receiver e
//  @return string
//
func (e SceneQrcode) TableName() string {
    return `scene_qrcodes`
}

//
// Query
//  @Description:
//  @receiver this
//  @param db
//  @return groups
//  @return err
//
func (e *SceneQrcode) Query(db *gorm.DB) (qrcode []*SceneQrcode) {

    err := db.Model(e).Find(&qrcode).Error
    if err != nil {
        panic(err)
    }
    return qrcode

}

//
// Action
//  @Description:
//  @receiver this
//  @param db
//  @param group
//  @return []*WeWorkAppGroup
//
func (e *SceneQrcode) Action(db *gorm.DB, qrcode []*SceneQrcode) {

    err := db.Table(e.TableName()).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "qid"}}, UpdateAll: true}).Create(&qrcode).Error
    if err != nil {
        panic(err)
    }

}

//
// UpdateColumn
//  @Description:
//  @receiver e
//  @param db
//  @param qid
//  @param value
//
func (e *SceneQrcode) UpdateColumn(db *gorm.DB, qid string, value map[string]interface{}) {

    err := db.Table(e.TableName()).Where(`qid = ?`, qid).UpdateColumns(&value).Error
    if err != nil {
        panic(err)
    }

}

//
// FindByQid
//  @Description:
//  @receiver e
//  @param db
//  @param qid
//  @return qrcode
//
func (e *SceneQrcode) FindByQid(db *gorm.DB, qid string) (qrcode *SceneQrcode) {

    err := db.Table(e.TableName()).Debug().Where(`qid = ? AND state < 3`, qid).Find(&qrcode).Error
    if err != nil {
        panic(err)
    }
    return qrcode
}

//
// FindEnableSceneQrcodeByQid
//  @Description:
//  @receiver e
//  @param db
//  @param qid
//  @return qrcode
//
func (e *SceneQrcode) FindEnableSceneQrcodeByQid(db *gorm.DB, qid string) (qrcode *SceneQrcode) {

    err := db.Table(e.TableName()).Debug().Where(`qid = ? AND state = 1`, qid).Find(&qrcode).Error
    if err != nil {
        panic(err)
    }
    return qrcode
}

//
// IncreaseCpa
//  @Description:
//  @receiver e
//  @param db
//  @param qid
//
func (e *SceneQrcode) IncreaseCpa(db *gorm.DB, qid string) {

    err := db.Table(e.TableName()).Where(`qid = ?`, qid).Update(`cpa`, gorm.Expr(`cpa + ?`, 1)).Error
    if err != nil {
        panic(err)
    }

}
