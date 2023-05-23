package product

import (
	"PowerX/internal/model/powermodel"
	"PowerX/internal/model/product"
	"PowerX/internal/types"
	"PowerX/internal/types/errorx"
	"context"
	"github.com/pkg/errors"
	"gorm.io/gorm"
	"strings"
)

type PriceBookEntryUseCase struct {
	db *gorm.DB
}

func NewPriceBookEntryUseCase(db *gorm.DB) *PriceBookEntryUseCase {
	return &PriceBookEntryUseCase{
		db: db,
	}
}

type FindPriceBookEntryOption struct {
	OrderBy     string
	Ids         []int64
	PriceBookId int64
	ProductIds  []int64
	SkuIds      []int64
	types.PageEmbedOption
}

func (uc *PriceBookEntryUseCase) buildFindQueryNoPage(query *gorm.DB, opt *FindPriceBookEntryOption) *gorm.DB {
	if len(opt.Ids) > 0 {
		query.Where("id in ?", opt.Ids)
	}
	if opt.PriceBookId > 0 {
		query.Where("price_book_id = ?", opt.PriceBookId)
	}

	if len(opt.ProductIds) > 0 {
		query.Where("(product_id IN ? AND sku_id = ?)", opt.ProductIds, 0)

	}
	if len(opt.SkuIds) > 0 {
		query.Where("sku_id in ?", opt.SkuIds)
	}

	orderBy := "id desc"
	if opt.OrderBy != "" {
		orderBy = opt.OrderBy + "," + orderBy
	}
	query.Order(orderBy)

	return query
}

func (uc *PriceBookEntryUseCase) PreloadItems(db *gorm.DB) *gorm.DB {
	db = db.
		Preload("Product.PivotCoverImages").
		Preload("SKU")
	return db
}

func (uc *PriceBookEntryUseCase) FindManyPriceBookEntries(ctx context.Context, opt *FindPriceBookEntryOption) types.Page[*product.PriceBookEntry] {
	var priceBookEntries []*product.PriceBookEntry
	var count int64
	query := uc.db.WithContext(ctx).Model(&product.PriceBookEntry{})

	query = uc.buildFindQueryNoPage(query, opt)
	if err := query.Count(&count).Error; err != nil {
		panic(errors.Wrap(err, "find many priceBookEntries failed"))
	}

	opt.DefaultPageIfNotSet()
	if opt.PageIndex != 0 && opt.PageSize != 0 {
		query.Offset((opt.PageIndex - 1) * opt.PageSize).Limit(opt.PageSize)
	}

	query = uc.PreloadItems(query)
	if err := query.
		Debug().
		Find(&priceBookEntries).Error; err != nil {
		panic(errors.Wrap(err, "find many priceBookEntries failed"))
	}
	return types.Page[*product.PriceBookEntry]{
		List:      priceBookEntries,
		PageIndex: opt.PageIndex,
		PageSize:  opt.PageSize,
		Total:     count,
	}

}

func (uc *PriceBookEntryUseCase) FindOnePriceBookEntry(ctx context.Context, opt *FindPriceBookEntryOption) (*product.PriceBookEntry, error) {
	var mpCustomer *product.PriceBookEntry
	query := uc.db.WithContext(ctx).Model(&product.PriceBookEntry{})

	query = uc.buildFindQueryNoPage(query, opt)
	if err := query.First(&mpCustomer).Error; err != nil {
		return nil, errorx.ErrRecordNotFound
	}
	return mpCustomer, nil
}

func (uc *PriceBookEntryUseCase) CreatePriceBookEntry(ctx context.Context, priceBookEntry *product.PriceBookEntry) error {
	if err := uc.db.WithContext(ctx).Create(&priceBookEntry).Error; err != nil {
		if strings.Contains(err.Error(), "duplicate key value violates unique constraint") {
			return errorx.WithCause(errorx.ErrDuplicatedInsert, "该对象不能重复创建")
		}
		panic(err)
	}
	return nil
}

func (uc *PriceBookEntryUseCase) UpsertPriceBookEntry(ctx context.Context, priceBookEntry *product.PriceBookEntry) (*product.PriceBookEntry, error) {

	priceBookEntries := []*product.PriceBookEntry{priceBookEntry}

	_, err := uc.UpsertPriceBookEntries(ctx, priceBookEntries)
	if err != nil {
		panic(errors.Wrap(err, "upsert priceBookEntry failed"))
	}

	return priceBookEntry, err
}

func (uc *PriceBookEntryUseCase) UpsertPriceBookEntries(ctx context.Context, priceBookEntries []*product.PriceBookEntry) ([]*product.PriceBookEntry, error) {

	err := powermodel.UpsertModelsOnUniqueID(uc.db.WithContext(ctx), &product.PriceBookEntry{}, product.PriceBookEntryUniqueId, priceBookEntries, nil, false)

	if err != nil {
		panic(errors.Wrap(err, "batch upsert priceBookEntries failed"))
	}

	return priceBookEntries, err
}

func (uc *PriceBookEntryUseCase) PatchPriceBookEntry(ctx context.Context, id int64, priceBookEntry *product.PriceBookEntry) {
	if err := uc.db.WithContext(ctx).Model(&product.PriceBookEntry{}).Where(id).Updates(&priceBookEntry).Error; err != nil {
		panic(err)
	}
}

func (uc *PriceBookEntryUseCase) GetPriceBookEntry(ctx context.Context, id int64) (*product.PriceBookEntry, error) {
	var priceBookEntry product.PriceBookEntry
	if err := uc.db.WithContext(ctx).First(&priceBookEntry, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errorx.WithCause(errorx.ErrBadRequest, "未找到价格手册")
		}
		panic(err)
	}
	return &priceBookEntry, nil
}

func (uc *PriceBookEntryUseCase) DeletePriceBookEntry(ctx context.Context, id int64) error {
	result := uc.db.WithContext(ctx).Delete(&product.PriceBookEntry{}, id)
	if err := result.Error; err != nil {
		panic(err)
	}
	if result.RowsAffected == 0 {
		return errorx.WithCause(errorx.ErrDeleteObjectNotFound, "未找到价格手册")
	}
	return nil
}
