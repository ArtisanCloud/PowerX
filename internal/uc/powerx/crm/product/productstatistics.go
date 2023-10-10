package product

import (
	"PowerX/internal/model/crm/product"
	"PowerX/internal/model/powermodel"
	"PowerX/internal/types"
	"PowerX/internal/types/errorx"
	"context"
	"github.com/pkg/errors"
	"gorm.io/gorm"
	"strings"
)

type ProductStatisticsUseCase struct {
	db *gorm.DB
}

func NewProductStatisticsUseCase(db *gorm.DB) *ProductStatisticsUseCase {
	return &ProductStatisticsUseCase{
		db: db,
	}
}

type FindProductStatisticsOption struct {
	OrderBy   string
	Ids       []int64
	ProductId int64
	types.PageEmbedOption
}

func (uc *ProductStatisticsUseCase) buildFindQueryNoPage(query *gorm.DB, opt *FindProductStatisticsOption) *gorm.DB {
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

func (uc *ProductStatisticsUseCase) PreloadItems(db *gorm.DB) *gorm.DB {
	db = db.
		Preload("Options")
	return db
}

func (uc *ProductStatisticsUseCase) FindManyProductStatistics(ctx context.Context, opt *FindProductStatisticsOption) types.Page[*product.ProductStatistics] {
	var ProductStatistics []*product.ProductStatistics
	var count int64
	query := uc.db.WithContext(ctx).Model(&product.ProductStatistics{})

	query = uc.buildFindQueryNoPage(query, opt)
	if err := query.Count(&count).Error; err != nil {
		panic(errors.Wrap(err, "find many ProductStatistics failed"))
	}

	opt.DefaultPageIfNotSet()
	if opt.PageIndex != 0 && opt.PageSize != 0 {
		query.Offset((opt.PageIndex - 1) * opt.PageSize).Limit(opt.PageSize)
	}

	query = uc.PreloadItems(query)
	if err := query.
		//Debug().
		Find(&ProductStatistics).Error; err != nil {
		panic(errors.Wrap(err, "find many ProductStatistics failed"))
	}
	return types.Page[*product.ProductStatistics]{
		List:      ProductStatistics,
		PageIndex: opt.PageIndex,
		PageSize:  opt.PageSize,
		Total:     count,
	}

}

func (uc *ProductStatisticsUseCase) FindOneProductStatistics(ctx context.Context, opt *FindProductStatisticsOption) (*product.ProductStatistics, error) {
	var mpCustomer *product.ProductStatistics
	query := uc.db.WithContext(ctx).Model(&product.ProductStatistics{})

	query = uc.buildFindQueryNoPage(query, opt)
	if err := query.First(&mpCustomer).Error; err != nil {
		return nil, errorx.ErrRecordNotFound
	}
	return mpCustomer, nil
}

func (uc *ProductStatisticsUseCase) CreateProductStatistics(ctx context.Context, ProductStatistics *product.ProductStatistics) error {
	if err := uc.db.WithContext(ctx).Create(&ProductStatistics).Error; err != nil {
		if strings.Contains(err.Error(), "duplicate key value violates unique constraint") {
			return errorx.WithCause(errorx.ErrDuplicatedInsert, "该对象不能重复创建")
		}
		panic(err)
	}
	return nil
}

func (uc *ProductStatisticsUseCase) UpsertProductStatistics(ctx context.Context, productStatistics *product.ProductStatistics) (*product.ProductStatistics, error) {

	productStatisticses := []*product.ProductStatistics{productStatistics}

	_, err := uc.UpsertProductStatisticses(ctx, productStatisticses)
	if err != nil {
		panic(errors.Wrap(err, "upsert productStatistics failed"))
	}

	return productStatistics, err
}

func (uc *ProductStatisticsUseCase) UpsertProductStatisticses(ctx context.Context, productStatisticses []*product.ProductStatistics) ([]*product.ProductStatistics, error) {

	err := powermodel.UpsertModelsOnUniqueID(uc.db.WithContext(ctx), &product.ProductStatistics{}, product.ProductStatisticsUniqueId, productStatisticses, nil, true)

	if err != nil {
		panic(errors.Wrap(err, "batch upsert productStatistics failed"))
	}

	return productStatisticses, err
}

func (uc *ProductStatisticsUseCase) PatchProductStatistics(ctx context.Context, id int64, ProductStatistics *product.ProductStatistics) {
	if err := uc.db.WithContext(ctx).Model(&product.ProductStatistics{}).Where(id).Updates(&ProductStatistics).Error; err != nil {
		panic(err)
	}
}

func (uc *ProductStatisticsUseCase) GetProductStatistics(ctx context.Context, id int64) (*product.ProductStatistics, error) {
	var ProductStatistics product.ProductStatistics
	if err := uc.db.WithContext(ctx).First(&ProductStatistics, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errorx.WithCause(errorx.ErrBadRequest, "未找到产品统计记录")
		}
		panic(err)
	}
	return &ProductStatistics, nil
}

func (uc *ProductStatisticsUseCase) GetProductStatisticsByProductId(ctx context.Context, productId int64) (*product.ProductStatistics, error) {
	var ProductStatistics product.ProductStatistics
	if err := uc.db.WithContext(ctx).
		Where("product_id", productId).
		Debug().
		FirstOrCreate(&ProductStatistics).
		Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errorx.WithCause(errorx.ErrBadRequest, "未找到产品统计记录")
		}
		panic(err)
	}
	return &ProductStatistics, nil
}

func (uc *ProductStatisticsUseCase) DeleteProductStatistics(ctx context.Context, id int64) error {
	result := uc.db.WithContext(ctx).Delete(&product.ProductStatistics{}, id)
	if err := result.Error; err != nil {
		panic(err)
	}
	if result.RowsAffected == 0 {
		return errorx.WithCause(errorx.ErrDeleteObjectNotFound, "未找到产品统计记录")
	}
	return nil
}
