package seed

import (
	product2 "PowerX/internal/model/custom/product"
	"PowerX/internal/model/product"
	product3 "PowerX/internal/uc/powerx/product"
	"context"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

var stores []*product.Store

func CreateServiceSpecific(db *gorm.DB) (err error) {
	var count int64
	if err = db.Model(&product2.ServiceSpecific{}).Count(&count).Error; err != nil {
		panic(errors.Wrap(err, "init root dep failed"))
	}
	if count == 0 {

		ucStore := product3.NewStoreUseCase(db)
		stores, _ = ucStore.FindAllShops(context.Background(), &product3.FindManyStoresOption{})
		if err != nil {
			panic(errors.Wrap(err, "get stores failed"))
		}

		configs := DefaultServiceSpecific()
		if err := db.Model(&product2.ServiceSpecific{}).Create(&configs).Error; err != nil {
			panic(errors.Wrap(err, "init root dep failed"))
		}
	}

	return err
}

func DefaultServiceSpecific() []*product2.ServiceSpecific {

	configs := []*product2.ServiceSpecific{}

	var item *product2.ServiceSpecific
	// 剪发（男）	洗头	5min	剪发	25min	洗头	5min	吹干	5min														40min
	item = getServiceSpecific(getProduct("剪发（男）"), []*product2.ServiceSpecific{
		getServiceSpecific(nil, nil, "洗头", false, 5, 5),
		getServiceSpecific(nil, nil, "剪发", false, 25, 25),
		getServiceSpecific(nil, nil, "洗头", false, 5, 5),
		getServiceSpecific(nil, nil, "吹干", false, 5, 5),
	}, "剪发（男）", false, 40, 40)
	item.Stores = stores
	configs = append(configs, item)

	// 剪发（女）	洗头	10min	剪发	40min	吹干	10min																	60min
	item = getServiceSpecific(getProduct("剪发（女）"), []*product2.ServiceSpecific{
		getServiceSpecific(nil, nil, "洗头", false, 10, 10),
		getServiceSpecific(nil, nil, "剪发", false, 40, 40),
		getServiceSpecific(nil, nil, "吹干", false, 10, 10),
	}, "剪发（女）", false, 60, 60)
	item.Stores = stores
	configs = append(configs, item)

	// 洗吹	洗头	10min	吹干	15min	造型	25min																		50min
	item = getServiceSpecific(getProduct("洗吹"), []*product2.ServiceSpecific{
		getServiceSpecific(nil, nil, "洗头", false, 10, 10),
		getServiceSpecific(nil, nil, "吹干", false, 15, 15),
		getServiceSpecific(nil, nil, "造型", false, 25, 25),
	}, "洗吹", false, 50, 50)
	item.Stores = stores
	configs = append(configs, item)

	// 染发（短）	刷颜色	30min	空闲	30min	洗头	10min	吹干	10min													80min
	item = getServiceSpecific(getProduct("染发（短）"), []*product2.ServiceSpecific{
		getServiceSpecific(nil, nil, "刷颜色", false, 30, 30),
		getServiceSpecific(nil, nil, "空闲", true, 30, 0),
		getServiceSpecific(nil, nil, "洗头", false, 10, 10),
		getServiceSpecific(nil, nil, "吹干", false, 10, 10),
	}, "染发（短）", false, 80, 50)
	item.Stores = stores
	configs = append(configs, item)

	// 染发（长）	刷颜色	40min	空闲	30min	洗头	10min	吹干	15min													95min
	item = getServiceSpecific(getProduct("染发（长）"), []*product2.ServiceSpecific{
		getServiceSpecific(nil, nil, "刷颜色", false, 40, 40),
		getServiceSpecific(nil, nil, "空闲", true, 30, 0),
		getServiceSpecific(nil, nil, "洗头", false, 10, 10),
		getServiceSpecific(nil, nil, "吹干", false, 15, 15),
	}, "染发（长）", false, 95, 65)
	item.Stores = stores
	configs = append(configs, item)

	// 补漂发根/次	刷漂粉	20min	空闲	30min	洗头	10min	吹干	5min												65min
	item = getServiceSpecific(getProduct("补漂发根/次"), []*product2.ServiceSpecific{
		getServiceSpecific(nil, nil, "刷漂粉", false, 20, 20),
		getServiceSpecific(nil, nil, "空闲", true, 30, 0),
		getServiceSpecific(nil, nil, "洗头", false, 10, 10),
		getServiceSpecific(nil, nil, "吹干", false, 5, 5),
	}, "补漂发根/次", false, 65, 35)
	item.Stores = stores
	configs = append(configs, item)

	// 漂发（短）/次	刷漂粉	20min	空闲	40min	洗头	10min	吹干	5min												75min
	item = getServiceSpecific(getProduct("漂发（短）/次"), []*product2.ServiceSpecific{
		getServiceSpecific(nil, nil, "刷漂粉", false, 20, 20),
		getServiceSpecific(nil, nil, "空闲", true, 40, 0),
		getServiceSpecific(nil, nil, "洗头", false, 10, 10),
		getServiceSpecific(nil, nil, "吹干", false, 5, 5),
	}, "漂发（短）/次", false, 65, 25)
	item.Stores = stores
	configs = append(configs, item)

	// 漂发（长）/次	刷漂粉	30min	空闲	40min	洗头	10min	吹干	5min												85min
	item = getServiceSpecific(getProduct("漂发（长）/次"), []*product2.ServiceSpecific{
		getServiceSpecific(nil, nil, "刷漂粉", false, 30, 30),
		getServiceSpecific(nil, nil, "空闲", true, 40, 0),
		getServiceSpecific(nil, nil, "洗头", false, 10, 10),
		getServiceSpecific(nil, nil, "吹干", false, 5, 5),
	}, "补漂发根/次", false, 75, 35)
	item.Stores = stores
	configs = append(configs, item)

	// 冷烫/柔顺	洗头	10min	软化	15min	空闲	25min	洗头	5min	造型	30min	定型	15min	洗头	5min	吹干	15min		120min
	item = getServiceSpecific(getProduct("冷烫/柔顺"), []*product2.ServiceSpecific{
		getServiceSpecific(nil, nil, "洗头", false, 10, 10),
		getServiceSpecific(nil, nil, "软化", false, 15, 15),
		getServiceSpecific(nil, nil, "空闲", true, 25, 0),
		getServiceSpecific(nil, nil, "洗头", false, 5, 5),
		getServiceSpecific(nil, nil, "造型", false, 30, 30),
		getServiceSpecific(nil, nil, "定型", false, 15, 15),
		getServiceSpecific(nil, nil, "洗头", false, 5, 5),
		getServiceSpecific(nil, nil, "吹干", false, 15, 15),
	}, "冷烫/柔顺", false, 95, 70)
	item.Stores = stores
	configs = append(configs, item)

	// 热烫/拉直	洗头	10min	软化	15min	空闲	25min	洗头	10min	造型	30min	定型	15min	洗头	5min	吹干	15min		125min
	item = getServiceSpecific(getProduct("热烫/拉直"), []*product2.ServiceSpecific{
		getServiceSpecific(nil, nil, "洗头", false, 10, 10),
		getServiceSpecific(nil, nil, "软化", false, 15, 15),
		getServiceSpecific(nil, nil, "空闲", true, 25, 0),
		getServiceSpecific(nil, nil, "洗头", false, 10, 10),
		getServiceSpecific(nil, nil, "造型", false, 30, 30),
		getServiceSpecific(nil, nil, "定型", false, 15, 15),
		getServiceSpecific(nil, nil, "洗头", false, 5, 5),
		getServiceSpecific(nil, nil, "吹干", false, 15, 15),
	}, "热烫/拉直", false, 125, 100)
	item.Stores = stores
	configs = append(configs, item)

	// 水疗	洗头	10min	护理	20min	洗头	5min	吹干	15min															50min
	item = getServiceSpecific(getProduct("水疗"), []*product2.ServiceSpecific{
		getServiceSpecific(nil, nil, "洗头", false, 10, 10),
		getServiceSpecific(nil, nil, "护理", false, 20, 20),
		getServiceSpecific(nil, nil, "洗头", false, 5, 5),
		getServiceSpecific(nil, nil, "吹干", false, 15, 15),
	}, "水疗", false, 50, 50)
	item.Stores = stores
	configs = append(configs, item)

	// 护理	洗头	10min	护理	10min	空闲	25min	洗头	5min	吹干	15min												65min
	item = getServiceSpecific(getProduct("护理"), []*product2.ServiceSpecific{
		getServiceSpecific(nil, nil, "洗头", false, 10, 10),
		getServiceSpecific(nil, nil, "护理", false, 10, 10),
		getServiceSpecific(nil, nil, "空闲", true, 25, 0),
		getServiceSpecific(nil, nil, "洗头", false, 5, 5),
		getServiceSpecific(nil, nil, "吹干", false, 15, 15),
	}, "护理", false, 65, 40)
	item.Stores = stores
	configs = append(configs, item)

	return configs
}

func getProduct(name string) *product.Product {
	ctx := context.Background()
	ddProductTypeService := UseCaseDD.GetCachedDDId(ctx, product.TypeProductType, product.ProductTypeService)
	ddProductTypeOnce := UseCaseDD.GetCachedDDId(ctx, product.TypeProductPlan, product.ProductPlanOnce)

	return &product.Product{
		Name:          name,
		Type:          int(ddProductTypeService),
		Plan:          int(ddProductTypeOnce),
		CanSellOnline: true,
		IsActivated:   true,
	}
}

func getServiceSpecific(product *product.Product, children []*product2.ServiceSpecific, name string, isFree bool, duration int, mDuration int) *product2.ServiceSpecific {
	return &product2.ServiceSpecific{
		Product:           product,
		Children:          children,
		Name:              name,
		IsFree:            isFree,
		Duration:          duration,
		MandatoryDuration: mDuration,
	}
}
