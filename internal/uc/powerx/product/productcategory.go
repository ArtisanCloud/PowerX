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

type ProductCategoryUseCase struct {
	db *gorm.DB
}

func NewProductCategoryUseCase(db *gorm.DB) *ProductCategoryUseCase {
	return &ProductCategoryUseCase{
		db: db,
	}
}

func (uc *ProductCategoryUseCase) buildFindQueryNoPage(query *gorm.DB, opt *product.FindProductCategoryOption) *gorm.DB {
	if len(opt.Ids) > 0 {
		query.Where("id in ?", opt.Ids)
	}
	if len(opt.Names) > 0 {
		query.Where("name in ?", opt.Names)
	}

	return query
}

func (uc *ProductCategoryUseCase) FindManyProductCategories(ctx context.Context, opt *product.FindProductCategoryOption) types.Page[*product.ProductCategory] {
	var productCategories []*product.ProductCategory
	var count int64
	query := uc.db.WithContext(ctx).Model(&product.ProductCategory{})

	if opt.PageIndex != 0 && opt.PageSize != 0 {
		query.Offset((opt.PageIndex - 1) * opt.PageSize).Limit(opt.PageSize)
	}
	query = uc.buildFindQueryNoPage(query, opt)
	if err := query.Count(&count).Error; err != nil {
		panic(errors.Wrap(err, "find productCategories failed"))
	}
	if err := query.Find(&productCategories).Error; err != nil {
		panic(errors.Wrap(err, "find productCategories failed"))
	}
	return types.Page[*product.ProductCategory]{
		List:  productCategories,
		Total: count,
	}
}

func (uc *ProductCategoryUseCase) FindOneMPCustomer(ctx context.Context, opt *product.FindProductCategoryOption) (*product.ProductCategory, error) {
	var mpCustomer *product.ProductCategory
	query := uc.db.WithContext(ctx).Model(&product.ProductCategory{})
	if opt.PageIndex != 0 && opt.PageSize != 0 {
		query.Offset((opt.PageIndex - 1) * opt.PageSize).Limit(opt.PageSize)
	}
	query = uc.buildFindQueryNoPage(query, opt)
	if err := query.First(&mpCustomer).Error; err != nil {
		return nil, errorx.ErrRecordNotFound
	}
	return mpCustomer, nil
}

func (uc *ProductCategoryUseCase) CreateProductCategory(ctx context.Context, productCategory *product.ProductCategory) {
	if err := uc.db.WithContext(ctx).Create(&productCategory).Error; err != nil {
		panic(err)
	}
}

func (uc *ProductCategoryUseCase) UpsertProductCategory(ctx context.Context, productCategory *product.ProductCategory) (*product.ProductCategory, error) {

	productCategorys := []*product.ProductCategory{productCategory}

	_, err := uc.UpsertProductCategories(ctx, productCategorys)
	if err != nil {
		panic(errors.Wrap(err, "upsert productCategory failed"))
	}

	return productCategory, err
}

func (uc *ProductCategoryUseCase) UpsertProductCategories(ctx context.Context, productCategorys []*product.ProductCategory) ([]*product.ProductCategory, error) {

	err := powermodel.UpsertModelsOnUniqueID(uc.db.WithContext(ctx), &product.ProductCategory{}, product.ProductCategoryUniqueId, productCategorys, nil)

	if err != nil {
		panic(errors.Wrap(err, "batch upsert product categories failed"))
	}

	return productCategorys, err
}

func (uc *ProductCategoryUseCase) PatchProductCategory(ctx context.Context, id int64, productCategory *product.ProductCategory) {
	if err := uc.db.WithContext(ctx).Model(&product.ProductCategory{}).Where(id).Updates(&productCategory).Error; err != nil {
		panic(err)
	}
}

func (uc *ProductCategoryUseCase) GetProductCategory(ctx context.Context, id int64) (*product.ProductCategory, error) {
	var productCategory product.ProductCategory
	if err := uc.db.WithContext(ctx).First(&productCategory, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errorx.WithCause(errorx.ErrBadRequest, "未找到产品品类")
		}
		panic(err)
	}
	return &productCategory, nil
}

func (uc *ProductCategoryUseCase) DeleteProductCategory(ctx context.Context, id int64) error {
	result := uc.db.WithContext(ctx).Delete(&product.ProductCategory{}, id)
	if err := result.Error; err != nil {
		panic(err)
	}
	if result.RowsAffected == 0 {
		return errorx.WithCause(errorx.ErrBadRequest, "未找到产品品类")
	}
	return nil
}
