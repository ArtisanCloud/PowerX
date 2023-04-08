package product

import (
	"PowerX/internal/model"
	"PowerX/internal/model/powermodel"
)

type ProductCategory struct {
	powermodel.PowerCompactModel

	PId         int64
	Name        string
	Sort        int8
	viceName    string
	Description string
	model.ImageAbleInfo
}

const ProductCategoryUniqueId = powermodel.CompactUniqueId
