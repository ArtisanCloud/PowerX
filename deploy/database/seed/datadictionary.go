package seed

import (
	"PowerX/internal/model"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

func CreateDataDictionaries(db *gorm.DB) (err error) {

	var count int64
	if err = db.Model(&model.DataDictionaryType{}).Count(&count).Error; err != nil {
		panic(errors.Wrap(err, "init data dictionary  failed"))
	}

	data := DefaultDataDictionary()

	if count == 0 {
		if err = db.Model(&model.DataDictionaryType{}).Create(data).Error; err != nil {
			panic(errors.Wrap(err, "init data dictionary failed"))
		}
	}

	return err
}

func DefaultDataDictionary() (data []*model.DataDictionaryType) {

	data = []*model.DataDictionaryType{
		defaultApprovalStatusDataDictionary(),
		defaultSalesChannelsDataDictionary(),
		defaultPromoteChannelsDataDictionary(),
		defaultSourceDataDictionary(),
		defaultProductPlanDataDictionary(),
		defaultProductTypeDataDictionary(),
	}

	return data

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
				Key:   model.ProductPlanOnce,
				Type:  model.TypeProductPlan,
				Name:  "实体商品",
				Value: model.ProductPlanOnce,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   model.ProductPlanPeriod,
				Type:  model.TypeProductPlan,
				Name:  "虚拟产品",
				Value: model.ProductPlanPeriod,
				Sort:  0,
			},
		},
		Type:        model.TypeProductPlan,
		Name:        "产品类型",
		Description: "产品类型分实体商品，虚拟产品",
	}
}
func defaultProductTypeDataDictionary() *model.DataDictionaryType {
	return &model.DataDictionaryType{
		Items: []*model.DataDictionaryItem{
			&model.DataDictionaryItem{
				Key:   model.ProductTypeGoods,
				Type:  model.TypeProductType,
				Name:  "普通商品",
				Value: model.ProductTypeGoods,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   model.ProductTypeService,
				Type:  model.TypeProductType,
				Name:  "周期性商品",
				Value: model.ProductTypeService,
				Sort:  0,
			},
		},
		Type:        model.TypeProductType,
		Name:        "产品计划",
		Description: "产品类型分实体商品，虚拟产品",
	}

}
