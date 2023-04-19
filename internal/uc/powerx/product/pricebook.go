package product

import (
	"PowerX/internal/model/powermodel"
	"PowerX/internal/model/product"
	"PowerX/internal/types"
	"PowerX/internal/types/errorx"
	"context"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

type PriceBookUseCase struct {
	db *gorm.DB
}

func NewPriceBookUseCase(db *gorm.DB) *PriceBookUseCase {
	return &PriceBookUseCase{
		db: db,
	}
}

func (uc *PriceBookUseCase) buildFindQueryNoPage(query *gorm.DB, opt *product.FindPriceBookOption) *gorm.DB {
	if len(opt.Ids) > 0 {
		query.Where("id in ?", opt.Ids)
	}
	if len(opt.Names) > 0 {
		query.Where("name in ?", opt.Names)
	}

	if opt.StoreId > 0 {
		query.Where("store_id", opt.StoreId)
	}

	orderBy := "id asc"
	if opt.OrderBy != "" {
		orderBy = opt.OrderBy + "," + orderBy
	}
	query.Order(orderBy)

	return query
}

func (uc *PriceBookUseCase) FindManyPriceBooks(ctx context.Context, opt *product.FindPriceBookOption) types.Page[*product.PriceBook] {
	var priceBooks []*product.PriceBook
	var count int64
	query := uc.db.WithContext(ctx).Model(&product.PriceBook{})

	if opt.PageIndex == 0 {
		opt.PageIndex = 1
	}
	if opt.PageSize == 0 {
		opt.PageSize = 20
	}
	if opt.PageIndex != 0 && opt.PageSize != 0 {
		query.Offset((opt.PageIndex - 1) * opt.PageSize).Limit(opt.PageSize)
	}

	query = uc.buildFindQueryNoPage(query, opt)
	if err := query.Count(&count).Error; err != nil {
		panic(errors.Wrap(err, "find many priceBooks failed"))
	}
	if err := query.
		Debug().
		Find(&priceBooks).Error; err != nil {
		panic(errors.Wrap(err, "find many priceBooks failed"))
	}
	return types.Page[*product.PriceBook]{
		List:      priceBooks,
		PageIndex: opt.PageIndex,
		PageSize:  opt.PageSize,
		Total:     count,
	}

}

func (uc *PriceBookUseCase) FindOnePriceBook(ctx context.Context, opt *product.FindPriceBookOption) (*product.PriceBook, error) {
	var mpCustomer *product.PriceBook
	query := uc.db.WithContext(ctx).Model(&product.PriceBook{})

	query = uc.buildFindQueryNoPage(query, opt)
	if err := query.First(&mpCustomer).Error; err != nil {
		return nil, errorx.ErrRecordNotFound
	}
	return mpCustomer, nil
}

func (uc *PriceBookUseCase) CreatePriceBook(ctx context.Context, priceBook *product.PriceBook) {
	if err := uc.db.WithContext(ctx).Create(&priceBook).Error; err != nil {
		panic(err)
	}
}

func (uc *PriceBookUseCase) UpsertPriceBook(ctx context.Context, priceBook *product.PriceBook) (*product.PriceBook, error) {

	priceBooks := []*product.PriceBook{priceBook}

	_, err := uc.UpsertPriceBooks(ctx, priceBooks)
	if err != nil {
		panic(errors.Wrap(err, "upsert priceBook failed"))
	}

	return priceBook, err
}

func (uc *PriceBookUseCase) UpsertPriceBooks(ctx context.Context, priceBooks []*product.PriceBook) ([]*product.PriceBook, error) {

	err := powermodel.UpsertModelsOnUniqueID(uc.db.WithContext(ctx), &product.PriceBook{}, product.PriceBookUniqueId, priceBooks, nil)

	if err != nil {
		panic(errors.Wrap(err, "batch upsert priceBooks failed"))
	}

	return priceBooks, err
}

func (uc *PriceBookUseCase) PatchPriceBook(ctx context.Context, id int64, priceBook *product.PriceBook) {
	if err := uc.db.WithContext(ctx).Model(&product.PriceBook{}).Where(id).Updates(&priceBook).Error; err != nil {
		panic(err)
	}
}

func (uc *PriceBookUseCase) GetPriceBook(ctx context.Context, id int64) (*product.PriceBook, error) {
	var priceBook product.PriceBook
	if err := uc.db.WithContext(ctx).First(&priceBook, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errorx.WithCause(errorx.ErrBadRequest, "未找到价格手册")
		}
		panic(err)
	}
	return &priceBook, nil
}

func (uc *PriceBookUseCase) DeletePriceBook(ctx context.Context, id int64) error {
	result := uc.db.WithContext(ctx).Delete(&product.PriceBook{}, id)
	if err := result.Error; err != nil {
		panic(err)
	}
	if result.RowsAffected == 0 {
		return errorx.WithCause(errorx.ErrDeleteObjectNotFound, "未找到价格手册")
	}
	return nil
}

func (uc *PriceBookUseCase) GetStandardPriceBook(ctx context.Context) (*product.PriceBook, error) {
	var priceBook product.PriceBook
	if err := uc.db.WithContext(ctx).First(&priceBook, "is_standard", true).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errorx.WithCause(errorx.ErrNotFoundStandardPriceBook, "未找到标准价格手册")
		}
	}
	return &priceBook, nil
}
