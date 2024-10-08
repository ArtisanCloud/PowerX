package customer

import (
	"PowerX/internal/model"
	"PowerX/internal/model/powermodel"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type WeWorkExternalContact struct {
	powermodel.PowerModel

	WeWorkExternalContactFollow WeWorkExternalContactFollow `gorm:"foreignKey:ExternalUserId;references:external_user_id" json:"WeWorkExternalContactFollow"`
	ExternalUserId              string                      `gorm:"comment:客户ID;unique;not null;" json:"externalUserId"`
	AppId                       string                      `gorm:"comment:应用ID;index:idx_app_id;column:app_id" json:"appId"`
	CorpId                      string                      `gorm:"comment:企业ID;index:idx_corp_id;column:corp_id" json:"corpId"`
	OpenId                      string                      `gorm:"comment:开放ID;index:idx_customer_open_id;column:open_id;" json:"openId"`
	UnionId                     string                      `gorm:"comment:微信唯一ID;index:idx_union_id;column:union_id" json:"unionId"`

	UserId          string `gorm:"comment:员工ID;column:user_id" json:"user_id"`
	Name            string `gorm:"comment:客户名称;column:name" json:"name"`
	Mobile          string `gorm:"comment:客户手机号;column:mobile" json:"mobile"`
	Position        string `gorm:"comment:客户位置;column:position" json:"position"`
	Avatar          string `gorm:"comment:客户头像;column:avatar" json:"avatar"`
	CorpName        string `gorm:"comment:企业名称;column:corp_name" json:"corpName"`
	CorpFullName    string `gorm:"comment:企业全称;column:corp_full_name" json:"corpFullName"`
	ExternalProfile string `gorm:"comment:基础信息;column:external_profile" json:"externalProfile"`
	Gender          int    `gorm:"comment:性别;column:gender" json:"gender"`
	WXType          int8   `gorm:"comment:类型;column:wx_type" json:"wxType"`
	Status          int    `gorm:"active" json:"status"`
	Active          bool   `gorm:"active" json:"active"`
}

func (mdl *WeWorkExternalContact) TableName() string {
	return model.PowerXSchema + "." + model.TableNameWeWorkExternalContact
}

func (mdl *WeWorkExternalContact) GetTableName(needFull bool) string {
	tableName := model.TableNameWeWorkExternalContact
	if needFull {
		tableName = mdl.TableName()
	}
	return tableName
}

// Query
//
//	@Description:
//	@receiver e
//	@param db
//	@return contacts
func (mdl *WeWorkExternalContact) Query(db *gorm.DB) (contacts []*WeWorkExternalContact) {

	err := db.Model(mdl).Find(&contacts).Error
	if err != nil {
		panic(err)
	}
	return contacts

}

// Action
//
//	@Description:
//	@receiver e
//	@param db
//	@param contacts
func (mdl *WeWorkExternalContact) Action(db *gorm.DB, contacts []*WeWorkExternalContact) {

	err := db.Table(mdl.TableName()).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "external_user_id"}}, UpdateAll: true}).CreateInBatches(&contacts, 100).Error
	if err != nil {
		panic(err)
	}

}
