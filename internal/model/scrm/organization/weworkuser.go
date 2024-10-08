package organization

import (
	"PowerX/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type WeWorkUser struct {
	model.Model

	WeWorkUserId           string `gorm:"comment:员工ID;column:we_work_user_id;unique" json:"we_work_user_id"`
	Name                   string `gorm:"comment:员工名称;column:name" json:"name"`
	Position               string `gorm:"comment:员工位置;column:position" json:"position"`
	Mobile                 string `gorm:"comment:员工电话;column:mobile" json:"mobile"`
	Gender                 string `gorm:"comment:员工性别;column:gender" json:"gender"`
	Email                  string `gorm:"comment:邮箱;column:email" json:"email"`
	BizMail                string `gorm:"comment:商务邮箱;column:biz_mail" json:"biz_mail"`
	Avatar                 string `gorm:"comment:头像;column:avatar" json:"avatar"`
	ThumbAvatar            string `gorm:"comment:ThumbAvatar;column:thumb_avatar" json:"thumb_avatar"`
	Telephone              string `gorm:"comment:电话;column:telephone" json:"telephone"`
	Alias                  string `gorm:"comment:别称;column:alias" json:"alias"`
	Address                string `gorm:"comment:地址;column:address" json:"address"`
	OpenUserId             string `gorm:"comment:开放ID;column:open_user_id" json:"open_user_id"`
	WeWorkMainDepartmentId int    `gorm:"comment:部门ID;column:we_work_main_department_id" json:"we_work_main_department_id"`
	Status                 int    `gorm:"comment:状态;column:status" json:"status"`
	QrCode                 string `gorm:"comment:二维码;column:qr_code" json:"qr_code"`
	Department             string `gorm:"comment:部门;column:department" json:"department"`
	RefUserId              int64  `gorm:"comment:RefUserId;column:ref_user_id" json:"ref_user_id"`
}

// Table
//
//	@Description:
//	@receiver e
//	@return string
func (e WeWorkUser) TableName() string {
	return model.TableNameWeWorkUser
}

type (
	AdapterUserSliceUserIDs func(user []*WeWorkUser) (ids []string)
)

// Query
//
//	@Description:
//	@receiver this
//	@param db
//	@return users
func (e WeWorkUser) Query(db *gorm.DB) (users []*WeWorkUser) {

	err := db.Model(e).Find(&users).Error
	if err != nil {
		panic(err)
	}
	return users

}

// Action
//
//	@Description:
//	@receiver e
//	@param db
//	@param users
func (e WeWorkUser) Action(db *gorm.DB, users []*WeWorkUser) {

	err := db.Table(e.TableName()).
		//Debug().
		Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "we_work_user_id"}}, UpdateAll: true}).CreateInBatches(&users, 100).Error
	if err != nil {
		panic(err)
	}

}
