package tag

import (
	"PowerX/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type WeWorkTagGroup struct {
	model.Model
	WeWorkGroupTags []*WeWorkTag `gorm:"foreignKey:GroupId;references:group_id" json:"WeWorkGroupTags"`
	AgentId         int          `gorm:"comment:应用ID;column:agent_id" json:"agent_id"`
	GroupId         string       `gorm:"comment:标签组ID;column:group_id;unique" json:"group_id"`
	Name            string       `gorm:"comment:标签组名称;column:name" json:"name"`
	Sort            int          `gorm:"comment:排序;column:sort" json:"sort"`
	IsDelete        bool         `gorm:"comment:是否删除;column:is_delete" json:"is_delete"`
}

// Table
//
//	@Description:
//	@receiver e
//	@return string
func (e WeWorkTagGroup) TableName() string {
	return model.TableNameWeWorkTagGroup
}

// Query
//
//	@Description:
//	@receiver this
//	@param db
//	@return groups
//	@return err
func (e *WeWorkTagGroup) Query(db *gorm.DB) (groups []*WeWorkTagGroup) {

	err := db.Model(e).Where(`is_delete = ?`, false).Preload(`WeWorkGroupTags`).Order(`sort ASC`).Find(&groups).Error
	if err != nil {
		panic(err)
	}
	return groups

}

// Action
//
//	@Description:
//	@receiver this
//	@param db
//	@param group
//	@return []*WeWorkAppGroup
func (e *WeWorkTagGroup) Action(db *gorm.DB, groups []*WeWorkTagGroup) {

	err := db.Table(e.TableName()).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "group_id"}}, UpdateAll: true}).Create(&groups).Error
	if err != nil {
		panic(err)
	}

}
