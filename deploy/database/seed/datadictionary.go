package seed

import (
	"PowerX/deploy/database/custom/seed"
	"PowerX/internal/model"
	"PowerX/internal/model/product"
	"PowerX/internal/uc/powerx"
	"PowerX/pkg/slicex"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

var UseCaseDD *powerx.DataDictionaryUseCase

func CreateDataDictionaries(db *gorm.DB) (err error) {

	var count int64
	if err = db.Model(&model.DataDictionaryType{}).Count(&count).Error; err != nil {
		panic(errors.Wrap(err, "init data dictionary  failed"))
	}

	UseCaseDD = powerx.NewDataDictionaryUseCase(db)
	data := DefaultDataDictionary()
	customData := seed.CustomDataDictionary(db)
	data = slicex.Concatenate(data, customData)

	if count == 0 {
		if err = db.Model(&model.DataDictionaryType{}).Create(data).Error; err != nil {
			panic(errors.Wrap(err, "init data dictionary failed"))
		}
	}

	return err
}

func DefaultDataDictionary() (data []*model.DataDictionaryType) {

	data = []*model.DataDictionaryType{
		defaultStatusDataDictionary(),
		defaultApprovalStatusDataDictionary(),
		defaultSalesChannelsDataDictionary(),
		defaultPromoteChannelsDataDictionary(),
		defaultSourceDataDictionary(),
		defaultProductPlanDataDictionary(),
		defaultProductTypeDataDictionary(),
		defaultArtisanLevelDataDictionary(),
	}

	return data

}

func defaultStatusDataDictionary() *model.DataDictionaryType {
	return &model.DataDictionaryType{
		Items: []*model.DataDictionaryItem{
			&model.DataDictionaryItem{
				Key:   model.StatusDraft,
				Type:  model.TypeApprovalStatus,
				Name:  "草稿",
				Value: model.StatusDraft,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   model.StatusActive,
				Type:  model.TypeApprovalStatus,
				Name:  "激活",
				Value: model.StatusActive,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   model.StatusCanceled,
				Type:  model.TypeApprovalStatus,
				Name:  "取消",
				Value: model.StatusCanceled,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   model.StatusPending,
				Type:  model.TypeApprovalStatus,
				Name:  "代办",
				Value: model.StatusPending,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   model.StatusInactive,
				Type:  model.TypeApprovalStatus,
				Name:  "无效",
				Value: model.StatusInactive,
				Sort:  0,
			},
		},
		Type:        model.TypeObjectStatus,
		Name:        "对象状态",
		Description: "默认对象的生命状态",
	}

}

func defaultApprovalStatusDataDictionary() *model.DataDictionaryType {
	return &model.DataDictionaryType{
		Items: []*model.DataDictionaryItem{
			&model.DataDictionaryItem{
				Key:   model.ApprovalStatusApply,
				Type:  model.TypeApprovalStatus,
				Name:  "待审核",
				Value: model.ApprovalStatusApply,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   model.ApprovalStatusReject,
				Type:  model.TypeApprovalStatus,
				Name:  "拒绝",
				Value: model.ApprovalStatusReject,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   model.ApprovalStatusSuccess,
				Type:  model.TypeApprovalStatus,
				Name:  "通过",
				Value: model.ApprovalStatusSuccess,
				Sort:  0,
			},
		},
		Type:        model.TypeApprovalStatus,
		Name:        "审核状态",
		Description: "可以在哪些平台上销售",
	}
}

func defaultSalesChannelsDataDictionary() *model.DataDictionaryType {
	return &model.DataDictionaryType{
		Items: []*model.DataDictionaryItem{
			&model.DataDictionaryItem{
				Key:   model.ChannelWechat,
				Type:  model.TypeSalesChannel,
				Name:  "微信",
				Value: model.ChannelWechat,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   model.ChannelTaoBao,
				Type:  model.TypeSalesChannel,
				Name:  "淘宝",
				Value: model.ChannelTaoBao,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   model.ChannelJD,
				Type:  model.TypeSalesChannel,
				Name:  "京东",
				Value: model.ChannelJD,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   model.ChannelDianPing,
				Type:  model.TypeSalesChannel,
				Name:  "点评网",
				Value: model.ChannelDianPing,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   model.ChannelMeiTuan,
				Type:  model.TypeSalesChannel,
				Name:  "美团",
				Value: model.ChannelMeiTuan,
				Sort:  0,
			},
		},
		Type:        model.TypeSalesChannel,
		Name:        "销售平台",
		Description: "可以在哪些平台上销售",
	}
}

func defaultPromoteChannelsDataDictionary() *model.DataDictionaryType {
	return &model.DataDictionaryType{
		Items: []*model.DataDictionaryItem{
			&model.DataDictionaryItem{
				Key:   model.ChannelWechat,
				Type:  model.TypePromoteChannel,
				Name:  "微信",
				Value: model.ChannelWechat,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   model.ChannelDianPing,
				Type:  model.TypePromoteChannel,
				Name:  "点评网",
				Value: model.ChannelDianPing,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   model.ChannelMeiTuan,
				Type:  model.TypePromoteChannel,
				Name:  "美团",
				Value: model.ChannelMeiTuan,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   model.ChannelDouYin,
				Type:  model.TypePromoteChannel,
				Name:  "抖音",
				Value: model.ChannelDouYin,
				Sort:  0,
			},
		},
		Type:        model.TypePromoteChannel,
		Name:        "推广平台",
		Description: "可以在哪些平台上推广",
	}
}

func defaultSourceDataDictionary() *model.DataDictionaryType {

	return &model.DataDictionaryType{
		Items: []*model.DataDictionaryItem{
			&model.DataDictionaryItem{
				Key:   model.ChannelDirect,
				Type:  model.TypeSourceChannel,
				Name:  "品牌自营",
				Value: model.ChannelDirect,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   model.ChannelWechat,
				Type:  model.TypeSourceChannel,
				Name:  "微信",
				Value: model.ChannelWechat,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   model.ChannelDianPing,
				Type:  model.TypeSourceChannel,
				Name:  "点评网",
				Value: model.ChannelDianPing,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   model.ChannelMeiTuan,
				Type:  model.TypeSourceChannel,
				Name:  "美团",
				Value: model.ChannelMeiTuan,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   model.ChannelDouYin,
				Type:  model.TypeSourceChannel,
				Name:  "抖音",
				Value: model.ChannelDouYin,
				Sort:  0,
			},
		},
		Type:        model.TypeSourceChannel,
		Name:        "公域渠道",
		Description: "获取公域流量的来源",
	}
}

func defaultProductPlanDataDictionary() *model.DataDictionaryType {
	return &model.DataDictionaryType{
		Items: []*model.DataDictionaryItem{
			&model.DataDictionaryItem{
				Key:   product.ProductPlanOnce,
				Type:  product.TypeProductPlan,
				Name:  "实体商品",
				Value: product.ProductPlanOnce,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   product.ProductPlanPeriod,
				Type:  product.TypeProductPlan,
				Name:  "虚拟产品",
				Value: product.ProductPlanPeriod,
				Sort:  0,
			},
		},
		Type:        product.TypeProductPlan,
		Name:        "产品类型",
		Description: "产品类型分实体商品，虚拟产品",
	}
}
func defaultProductTypeDataDictionary() *model.DataDictionaryType {
	return &model.DataDictionaryType{
		Items: []*model.DataDictionaryItem{
			&model.DataDictionaryItem{
				Key:   product.ProductTypeGoods,
				Type:  product.TypeProductType,
				Name:  "普通商品",
				Value: product.ProductTypeGoods,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   product.ProductTypeService,
				Type:  product.TypeProductType,
				Name:  "周期性商品",
				Value: product.ProductTypeService,
				Sort:  0,
			},
		},
		Type:        product.TypeProductType,
		Name:        "产品计划",
		Description: "产品类型分实体商品，虚拟产品",
	}

}

func defaultArtisanLevelDataDictionary() *model.DataDictionaryType {
	return &model.DataDictionaryType{
		Items: []*model.DataDictionaryItem{
			&model.DataDictionaryItem{
				Key:   product.ArtisanLevelBasic,
				Type:  product.ArtisanLevelType,
				Name:  "初级",
				Value: product.ArtisanLevelBasic,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   product.ArtisanLevelMedium,
				Type:  product.ArtisanLevelType,
				Name:  "中级",
				Value: product.ArtisanLevelMedium,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   product.ArtisanLevelAdvanced,
				Type:  product.ArtisanLevelType,
				Name:  "高级",
				Value: product.ArtisanLevelAdvanced,
				Sort:  0,
			},
		},
		Type:        product.ArtisanLevelType,
		Name:        "Artisan等级",
		Description: "Artisan的等级区分",
	}

}
