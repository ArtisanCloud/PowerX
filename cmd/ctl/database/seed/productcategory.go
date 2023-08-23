package seed

import (
	"PowerX/internal/model"
	"PowerX/internal/model/product"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

func CreateProductCategories(db *gorm.DB) (err error) {

	var count int64
	if err = db.Model(&product.ProductCategory{}).Count(&count).Error; err != nil {
		panic(errors.Wrap(err, "init product category  failed"))
	}

	data := DefaultProductCategory()
	if count == 0 {
		for _, item := range data {
			if err = db.Model(&product.ProductCategory{}).Create(item).Error; err != nil {
				panic(errors.Wrap(err, "init product category failed"))
			}
		}
	}
	return err
}

func DefaultProductCategory() (data []*product.ProductCategory) {

	data = []*product.ProductCategory{
		{
			Name:         "运动上衣",
			Sort:         0,
			ViceName:     "",
			Description:  "",
			CoverImageId: 2,
			Children: []*product.ProductCategory{
				{Name: "短袖T恤", Sort: 0, CoverImageId: 3},
				{Name: "长袖T恤", Sort: 0, CoverImageId: 3},
				{Name: "运动背心", Sort: 0, CoverImageId: 3},
				{Name: "运动长袖衬衫", Sort: 0, CoverImageId: 3},
				{Name: "运动外套", Sort: 0, CoverImageId: 3},
				{Name: "运动夹克", Sort: 0, CoverImageId: 3},
			},
		},

		{

			Name:        "运动裤子",
			Sort:        0,
			ViceName:    "",
			Description: "",
			Children: []*product.ProductCategory{
				{Name: "运动长裤", Sort: 0, CoverImageId: 4},
				{Name: "运动短裤", Sort: 0, CoverImageId: 4},
				{Name: "运动紧身裤", Sort: 0, CoverImageId: 4},
				{Name: "运动运动裤", Sort: 0, CoverImageId: 4},
				{Name: "运动牛仔裤", Sort: 0, CoverImageId: 4},
			},
		},
		{
			Name:         "运动裙子",
			Sort:         0,
			ViceName:     "",
			Description:  "",
			CoverImageId: 6,
			Children: []*product.ProductCategory{
				{Name: "运动长裤", Sort: 0, CoverImageId: 5},
				{Name: "运动短裤", Sort: 0, CoverImageId: 5},
				{Name: "运动紧身裤", Sort: 0, CoverImageId: 5},
				{Name: "运动运动裤", Sort: 0, CoverImageId: 5},
				{Name: "运动牛仔裤", Sort: 0, CoverImageId: 5},
			},
		},
	}

	return data
}

func DefaultThreeLevelProductCategory() (data []*product.ProductCategory) {

	data = []*product.ProductCategory{
		{
			Name:         "运动上装",
			Sort:         0,
			ViceName:     "",
			Description:  "",
			CoverImageId: 1,
			ImageAbleInfo: model.ImageAbleInfo{
				Icon:            "icon-person",
				BackgroundColor: "#EEEEEE",
			},
			Children: []*product.ProductCategory{
				{
					Name:         "运动上衣",
					Sort:         0,
					ViceName:     "",
					Description:  "",
					CoverImageId: 2,
					Children: []*product.ProductCategory{
						{Name: "短袖T恤", Sort: 0, CoverImageId: 3},
						{Name: "长袖T恤", Sort: 0, CoverImageId: 3},
						{Name: "运动背心", Sort: 0, CoverImageId: 3},
						{Name: "运动长袖衬衫", Sort: 0, CoverImageId: 3},
						{Name: "运动外套", Sort: 0, CoverImageId: 3},
						{Name: "运动夹克", Sort: 0, CoverImageId: 3},
					},
				},
			},
		},
		{
			Name:         "运动下装",
			Sort:         0,
			ViceName:     "",
			Description:  "",
			CoverImageId: 7,
			Children: []*product.ProductCategory{
				{
					Name:        "运动裤子",
					Sort:        0,
					ViceName:    "",
					Description: "",
					Children: []*product.ProductCategory{
						{Name: "运动长裤", Sort: 0, CoverImageId: 4},
						{Name: "运动短裤", Sort: 0, CoverImageId: 4},
						{Name: "运动紧身裤", Sort: 0, CoverImageId: 4},
						{Name: "运动运动裤", Sort: 0, CoverImageId: 4},
						{Name: "运动牛仔裤", Sort: 0, CoverImageId: 4},
					},
				},
				{
					Name:         "运动裙子",
					Sort:         0,
					ViceName:     "",
					Description:  "",
					CoverImageId: 6,
					Children: []*product.ProductCategory{
						{Name: "运动长裤", Sort: 0, CoverImageId: 5},
						{Name: "运动短裤", Sort: 0, CoverImageId: 5},
						{Name: "运动紧身裤", Sort: 0, CoverImageId: 5},
						{Name: "运动运动裤", Sort: 0, CoverImageId: 5},
						{Name: "运动牛仔裤", Sort: 0, CoverImageId: 5},
					},
				},
			},
		},
	}
	return data
}
