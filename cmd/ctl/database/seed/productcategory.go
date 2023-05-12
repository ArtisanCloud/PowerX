package seed

import (
	"PowerX/internal/model/product"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

func CreateProductCategories(db *gorm.DB) (err error) {

	var count int64
	if err = db.Model(&product.ProductCategory{}).Count(&count).Error; err != nil {
		panic(errors.Wrap(err, "init price category  failed"))
	}

	data := DefaultProductCategory()
	if count == 0 {
		for _, item := range data {
			if err = db.Model(&product.ProductCategory{}).Create(item).Error; err != nil {
				panic(errors.Wrap(err, "init price category failed"))
			}
		}
	}
	return err
}

func DefaultProductCategory() (data []*product.ProductCategory) {

	data = []*product.ProductCategory{
		&product.ProductCategory{
			Name:        "女装",
			Sort:        0,
			ViceName:    "",
			Description: "",
			Children: []*product.ProductCategory{
				&product.ProductCategory{
					Name:        "上装",
					Sort:        0,
					ViceName:    "",
					Description: "",
					Children: []*product.ProductCategory{
						&product.ProductCategory{
							Name:        "T恤",
							Sort:        0,
							ViceName:    "",
							Description: "",
						},
						&product.ProductCategory{
							Name:        "衬衫",
							Sort:        1,
							ViceName:    "",
							Description: "",
						},
						&product.ProductCategory{
							Name:        "针织衫",
							Sort:        2,
							ViceName:    "",
							Description: "",
						},
						&product.ProductCategory{
							Name:        "卫衣",
							Sort:        3,
							ViceName:    "",
							Description: "",
						},
						&product.ProductCategory{
							Name:        "毛衣",
							Sort:        4,
							ViceName:    "",
							Description: "",
						},
						&product.ProductCategory{
							Name:        "蕾丝衫",
							Sort:        5,
							ViceName:    "",
							Description: "",
						},
						&product.ProductCategory{
							Name:        "套头衫",
							Sort:        6,
							ViceName:    "",
							Description: "",
						},
					},
				},
				&product.ProductCategory{
					Name:        "下装",
					Sort:        1,
					ViceName:    "",
					Description: "",
					Children: []*product.ProductCategory{
						&product.ProductCategory{
							Name:        "牛仔裤",
							Sort:        0,
							ViceName:    "",
							Description: "",
						},
						&product.ProductCategory{
							Name:        "运动裤",
							Sort:        1,
							ViceName:    "",
							Description: "",
						},
						&product.ProductCategory{
							Name:        "西装裤",
							Sort:        2,
							ViceName:    "",
							Description: "",
						},
						&product.ProductCategory{
							Name:        "短裤",
							Sort:        3,
							ViceName:    "",
							Description: "",
						},
						&product.ProductCategory{
							Name:        "裙子",
							Sort:        4,
							ViceName:    "",
							Description: "",
						},
					},
				},
				&product.ProductCategory{
					Name:        "裙装",
					Sort:        2,
					ViceName:    "",
					Description: "",
					Children: []*product.ProductCategory{
						&product.ProductCategory{
							Name:        "连衣裙",
							Sort:        0,
							ViceName:    "",
							Description: "",
						},
						&product.ProductCategory{
							Name:        "半身裙",
							Sort:        1,
							ViceName:    "",
							Description: "",
						},
						&product.ProductCategory{
							Name:        "吊带裙",
							Sort:        2,
							ViceName:    "",
							Description: "",
						},
						&product.ProductCategory{
							Name:        "长裙",
							Sort:        3,
							ViceName:    "",
							Description: "",
						},
					},
				},
				&product.ProductCategory{
					Name:        "内衣",
					Sort:        3,
					ViceName:    "",
					Description: "",
					Children: []*product.ProductCategory{
						&product.ProductCategory{
							Name:        "文胸",
							Sort:        0,
							ViceName:    "",
							Description: "",
						},
						&product.ProductCategory{
							Name:        "内裤",
							Sort:        1,
							ViceName:    "",
							Description: "",
						},
						&product.ProductCategory{
							Name:        "塑身内衣",
							Sort:        2,
							ViceName:    "",
							Description: "",
						},
						&product.ProductCategory{
							Name:        "睡衣",
							Sort:        3,
							ViceName:    "",
							Description: "",
						},
						&product.ProductCategory{
							Name:        "袜子",
							Sort:        4,
							ViceName:    "",
							Description: "",
						},
					},
				},
			},
		},
		&product.ProductCategory{
			Name:        "男装",
			Sort:        1,
			ViceName:    "",
			Description: "",
			Children: []*product.ProductCategory{
				&product.ProductCategory{
					Name:        "上装",
					Sort:        0,
					ViceName:    "",
					Description: "",
					Children: []*product.ProductCategory{
						&product.ProductCategory{
							Name:        "T恤",
							Sort:        0,
							ViceName:    "",
							Description: "",
						},
						&product.ProductCategory{
							Name:        "衬衫",
							Sort:        1,
							ViceName:    "",
							Description: "",
						},
						&product.ProductCategory{
							Name:        "针织衫",
							Sort:        2,
							ViceName:    "",
							Description: "",
						},
						&product.ProductCategory{
							Name:        "卫衣",
							Sort:        3,
							ViceName:    "",
							Description: "",
						},
						&product.ProductCategory{
							Name:        "毛衣",
							Sort:        4,
							ViceName:    "",
							Description: "",
							Children: []*product.ProductCategory{
								&product.ProductCategory{
									Name:        "针织毛衣",
									Sort:        0,
									ViceName:    "",
									Description: "",
								},
								&product.ProductCategory{
									Name:        "羊毛衫",
									Sort:        1,
									ViceName:    "",
									Description: "",
								},
								&product.ProductCategory{
									Name:        "卫衣",
									Sort:        2,
									ViceName:    "",
									Description: "",
								},
							},
						},
						&product.ProductCategory{
							Name:        "裙子",
							Sort:        5,
							ViceName:    "",
							Description: "",
							Children: []*product.ProductCategory{
								&product.ProductCategory{
									Name:        "连衣裙",
									Sort:        0,
									ViceName:    "",
									Description: "",
								},
								&product.ProductCategory{
									Name:        "半身裙",
									Sort:        1,
									ViceName:    "",
									Description: "",
								},
							},
						},
						&product.ProductCategory{
							Name:        "鞋子",
							Sort:        6,
							ViceName:    "",
							Description: "",
							Children: []*product.ProductCategory{
								&product.ProductCategory{
									Name:        "运动鞋",
									Sort:        0,
									ViceName:    "",
									Description: "",
								},
								&product.ProductCategory{
									Name:        "休闲鞋",
									Sort:        1,
									ViceName:    "",
									Description: "",
								},
								&product.ProductCategory{
									Name:        "高跟鞋",
									Sort:        2,
									ViceName:    "",
									Description: "",
								},
								&product.ProductCategory{
									Name:        "靴子",
									Sort:        3,
									ViceName:    "",
									Description: "",
								},
							},
						},
					},
				},
			},
		},
	}
	return data
}
