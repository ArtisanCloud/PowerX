package product

import (
	"PowerX/internal/model"
	"PowerX/internal/model/media"
	"PowerX/internal/model/powermodel"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"time"
)

type ProductAttribute struct {
	Inventory  int16          `gorm:"comment:库存" json:"inventory"`
	SoldAmount int16          `gorm:"comment:已售数量" json:"soldAmount"`
	Weight     float32        `gorm:"comment:重量" json:"weight"`
	Volume     float32        `gorm:"comment:体积" json:"volume"`
	Encode     string         `gorm:"comment:编码" json:"encode"`
	BarCode    string         `gorm:"comment:条形码" json:"barCode"`
	Extra      datatypes.JSON `gorm:"comment:额外信息，可以json方式存储" json:"extra"`
}

type Product struct {
	SKUs                   []*SKU                               `gorm:"foreignKey:ProductId;references:Id" json:"skus"`
	ProductSpecifics       []*ProductSpecific                   `gorm:"foreignKey:ProductId;references:Id" json:"productSpecifics"`
	ProductCategories      []*ProductCategory                   `gorm:"many2many:public.pivot_product_to_product_category;foreignKey:Id;joinForeignKey:ProductId;References:Id;JoinReferences:ProductCategoryId" json:"productCategories"`
	PivotCoverImages       []*media.PivotMediaResourceToObject  `gorm:"polymorphic:Object;polymorphicValue:products" json:"pivotCoverImages"`
	PivotDetailImages      []*media.PivotMediaResourceToObject  `gorm:"polymorphic:Object;polymorphicValue:products" json:"pivotDetailImages"`
	PriceBooks             []*PriceBook                         `gorm:"many2many:public.price_book_entries;foreignKey:Id;joinForeignKey:Id;References:Id;JoinReferences:PriceBookId" json:"priceBooks"`
	PriceBookEntries       []*PriceBookEntry                    `gorm:"foreignKey:ProductId;references:Id" json:"priceBookEntries"`
	PivotSalesChannels     []*model.PivotDataDictionaryToObject `gorm:"polymorphic:Object;polymorphicValue:products" json:"pivotSalesChannels"`
	PivotPromoteChannels   []*model.PivotDataDictionaryToObject `gorm:"polymorphic:Object;polymorphicValue:products" json:"pivotPromoteChannels"`
	ProductCategoryIds     []int64                              `gorm:"-"`
	SalesChannelsItemIds   []int64                              `gorm:"-"`
	PromoteChannelsItemIds []int64                              `gorm:"-"`
	//Coupons          []*Coupon         `gorm:"many2many:public.r_product_to_coupon;foreignKey:Id;joinForeignKey:ProductId;References:Id;JoinReferences:CouponId" json:"coupons"`

	powermodel.PowerModel

	Name                string    `gorm:"comment:产品名称"`
	SPU                 string    `gorm:"comment:产品货号"`
	Type                int       `gorm:"comment:产品类型，比如商品，还是服务"`
	Plan                int       `gorm:"comment:产品计划，比如是周期性产品还是一次性产品"`
	AccountingCategory  string    `gorm:"comment:财务类别，方便和财务系统对账和审批"`
	CanSellOnline       bool      `gorm:"comment:是否允许线上销售"`
	CanUseForDeduct     bool      `gorm:"comment:产品购买，是否可以使用抵扣方式"`
	ApprovalStatus      int       `gorm:"comment:产品上架，是否审核通过"`
	IsActivated         bool      `gorm:"comment:是否被激活"`
	Description         string    `gorm:"comment:产品描述; type:text"`
	AllowedSellQuantity int       `gorm:"comment:允许购买数量上限"`
	ValidityPeriodDays  int       `gorm:"comment:售卖时间期限，按天"`
	SaleStartDate       time.Time `gorm:"comment:售卖开始时间"`
	SaleEndDate         time.Time `gorm:"comment:售卖结束时间"`
	Sort                int       `gorm:"comment:排序，越大约考前"`
	ProductAttribute
}

const TableNameProduct = "products"
const ProductUniqueId = powermodel.UniqueId

// Data Dictionary
const TypeProductType = "_product_type"
const TypeProductPlan = "_product_plan"

const ProductTypeToken = "_token"
const ProductTypeGoods = "_goods"
const ProductTypeService = "_service"

const ProductPlanOnce = "_once"
const ProductPlanPeriod = "_period"

func (mdl *Product) GetTableName(needFull bool) string {
	tableName := TableNameProduct
	if needFull {
		tableName = "public." + tableName
	}
	return tableName
}

func (mdl *Product) GetForeignReferValue() int64 {
	return mdl.Id
}

// -- pivot employees
func (mdl *Product) LoadPivotSalesChannels(db *gorm.DB, conditions *map[string]interface{}, withClauseAssociations bool) ([]*model.PivotDataDictionaryToObject, error) {
	items := []*model.PivotDataDictionaryToObject{}
	if conditions == nil {
		conditions = &map[string]interface{}{}
	}

	(*conditions)[model.PivotDataDictionaryToObjectOwnerKey] = TableNameProduct
	(*conditions)[model.PivotDataDictionaryToObjectForeignKey] = mdl.Id
	(*conditions)["data_dictionary_type"] = model.TypeSalesChannel

	err := powermodel.SelectMorphPivots(db, &model.PivotDataDictionaryToObject{}, false, false, conditions).
		Preload("DataDictionaryItem").
		Find(&items).Error

	//fmt.Dump(items)
	return items, err
}

func (mdl *Product) ClearPivotSalesChannels(db *gorm.DB) error {
	conditions := &map[string]interface{}{}
	(*conditions)[model.PivotDataDictionaryToObjectOwnerKey] = TableNameProduct
	(*conditions)[model.PivotDataDictionaryToObjectForeignKey] = mdl.Id
	(*conditions)["data_dictionary_type"] = model.TypeSalesChannel

	return powermodel.ClearMorphPivots(db, &model.PivotDataDictionaryToObject{}, false, false, conditions)
}

func (mdl *Product) LoadPromoteChannels(db *gorm.DB, conditions *map[string]interface{}, withClauseAssociations bool) ([]*model.PivotDataDictionaryToObject, error) {
	items := []*model.PivotDataDictionaryToObject{}
	if conditions == nil {
		conditions = &map[string]interface{}{}
	}

	(*conditions)[model.PivotDataDictionaryToObjectOwnerKey] = TableNameProduct
	(*conditions)[model.PivotDataDictionaryToObjectForeignKey] = mdl.Id
	(*conditions)["data_dictionary_type"] = model.TypePromoteChannel

	err := powermodel.SelectMorphPivots(db, &model.PivotDataDictionaryToObject{}, false, false, conditions).
		Preload("DataDictionaryItem").
		Find(&items).Error

	return items, err
}

func (mdl *Product) ClearPivotPromoteChannels(db *gorm.DB) error {
	conditions := &map[string]interface{}{}
	(*conditions)[model.PivotDataDictionaryToObjectOwnerKey] = TableNameProduct
	(*conditions)[model.PivotDataDictionaryToObjectForeignKey] = mdl.Id
	(*conditions)["data_dictionary_type"] = model.TypePromoteChannel

	return powermodel.ClearMorphPivots(db, &model.PivotDataDictionaryToObject{}, false, false, conditions)
}

// -- Product Category
func (mdl *Product) LoadProductCategories(db *gorm.DB, conditions *map[string]interface{}) ([]*ProductCategory, error) {
	mdl.ProductCategories = []*ProductCategory{}

	if conditions == nil {
		conditions = &map[string]interface{}{}
	}
	(*conditions)[TableNamePivotProductToProductCategory+".deleted_at"] = nil

	err := powermodel.AssociationRelationship(db, conditions, mdl, "ProductCategories", false).Find(&mdl.ProductCategories)
	if err != nil {
		panic(err)
	}
	return mdl.ProductCategories, err
}

func (mdl *Product) ClearProductCategories(db *gorm.DB) error {
	conditions := &map[string]interface{}{}
	(*conditions)[PivotProductToCategoryForeignKey] = mdl.Id
	return powermodel.ClearMorphPivots(db, &PivotProductToProductCategory{}, false, false, conditions)
}

func (mdl *Product) LoadPivotCoverImages(db *gorm.DB, conditions *map[string]interface{}) ([]*media.PivotMediaResourceToObject, error) {
	items := []*media.PivotMediaResourceToObject{}
	if conditions == nil {
		conditions = &map[string]interface{}{}
	}

	(*conditions)[media.PivotMediaResourceToObjectOwnerKey] = TableNameProduct
	(*conditions)[media.PivotMediaResourceToObjectForeignKey] = mdl.Id

	err := powermodel.SelectMorphPivots(db, &media.PivotMediaResourceToObject{}, false, false, conditions).
		Preload("MediaResource").
		Where("media_usage", media.MediaUsageCover).
		Find(&items).Error

	return items, err
}

func (mdl *Product) ClearPivotCoverImages(db *gorm.DB) error {
	conditions := &map[string]interface{}{}
	(*conditions)[media.PivotMediaResourceToObjectOwnerKey] = TableNameProduct
	(*conditions)[media.PivotMediaResourceToObjectForeignKey] = mdl.Id
	(*conditions)["media_usage"] = media.MediaUsageCover

	return powermodel.ClearMorphPivots(db, &media.PivotMediaResourceToObject{}, false, false, conditions)
}

func (mdl *Product) LoadPivotDetailImages(db *gorm.DB, conditions *map[string]interface{}) ([]*media.PivotMediaResourceToObject, error) {
	items := []*media.PivotMediaResourceToObject{}
	if conditions == nil {
		conditions = &map[string]interface{}{}
	}

	(*conditions)[media.PivotMediaResourceToObjectOwnerKey] = TableNameProduct
	(*conditions)[media.PivotMediaResourceToObjectForeignKey] = mdl.Id

	err := powermodel.SelectMorphPivots(db, &media.PivotMediaResourceToObject{}, false, false, conditions).
		Preload("MediaResource").
		Where("media_usage", media.MediaUsageDetail).
		Find(&items).Error

	return items, err
}

func (mdl *Product) ClearPivotDetailImages(db *gorm.DB) error {
	conditions := &map[string]interface{}{}
	(*conditions)[media.PivotMediaResourceToObjectOwnerKey] = TableNameProduct
	(*conditions)[media.PivotMediaResourceToObjectForeignKey] = mdl.Id
	(*conditions)["media_usage"] = media.MediaUsageDetail

	return powermodel.ClearMorphPivots(db, &media.PivotMediaResourceToObject{}, false, false, conditions)
}

// -- PriceBookEntries
func (mdl *Product) LoadPriceBookEntries(db *gorm.DB, conditions *map[string]interface{}) ([]*PriceBookEntry, error) {
	mdl.PriceBookEntries = []*PriceBookEntry{}
	err := powermodel.AssociationRelationship(db, conditions, mdl, "PriceBookEntries", false).Find(&mdl.PriceBookEntries)
	if err != nil {
		panic(err)
	}
	return mdl.PriceBookEntries, err
}
