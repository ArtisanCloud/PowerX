package seed

import (
	"PowerX/internal/model/custom"
	"PowerX/internal/model/product"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

func CreateStore(db *gorm.DB) (err error) {
	var count int64
	if err = db.Model(&product.Store{}).Count(&count).Error; err != nil {
		panic(errors.Wrap(err, "init root dep failed"))
	}
	if count == 0 {
		stores := DefaultStore()
		if err := db.Model(&product.Store{}).Create(&stores).Error; err != nil {
			panic(errors.Wrap(err, "init root dep failed"))
		}
	}

	return err
}

func DefaultStore() []*product.Store {
	return []*product.Store{
		&product.Store{
			Artisans: []*product.Artisan{
				&product.Artisan{
					ArtisanSpecific: &custom.ArtisanSpecific{
						MaxServiceDuration: 60,
						MandatoryDuration:  10,
					},
					Name:        "发型师A",
					PhoneNumber: "13564674240",
				},
				&product.Artisan{
					Name:        "发型师B",
					PhoneNumber: "13564674241",
					ArtisanSpecific: &custom.ArtisanSpecific{
						MaxServiceDuration: 40,
						MandatoryDuration:  10,
					},
				},
				&product.Artisan{
					Name:        "发型师C",
					PhoneNumber: "13564674242",
					ArtisanSpecific: &custom.ArtisanSpecific{
						MaxServiceDuration: 30,
						MandatoryDuration:  10,
					},
				},
			},
			Name:          "816 * Salon (大华店)",
			Address:       "宝山区 大华二路273号816salon（TB美甲美睫二楼）",
			ContactNumber: "18616325540",
		},
		&product.Store{
			Artisans: []*product.Artisan{
				&product.Artisan{
					ArtisanSpecific: &custom.ArtisanSpecific{
						MaxServiceDuration: 60,
						MandatoryDuration:  10,
					},
					Name:        "发型师D",
					PhoneNumber: "13564674243",
				},
				&product.Artisan{
					ArtisanSpecific: &custom.ArtisanSpecific{
						MaxServiceDuration: 40,
						MandatoryDuration:  10,
					},
					Name:        "发型师E",
					PhoneNumber: "13564674244",
				},
				&product.Artisan{
					ArtisanSpecific: &custom.ArtisanSpecific{
						MaxServiceDuration: 30,
						MandatoryDuration:  10,
					},
					Name:        "发型师F",
					PhoneNumber: "13564674245",
				},
			},
			Name:          "816 * Salon (松江大学城)",
			Address:       "松江区 文汇路1028号2层201",
			ContactNumber: "18616325541",
		},
		&product.Store{
			Artisans: []*product.Artisan{
				&product.Artisan{
					ArtisanSpecific: &custom.ArtisanSpecific{
						MaxServiceDuration: 60,
						MandatoryDuration:  10,
					},
					Name:        "发型师G",
					PhoneNumber: "13564674246",
				},
				&product.Artisan{
					ArtisanSpecific: &custom.ArtisanSpecific{
						MaxServiceDuration: 40,
						MandatoryDuration:  10,
					},
					Name:        "发型师H",
					PhoneNumber: "13564674247",
				},
				&product.Artisan{
					ArtisanSpecific: &custom.ArtisanSpecific{
						MaxServiceDuration: 30,
						MandatoryDuration:  10,
					},
					Name:        "发型师I",
					PhoneNumber: "13564674248",
				},
			},
			Name:          "816 * Salon (九亭店)",
			Address:       "松江区 涞坊路373号",
			ContactNumber: "18616325542",
		},
	}
}
