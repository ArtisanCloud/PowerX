package infoorganizatoin

import (
	"PowerX/internal/model"
	"PowerX/internal/model/media"
	"PowerX/internal/model/powermodel"
	"gorm.io/gorm"
)

type Category struct {
	powermodel.PowerModel

	CoverImage *media.MediaResource `gorm:"foreignKey:CoverImageId;references:Id" json:"coverImage"`
	Parent     *Category            `gorm:"foreignKey:PId;references:Id" json:"parent"`
	Children   []*Category          `gorm:"foreignKey:PId;references:Id" json:"children"`

	PId          int64  `gorm:"comment:上级品类" json:"pId"`
	Name         string `gorm:"comment:品类名称" json:"name"`
	Sort         int    `gorm:"comment:排序" json:"sort"`
	ViceName     string `gorm:"comment:副标题" json:"viceName"`
	Description  string `gorm:"comment:描述" json:"description"`
	CoverImageId int64  `gorm:"comment:封面图Id" json:"coverImageId"`

	model.ImageAbleInfo
}

const CategoryUniqueId = powermodel.UniqueId

func (mdl *Category) TableName() string {
	return model.PowerXSchema + "." + model.TableNameCategory
}

func (mdl *Category) GetTableName(needFull bool) string {
	tableName := model.TableNameCategory
	if needFull {
		tableName = mdl.TableName()
	}
	return tableName
}

func (mdl *Category) GetCategoryIds(categories []*Category) []int64 {
	uniqueIds := make(map[int64]bool)
	arrayIds := []int64{}
	if len(categories) <= 0 {
		return arrayIds
	}
	for _, category := range categories {
		if !uniqueIds[category.Id] {
			arrayIds = append(arrayIds, category.Id)
			uniqueIds[category.Id] = true
		}
	}
	return arrayIds
}

func (mdl *Category) LoadChildren(db *gorm.DB, conditions *map[string]interface{}, withClauseAssociations bool) error {

	mdl.Children = []*Category{}
	err := powermodel.AssociationRelationship(db, conditions, mdl, "Children", false).Find(&mdl.Children)
	//fmt.Dump(mdl.Artisans)
	return err
}
