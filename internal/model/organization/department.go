package organization

import (
	"PowerX/internal/model"
	"PowerX/internal/model/powermodel"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Department struct {
	powermodel.PowerModel

	PDep        *Department   `gorm:"foreignKey:PId"`
	Leader      *User         `gorm:"foreignKey:LeaderUuid"`
	Ancestors   []*Department `gorm:"many2many:department_ancestors;"`
	Name        string        `gorm:"comment:部门名称;column:name" json:"name"`
	PId         int64         `gorm:"comment:部门名ID;column:pid" json:"pid"`
	LeaderUuid  *string       `gorm:"comment:领导ID;column:leader_id;type:uuid;default:null" json:"leader_id"`
	Desc        string        `gorm:"comment:描述;column:desc" json:"desc"`
	PhoneNumber string        `gorm:"comment:部门电话;column:phone_number" json:"phone_number"`
	Email       string        `gorm:"comment:部门邮箱;column:email" json:"email"`
	Remark      string        `gorm:"comment:备注;column:remark" json:"remark"`
	IsReserved  bool          `gorm:"comment:保留;column:is_reserved" json:"is_reserved"`
	//
	IsWeWorkArchitecture bool `gorm:"comment:是否启用企微架构;column:is_we_work_architecture" json:"is_we_work_architecture"`
}

func (mdl *Department) TableName() string {
	return model.PowerXSchema + "." + model.TableNameDepartment
}

func (mdl *Department) GetTableName(needFull bool) string {
	tableName := model.TableNameDepartment
	if needFull {
		tableName = mdl.TableName()
	}
	return tableName
}

func (mdl *Department) Action(db *gorm.DB, departments []*Department) {

	err := db.Table(mdl.TableName()).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "we_work_user_id"}}, UpdateAll: true}).CreateInBatches(&departments, 100).Error
	if err != nil {
		panic(err)
	}

}
