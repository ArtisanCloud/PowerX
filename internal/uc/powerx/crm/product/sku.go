package product

import (
	"PowerX/internal/model/crm/product"
	"PowerX/internal/model/powermodel"
	"PowerX/internal/types"
	"PowerX/internal/types/errorx"
	"context"
	"github.com/pkg/errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strings"
)

type SKUUseCase struct {
	db *gorm.DB
}

func NewSKUUseCase(db *gorm.DB) *SKUUseCase {
	return &SKUUseCase{
		db: db,
	}
}

type FindSKUOption struct {
	OrderBy   string
	Ids       []int64
	ProductId int64
	types.PageEmbedOption
}

func (uc *SKUUseCase) buildFindQueryNoPage(query *gorm.DB, opt *FindSKUOption) *gorm.DB {
	if len(opt.Ids) > 0 {
		query.Where("id in ?", opt.Ids)
	}

	if opt.ProductId > 0 {
		query.Where("product_id = ?", opt.ProductId)

	}

	orderBy := "id desc"
	if opt.OrderBy != "" {
		orderBy = opt.OrderBy + "," + orderBy
	}
	query.Order(orderBy)

	return query
}

func (uc *SKUUseCase) PreloadItems(db *gorm.DB) *gorm.DB {
	db = db.
		Preload("Options")
	return db
}

func (uc *SKUUseCase) FindManySKUs(ctx context.Context, opt *FindSKUOption) types.Page[*product.SKU] {
	var SKUs []*product.SKU
	var count int64
	query := uc.db.WithContext(ctx).Model(&product.SKU{})

	query = uc.buildFindQueryNoPage(query, opt)
	if err := query.Count(&count).Error; err != nil {
		panic(errors.Wrap(err, "find many SKUs failed"))
	}

	opt.DefaultPageIfNotSet()
	if opt.PageIndex != 0 && opt.PageSize != 0 {
		query.Offset((opt.PageIndex - 1) * opt.PageSize).Limit(opt.PageSize)
	}

	query = uc.PreloadItems(query)
	if err := query.
		//Debug().
		Find(&SKUs).Error; err != nil {
		panic(errors.Wrap(err, "find many SKUs failed"))
	}
	return types.Page[*product.SKU]{
		List:      SKUs,
		PageIndex: opt.PageIndex,
		PageSize:  opt.PageSize,
		Total:     count,
	}

}

func (uc *SKUUseCase) FindOneSKU(ctx context.Context, opt *FindSKUOption) (*product.SKU, error) {
	var mpCustomer *product.SKU
	query := uc.db.WithContext(ctx).Model(&product.SKU{})

	query = uc.buildFindQueryNoPage(query, opt)
	if err := query.First(&mpCustomer).Error; err != nil {
		return nil, errorx.ErrRecordNotFound
	}
	return mpCustomer, nil
}

func (uc *SKUUseCase) ConfigSKU(ctx context.Context, SKUs []*product.SKU) error {
	db := uc.db.WithContext(ctx)
	err := db.Transaction(func(tx *gorm.DB) error {

		// upsert product specifics
		err := db.
			//Debug().
			Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: product.SkuUniqueId}},
				DoUpdates: clause.AssignmentColumns([]string{"sku_no", "inventory"}),
			}).Create(SKUs).Error

		if err != nil {
			return err
		}

		return nil
	})

	return err
}

func (uc *SKUUseCase) CreateSKU(ctx context.Context, SKU *product.SKU) error {
	if err := uc.db.WithContext(ctx).Create(&SKU).Error; err != nil {
		if strings.Contains(err.Error(), "duplicate key value violates unique constraint") {
			return errorx.WithCause(errorx.ErrDuplicatedInsert, "该对象不能重复创建")
		}
		panic(err)
	}
	return nil
}

func (uc *SKUUseCase) UpsertSKU(ctx context.Context, productSpecific *product.SKU) (*product.SKU, error) {

	productSpecifics := []*product.SKU{productSpecific}

	_, err := uc.UpsertSKUs(ctx, productSpecifics)
	if err != nil {
		panic(errors.Wrap(err, "upsert productSpecific failed"))
	}

	return productSpecific, err
}

func (uc *SKUUseCase) UpsertSKUs(ctx context.Context, productSpecifics []*product.SKU) ([]*product.SKU, error) {

	err := powermodel.UpsertModelsOnUniqueID(uc.db.WithContext(ctx), &product.SKU{}, product.SkuUniqueId, productSpecifics, nil, true)

	if err != nil {
		panic(errors.Wrap(err, "batch upsert productSpecific failed"))
	}

	return productSpecifics, err
}

func (uc *SKUUseCase) PatchSKU(ctx context.Context, id int64, SKU *product.SKU) {
	if err := uc.db.WithContext(ctx).Model(&product.SKU{}).Where(id).Updates(&SKU).Error; err != nil {
		panic(err)
	}
}

func (uc *SKUUseCase) GetSKU(ctx context.Context, id int64) (*product.SKU, error) {
	var SKU product.SKU
	if err := uc.db.WithContext(ctx).First(&SKU, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errorx.WithCause(errorx.ErrBadRequest, "未找到价格手册")
		}
		panic(err)
	}
	return &SKU, nil
}

func (uc *SKUUseCase) DeleteSKU(ctx context.Context, id int64) error {
	result := uc.db.WithContext(ctx).Delete(&product.SKU{}, id)
	if err := result.Error; err != nil {
		panic(err)
	}
	if result.RowsAffected == 0 {
		return errorx.WithCause(errorx.ErrDeleteObjectNotFound, "未找到价格手册")
	}
	return nil
}
