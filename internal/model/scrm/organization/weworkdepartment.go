package organization

import (
	"PowerX/internal/model"
	"PowerX/internal/model/powermodel"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type WeWorkDepartment struct {
	powermodel.PowerModel

	// Leader         *WeWorkUser `gorm:"foreignKey:LeaderId"`
	WeWorkDepId      int    `gorm:"comment:部门ID;column:we_work_dep_id;unique" json:"we_work_dep_id"`
	Name             string `gorm:"comment:部门名称;column:name" json:"name"`
	NameEn           string `gorm:"comment:部门英文名称;column:name_en" json:"name_en"`
	WeWorkParentId   int    `gorm:"comment:上级部门ID;column:we_work_parent_id" json:"we_work_parent_id"`
	Order            int    `gorm:"comment:Order;column:order" json:"order"`
	DepartmentLeader string `gorm:"comment:部门Leader;column:department_leader" json:"department_leader"`
	RefDepartmentId  int64  `gorm:"comment:-;column:ref_department_id" json:"ref_department_id"`
}

func (mdl *WeWorkDepartment) TableName() string {
	return model.PowerXSchema + "." + model.TableNameWeWorkDepartment
}

func (mdl *WeWorkDepartment) GetTableName(needFull bool) string {
	tableName := model.TableNameWeWorkDepartment
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
//	@return departments
func (e WeWorkDepartment) Query(db *gorm.DB) (departments []*WeWorkDepartment) {

	err := db.Model(e).Find(&departments).Error
	if err != nil {
		panic(err)
	}
	return departments

}

// Action
//
//	@Description:
//	@receiver e
//	@param db
//	@param contacts
func (e *WeWorkDepartment) Action(db *gorm.DB, contacts []*WeWorkDepartment) {

	err := db.Table(e.TableName()).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "we_work_dep_id"}}, UpdateAll: true}).CreateInBatches(&contacts, 100).Error
	if err != nil {
		panic(err)
	}

}
