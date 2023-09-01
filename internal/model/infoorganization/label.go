package infoorganizatoin

import (
	"PowerX/internal/model"
	"PowerX/internal/model/media"
	"PowerX/internal/model/powermodel"
	"gorm.io/gorm"
)

type Label struct {
	powermodel.PowerModel

	CoverImage *media.MediaResource `gorm:"foreignKey:CoverImageId;references:Id" json:"coverImage"`
	Parent     *Label               `gorm:"foreignKey:PId;references:Id" json:"parent"`
	Children   []*Label             `gorm:"foreignKey:PId;references:Id" json:"children"`

	PId          int64  `gorm:"comment:上级框架标签" json:"pId"`
	Name         string `gorm:"comment:框架标签名称" json:"name"`
	Sort         int    `gorm:"comment:排序" json:"sort"`
	ViceName     string `gorm:"comment:副标题" json:"viceName"`
	Description  string `gorm:"comment:描述" json:"description"`
	CoverImageId int64  `gorm:"comment:封面图Id" json:"coverImageId"`

	model.ImageAbleInfo
}

const LabelUniqueId = powermodel.UniqueId

func GetLabelIds(categories []*Label) []int64 {
	uniqueIds := make(map[int64]bool)
	arrayIds := []int64{}
	if len(categories) <= 0 {
		return arrayIds
	}
	for _, label := range categories {
		if !uniqueIds[label.Id] {
			arrayIds = append(arrayIds, label.Id)
			uniqueIds[label.Id] = true
		}
	}
	return arrayIds
}

func (mdl *Label) LoadChildren(db *gorm.DB, conditions *map[string]interface{}, withClauseAssociations bool) error {

	mdl.Children = []*Label{}
	err := powermodel.AssociationRelationship(db, conditions, mdl, "Children", false).Find(&mdl.Children)
	//fmt.Dump(mdl.Artisans)
	return err
}
