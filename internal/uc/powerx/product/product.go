package product

import (
	model2 "PowerX/internal/model"
	"PowerX/internal/model/media"
	"PowerX/internal/model/powermodel"
	model "PowerX/internal/model/product"
	"PowerX/internal/types"
	"PowerX/internal/types/errorx"
	"PowerX/pkg/slicex"
	"context"
	"encoding/json"
	"github.com/pkg/errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strings"
)

type ProductUseCase struct {
	db *gorm.DB
}

func NewProductUseCase(db *gorm.DB) *ProductUseCase {
	return &ProductUseCase{
		db: db,
	}
}

type FindManyProductsOption struct {
	Types         []int
	NotInTypes    []int
	Plans         []int
	SkuIds        []int64
	Ids           []int64
	NeedActivated bool
	CategoryId    int
	LikeName      string
	OrderBy       string
	types.PageEmbedOption
}

func (uc *ProductUseCase) buildFindQueryNoPage(db *gorm.DB, opt *FindManyProductsOption) *gorm.DB {

	if len(opt.LikeName) > 0 {
		db = db.Where("name LIKE ?", "%"+opt.LikeName+"%")
	}

	if len(opt.Types) > 0 {
		db = db.Where("type IN ?", opt.Types)
	}

	if len(opt.NotInTypes) > 0 {
		db = db.Where("type NOT IN ?", opt.NotInTypes)
	}

	if len(opt.Ids) > 0 {
		db = db.Where("id IN ?", opt.Ids)
	}

	if len(opt.Plans) > 0 {
		db = db.Where("plan IN ?", opt.Plans)
	}

	if opt.NeedActivated {
		db = db.Where("is_activated = ?", true)
	}

	if opt.CategoryId > 0 {
		db = db.
			Joins("LEFT JOIN pivot_product_to_product_category ON pivot_product_to_product_category.product_id = products.id").
			Joins("LEFT JOIN product_categories ON product_categories.id = pivot_product_to_product_category.product_category_id").
			Where("product_categories.id = ?", opt.CategoryId).
			Where("pivot_product_to_product_category.deleted_at IS NULL")
	}

	if opt.LikeName != "" {
		db = db.Where("name LIKE ?", "%"+opt.LikeName+"%")
	}

	orderBy := "sort desc, id "
	if opt.OrderBy != "" {
		orderBy = opt.OrderBy + "," + orderBy
	}
	db.Order(orderBy)

	return db
}

func (uc *ProductUseCase) PreloadItems(db *gorm.DB) *gorm.DB {
	db = db.
		Preload("PivotCoverImages", "media_usage = ?", media.MediaUsageCover).Preload("PivotCoverImages.MediaResource").
		Preload("PivotDetailImages", "media_usage = ?", media.MediaUsageDetail).Preload("PivotDetailImages.MediaResource").
		Preload("ProductCategories").
		Preload("PriceBookEntries.PriceBook").
		Preload("SKUs.PriceBookEntry").
		Preload("SKUs.PivotSkuToSpecificOptions").
		Preload("ProductSpecifics.Options").
		Preload("PivotSalesChannels.DataDictionaryItem", "type=?", model2.TypeSalesChannel).
		Preload("PivotPromoteChannels.DataDictionaryItem", "type=?", model2.TypePromoteChannel)

	return db
}

func (uc *ProductUseCase) FindManyProducts(ctx context.Context, opt *FindManyProductsOption) (pageList types.Page[*model.Product], err error) {
	var products []*model.Product
	db := uc.db.WithContext(ctx).Model(&model.Product{})

	db = uc.buildFindQueryNoPage(db, opt)

	var count int64
	if err := db.Count(&count).Error; err != nil {
		panic(err)
	}

	opt.DefaultPageIfNotSet()
	if opt.PageIndex != 0 && opt.PageSize != 0 {
		db.Offset((opt.PageIndex - 1) * opt.PageSize).Limit(opt.PageSize)
	}

	db = uc.PreloadItems(db)
	if err := db.
		Debug().
		Find(&products).Error; err != nil {
		panic(err)
	}

	return types.Page[*model.Product]{
		List:      products,
		PageIndex: opt.PageIndex,
		PageSize:  opt.PageSize,
		Total:     count,
	}, nil
}

func (uc *ProductUseCase) CreateProduct(ctx context.Context, product *model.Product) error {

	if err := uc.db.WithContext(ctx).
		//Debug().
		Create(&product).Error; err != nil {
		if strings.Contains(err.Error(), "duplicate key value violates unique constraint") {
			return errorx.WithCause(errorx.ErrDuplicatedInsert, "该对象不能重复创建")
		}
		panic(err)
	}
	return nil
}

func (uc *ProductUseCase) UpsertProduct(ctx context.Context, product *model.Product) (*model.Product, error) {

	products := []*model.Product{product}

	err := uc.db.Transaction(func(tx *gorm.DB) error {
		// 删除产品的相关联对象
		_, err := uc.ClearAssociations(tx, product)
		if err != nil {
			return err
		}

		// 更新产品对象主体
		_, err = uc.UpsertProducts(ctx, products)
		if err != nil {
			return errors.Wrap(err, "upsert product failed")
		}

		return err
	})

	return product, err
}

func (uc *ProductUseCase) UpsertProducts(ctx context.Context, products []*model.Product) ([]*model.Product, error) {

	err := powermodel.UpsertModelsOnUniqueID(uc.db.WithContext(ctx), &model.Product{}, model.ProductUniqueId, products, nil, true)

	if err != nil {
		panic(errors.Wrap(err, "batch upsert products failed"))
	}

	return products, err
}

func (uc *ProductUseCase) PatchProduct(ctx context.Context, id int64, product *model.Product) {
	if err := uc.db.WithContext(ctx).Model(&model.Product{}).
		Where(id).Updates(&product).Error; err != nil {
		panic(err)
	}
}

func (uc *ProductUseCase) GetProduct(ctx context.Context, id int64) (*model.Product, error) {
	var product = &model.Product{}

	db := uc.db.WithContext(ctx)
	db = uc.PreloadItems(db)
	if err := db.
		//Debug().
		First(product, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errorx.WithCause(errorx.ErrBadRequest, "未找到产品")
		}
		panic(err)
	}

	return product, nil
}

func (uc *ProductUseCase) DeleteProduct(ctx context.Context, id int64) error {

	// 获取产品相关项
	product, err := uc.GetProduct(ctx, id)
	if err != nil {
		return errorx.ErrNotFoundObject
	}

	err = uc.db.Transaction(func(tx *gorm.DB) error {
		// 删除产品相关项
		_, err = uc.ClearAssociations(tx, product)
		if err != nil {
			return err
		}

		result := tx.Delete(&model.Product{}, id)
		if err := result.Error; err != nil {
			panic(err)
		}
		if result.RowsAffected == 0 {
			return errorx.WithCause(errorx.ErrBadRequest, "未找到产品")
		}
		return err
	})

	return err
}

func (uc *ProductUseCase) LoadAssociations(product *model.Product) (*model.Product, error) {
	var err error
	product.PivotSalesChannels, err = product.LoadPivotSalesChannels(uc.db, nil, false)
	if err != nil {
		return nil, err
	}
	product.PivotPromoteChannels, err = product.LoadPromoteChannels(uc.db, nil, false)
	if err != nil {
		return nil, err
	}
	product.ProductCategories, err = product.LoadProductCategories(uc.db, nil)
	if err != nil {
		return nil, err
	}

	product.PivotDetailImages, err = product.LoadPivotDetailImages(uc.db, nil)
	if err != nil {
		return nil, err
	}

	return product, err
}

func (uc *ProductUseCase) ClearAssociations(db *gorm.DB, product *model.Product) (*model.Product, error) {
	var err error

	// 清除销售渠道的关联
	err = product.ClearPivotSalesChannels(db)
	if err != nil {
		return nil, err
	}
	// 清除推广渠道的关联
	err = product.ClearPivotPromoteChannels(db)
	if err != nil {
		return nil, err
	}

	// --- 清除产品品类记录 ---
	err = product.ClearProductCategories(db)
	if err != nil {
		return nil, err
	}

	// 清除产品头图图片记录
	err = product.ClearPivotCoverImages(db)
	if err != nil {
		return nil, err
	}

	// 清除产品详细图片记录
	err = product.ClearPivotDetailImages(db)
	if err != nil {
		return nil, err
	}

	return product, err

}

func (uc *ProductUseCase) ReFactSKUs(ctx context.Context, product *model.Product) error {

	db := uc.db.WithContext(ctx)
	err := db.Transaction(func(tx *gorm.DB) error {

		// upsert skus
		skus := uc.GenerateSKUsFromSpecifics(context.Background(), product)
		if len(skus) > 0 {
			err := db.Model(&model.SKU{}).
				//Debug().
				Clauses(clause.OnConflict{
					Columns:   []clause.Column{{Name: model.SkuUniqueId}},
					DoUpdates: clause.AssignmentColumns(powermodel.GetModelFields(&model.SKU{})),
				}).
				Create(&skus).Error
			if err != nil {
				return err
			}
		}

		// upsert PivotSKUsFromSpecifics
		pivots := uc.GeneratePivotSKUsFromSpecifics(context.Background(), product.ProductSpecifics, skus)
		if len(pivots) > 0 {
			err := db.Model(&model.PivotSkuToSpecificOption{}).
				//Debug().
				Clauses(clause.OnConflict{
					Columns:   []clause.Column{{Name: model.PivotPivotSkuToSpecificOptionsUniqueId}},
					DoUpdates: clause.AssignmentColumns(powermodel.GetModelFields(&model.PivotSkuToSpecificOption{})),
				}).Create(&pivots).Error
			if err != nil {
				panic(errors.Wrap(err, "init sku pivots failed"))
			}

		}

		return nil
	})

	return err
}

func (uc *ProductUseCase) GenerateSKUsFromSpecifics(ctx context.Context, product *model.Product) []*model.SKU {
	var skus []*model.SKU
	GenerateSKURecursively(product, product.ProductSpecifics, 0, &model.SKU{}, &skus)
	return skus
}

func GenerateSKURecursively(product *model.Product, specifics []*model.ProductSpecific, currentIndexOfSpecific int, currentSKU *model.SKU, skus *[]*model.SKU) {
	if currentIndexOfSpecific == len(specifics) {
		// 如何平行的遍历，已经到达了每个规格，就把sku添加到sku数组里
		*skus = append(*skus, currentSKU)
		return
	}
	currentSpecific := specifics[currentIndexOfSpecific]
	for _, option := range currentSpecific.Options {
		newSKU := *currentSKU // 创建新的 SKU 副本
		newSKU.OptionIds, _ = json.Marshal([]int64{})
		newSKU.ProductId = currentSpecific.ProductId

		// 创建sku名字
		if newSKU.SkuNo == "" {
			newSKU.SkuNo = product.SPU + "-" + option.Name
		} else {
			newSKU.SkuNo = newSKU.SkuNo + "-" + option.Name
		}

		// 将规格项的ID追加到当前sku副本中
		var currentOptionIds []int64
		// 先读取当前SKU到规格项IDs数组，因为这里非常重要，是需要压栈上层记录到SKU
		_ = json.Unmarshal(currentSKU.OptionIds, &currentOptionIds)
		// 然后把该规格项到ID，添加到新到复刻到SKU组中
		currentOptionIds = append(currentOptionIds, option.Id)
		newSKU.OptionIds, _ = json.Marshal(currentOptionIds)
		newSKU.UniqueID = newSKU.GetComposedUniqueID()
		//fmt.Dump(newSKU.OptionIds.String(), newSKU.ProductId, newSKU.UniqueID)

		GenerateSKURecursively(product, specifics, currentIndexOfSpecific+1, &newSKU, skus)
	}
}

func (uc *ProductUseCase) GeneratePivotSKUsFromSpecifics(ctx context.Context, specifics []*model.ProductSpecific, skus []*model.SKU) []*model.PivotSkuToSpecificOption {

	pivots := []*model.PivotSkuToSpecificOption{}
	for _, sku := range skus {
		var currentOptionIds []int64
		_ = json.Unmarshal(sku.OptionIds, &currentOptionIds)

		for _, specific := range specifics {
			for _, option := range specific.Options {

				isActivated := false
				if slicex.Contains(currentOptionIds, option.Id) {
					isActivated = true
				}

				pivot := &model.PivotSkuToSpecificOption{
					ProductId:        sku.ProductId,
					SkuId:            sku.Id,
					SpecificId:       specific.Id,
					SpecificOptionId: option.Id,
					IsActivated:      isActivated,
				}
				pivot.UniqueID = pivot.GetPivotComposedUniqueID()

				pivots = append(pivots, pivot)
			}
		}
	}

	return pivots
}
