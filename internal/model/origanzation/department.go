package origanzation

import (
	"PowerX/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Department struct {
	model.Model
	PDep        *Department   `gorm:"foreignKey:PId"`
	Leader      *User         `gorm:"foreignKey:LeaderId"`
	Ancestors   []*Department `gorm:"many2many:department_ancestors;"`
	Name        string        `gorm:"comment:部门名称;column:name" json:"name"`
	PId         int64         `gorm:"comment:部门名ID;column:pid" json:"pid"`
	LeaderId    int64         `gorm:"comment:领导ID;column:leader_id" json:"leader_id"`
	Desc        string        `gorm:"comment:描述;column:desc" json:"desc"`
	PhoneNumber string        `gorm:"comment:部门电话;column:phone_number" json:"phone_number"`
	Email       string        `gorm:"comment:部门邮箱;column:email" json:"email"`
	Remark      string        `gorm:"comment:备注;column:remark" json:"remark"`
	IsReserved  bool          `gorm:"comment:保留;column:is_reserved" json:"is_reserved"`
	//
	IsWeWorkArchitecture bool `gorm:"comment:是否启用企微架构;column:is_we_work_architecture" json:"is_we_work_architecture"`
}

func (e *Department) TableName() string {
	return `departments`
}

func (e *Department) Action(db *gorm.DB, departments []*Department) {

	err := db.Table(e.TableName()).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "we_work_user_id"}}, UpdateAll: true}).CreateInBatches(&departments, 100).Error
	if err != nil {
		panic(err)
	}

}
