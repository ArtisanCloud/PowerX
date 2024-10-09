package customer

import (
	"PowerX/internal/model"
	"PowerX/internal/model/powermodel"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type WeWorkExternalContactFollow struct {
	powermodel.PowerModel

	ExternalUserId string `gorm:"comment:客户ID;column:external_user_id;unique" json:"external_user_id"`
	UserId         string `gorm:"comment:员工ID;column:user_id" json:"user_id"`
	Remark         string `gorm:"comment:备注;column:remark" json:"remark"`
	Description    string `gorm:"comment:描述;column:description" json:"description"`
	CreatedTime    int    `gorm:"comment:创建时间;column:create_time" json:"create_time"`
	Tags           string `gorm:"comment:TAG;column:tags" json:"tags"`
	TagIds         string `gorm:"comment:企业标签;column:tag_ids" json:"tag_ids"`
	WechatChannels string `gorm:"comment:Channels;column:wechat_channels" json:"wechat_channels"`
	RemarkCorpName string `gorm:"comment:企业备注;column:remark_corp_name" json:"remark_corp_name"`
	RemarkMobiles  string `gorm:"comment:电话备注;column:remark_mobiles" json:"remark_mobiles"`
	OpenUserId     string `gorm:"comment:开放ID;column:open_user_id" json:"open_user_id"`
	AddWay         int    `gorm:"comment:AddWay;column:add_way" json:"add_way"`
	State          string `gorm:"comment:State;column:state" json:"state"`
}

func (mdl *WeWorkExternalContactFollow) TableName() string {
	return model.PowerXSchema + "." + model.TableNameWeWorkExternalContactFollow
}

func (mdl *WeWorkExternalContactFollow) GetTableName(needFull bool) string {
	tableName := model.TableNameWeWorkExternalContactFollow
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
//	@return follows
func (e WeWorkExternalContactFollow) Query(db *gorm.DB) (follows []*WeWorkExternalContactFollow) {

	err := db.Model(e).Find(&follows).Error
	if err != nil {
		panic(err)
	}
	return follows

}

// Action
//
//	@Description:
//	@receiver e
//	@param db
//	@param contacts
func (e *WeWorkExternalContactFollow) Action(db *gorm.DB, contacts []*WeWorkExternalContactFollow) {

	err := db.Table(e.TableName()).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "external_user_id"}}, UpdateAll: true}).CreateInBatches(&contacts, 100).Error
	if err != nil {
		panic(err)
	}

}

// FindFollowByExternalUserId
//
//	@Description:
//	@receiver e
//	@param db
//	@param externalUserIds
//	@return follows
func (e WeWorkExternalContactFollow) FindFollowByExternalUserId(db *gorm.DB, externalUserId string) (follow *WeWorkExternalContactFollow) {

	err := db.Model(e).Where(`external_user_id = ?`, externalUserId).Find(&follow).Error
	if err != nil {
		panic(err)
	}
	return follow

}
