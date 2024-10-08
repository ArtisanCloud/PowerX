package tag

import (
	"PowerX/internal/model"
	"PowerX/internal/model/media"
	"PowerX/internal/model/powermodel"
	"gorm.io/gorm"
)

type Tag struct {
	powermodel.PowerModel

	CoverImage *media.MediaResource `gorm:"foreignKey:CoverImageId;references:Id" json:"coverImage"`
	Parent     *Tag                 `gorm:"foreignKey:PId;references:Id" json:"parent"`
	Children   []*Tag               `gorm:"foreignKey:PId;references:Id" json:"children"`

	PId          int64  `gorm:"comment:上级品类" json:"pId"`
	Name         string `gorm:"comment:品类名称" json:"name"`
	Sort         int    `gorm:"comment:排序" json:"sort"`
	ViceName     string `gorm:"comment:副标题" json:"viceName"`
	Description  string `gorm:"comment:描述" json:"description"`
	CoverImageId int64  `gorm:"comment:封面图Id" json:"coverImageId"`

	model.ImageAbleInfo
}

const TagUniqueId = powermodel.UniqueId

func (mdl *Tag) TableName() string {
	return model.PowerXSchema + "." + model.TableNameTag
}

func (mdl *Tag) GetTableName(needFull bool) string {
	tableName := model.TableNameTag
	if needFull {
		tableName = mdl.TableName()
	}
	return tableName
}
func GetTagIds(tags []*Tag) []int64 {
	uniqueIds := make(map[int64]bool)
	arrayIds := []int64{}
	if len(tags) <= 0 {
		return arrayIds
	}
	for _, tag := range tags {
		if !uniqueIds[tag.Id] {
			arrayIds = append(arrayIds, tag.Id)
			uniqueIds[tag.Id] = true
		}
	}
	return arrayIds
}

func (mdl *Tag) LoadChildren(db *gorm.DB, conditions *map[string]interface{}, withClauseAssociations bool) error {

	mdl.Children = []*Tag{}
	err := powermodel.AssociationRelationship(db, conditions, mdl, "Children", false).Find(&mdl.Children)
	//fmt.Dump(mdl.Artisans)
	return err
}
