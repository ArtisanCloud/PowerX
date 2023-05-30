package product

import (
	"PowerX/internal/model"
	"PowerX/internal/model/powermodel"
)

type ProductCategory struct {
	powermodel.PowerModel

	Parent   *ProductCategory   `gorm:"foreignKey:PId;references:Id" json:"parent"`
	Children []*ProductCategory `gorm:"foreignKey:PId;references:Id" json:"children"`

	PId         int64  `gorm:"comment:上级品类"`
	Name        string `gorm:"comment:品类名称"`
	Sort        int    `gorm:"comment:排序"`
	ViceName    string `gorm:"comment:副标题"`
	Description string `gorm:"comment:描述"`

	model.ImageAbleInfo
}

const ProductCategoryUniqueId = powermodel.UniqueId

func GetCategoryIds(categories []*ProductCategory) []int64 {
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
