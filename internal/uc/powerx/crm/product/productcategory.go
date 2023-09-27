package product

import (
	"PowerX/internal/model/powermodel"
	"PowerX/internal/model/product"
	"PowerX/internal/types/errorx"
	"context"
	"github.com/pkg/errors"
	"gorm.io/gorm"
	"strings"
)

type ProductCategoryUseCase struct {
	db *gorm.DB
}

func NewProductCategoryUseCase(db *gorm.DB) *ProductCategoryUseCase {
	return &ProductCategoryUseCase{
		db: db,
	}
}

type FindProductCategoryOption struct {
	OrderBy     string
	CategoryPId int
	Limit       int
	Ids         []int64
	Names       []string
}

func (uc *ProductCategoryUseCase) buildFindQueryNoPage(query *gorm.DB, opt *FindProductCategoryOption) *gorm.DB {
	if len(opt.Ids) > 0 {
		query.Where("id in ?", opt.Ids)
	}
	if len(opt.Names) > 0 {
		query.Where("name in ?", opt.Names)
	}
	if opt.Limit > 0 {
		query.Limit(opt.Limit)
	}

	orderBy := "sort desc, id "
	if opt.OrderBy != "" {
		orderBy = opt.OrderBy + "," + orderBy
	}
	query.Order(orderBy)

	return query
}

func (uc *ProductCategoryUseCase) PreloadItems(db *gorm.DB) *gorm.DB {
	db = db.
		Preload("CoverImage")

	return db
}

func (uc *ProductCategoryUseCase) ListProductCategoryTree(ctx context.Context, opt *FindProductCategoryOption, pId int64) []*product.ProductCategory {
	if pId < 0 {
		panic(errors.New("find productCategories pId invalid"))
	}

	var categories []*product.ProductCategory

	query := uc.db.WithContext(ctx).Model(&product.ProductCategory{})
	query = uc.buildFindQueryNoPage(query, opt)

	query = uc.PreloadItems(query)
	err := query.
		Where("p_id", pId).
		//Debug().
		Find(&categories).
		Error
	if err != nil {
		panic(errors.Wrap(err, "find all productCategories failed"))
	}
	var children []*product.ProductCategory
	for i, category := range categories {

		children = uc.ListProductCategoryTree(ctx, opt, category.Id)

		if len(children) > 0 {
			categories[i].Children = children
		}
	}
	return categories
}

func (uc *ProductCategoryUseCase) FindProductCategoriesByParentId(ctx context.Context, opt *FindProductCategoryOption) []*product.ProductCategory {
	if opt.CategoryPId < 0 {
		panic(errors.New("find productCategories pId invalid"))
	}

	var productCategories []*product.ProductCategory
	query := uc.db.WithContext(ctx).Model(&product.ProductCategory{})

	query = uc.buildFindQueryNoPage(query, opt)

	query = uc.PreloadItems(query)
	if err := query.
		Where("p_id", opt.CategoryPId).
		Find(&productCategories).Error; err != nil {
		panic(errors.Wrap(err, "find all productCategories failed"))
	}
	return productCategories
}

func (uc *ProductCategoryUseCase) FindAllProductCategories(ctx context.Context, opt *FindProductCategoryOption) []*product.ProductCategory {

	var productCategories []*product.ProductCategory
	query := uc.db.WithContext(ctx).Model(&product.ProductCategory{})

	query = uc.buildFindQueryNoPage(query, opt)

	if err := query.
		Find(&productCategories).Error; err != nil {
		panic(errors.Wrap(err, "find all productCategories failed"))
	}
	return productCategories
}

func (uc *ProductCategoryUseCase) FindOneProductCategory(ctx context.Context, opt *FindProductCategoryOption) (*product.ProductCategory, error) {
	var mpCustomer *product.ProductCategory
	query := uc.db.WithContext(ctx).Model(&product.ProductCategory{})

	query = uc.buildFindQueryNoPage(query, opt)
	if err := query.First(&mpCustomer).Error; err != nil {
		return nil, errorx.ErrRecordNotFound
	}
	return mpCustomer, nil
}

func (uc *ProductCategoryUseCase) CreateProductCategory(ctx context.Context, productCategory *product.ProductCategory) error {
	if err := uc.db.WithContext(ctx).Create(&productCategory).Error; err != nil {
		if strings.Contains(err.Error(), "duplicate key value violates unique constraint") {
			return errorx.WithCause(errorx.ErrDuplicatedInsert, "该对象不能重复创建")
		}
		panic(err)
	}
	return nil
}

func (uc *ProductCategoryUseCase) UpsertProductCategory(ctx context.Context, productCategory *product.ProductCategory) (*product.ProductCategory, error) {

	// 查询父节点
	if productCategory.PId > 0 {
		var pProductCategory *product.ProductCategory
		err := uc.db.WithContext(ctx).
			Where(productCategory.PId).First(&pProductCategory).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, errorx.WithCause(errorx.ErrBadRequest, "父品类不存在")
			}
			panic(errors.Wrap(err, "query parent product category failed"))
		}
	} else if productCategory.PId < 0 {
		panic(errors.New("query parent product category in invalid"))
	}

	productCategories := []*product.ProductCategory{productCategory}

	_, err := uc.UpsertProductCategories(ctx, productCategories)
	if err != nil {
		panic(errors.Wrap(err, "upsert productCategory failed"))
	}

	return productCategory, err
}

func (uc *ProductCategoryUseCase) UpsertProductCategories(ctx context.Context, productCategories []*product.ProductCategory) ([]*product.ProductCategory, error) {

	err := powermodel.UpsertModelsOnUniqueID(uc.db.WithContext(ctx), &product.ProductCategory{}, product.ProductCategoryUniqueId, productCategories, nil, false)

	if err != nil {
		panic(errors.Wrap(err, "batch upsert product categories failed"))
	}

	return productCategories, err
}

func (uc *ProductCategoryUseCase) PatchProductCategory(ctx context.Context, id int64, productCategory *product.ProductCategory) {
	if err := uc.db.WithContext(ctx).Model(&product.ProductCategory{}).Where(id).Updates(productCategory).Error; err != nil {
		panic(err)
	}
}

func (uc *ProductCategoryUseCase) GetProductCategory(ctx context.Context, id int64) (*product.ProductCategory, error) {
	var productCategory product.ProductCategory
	db := uc.db.WithContext(ctx)
	db = uc.PreloadItems(db)
	if err := db.
		//Debug().
		First(&productCategory, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errorx.WithCause(errorx.ErrBadRequest, "未找到产品品类")
		}
		panic(err)
	}

	_ = productCategory.LoadChildren(db, nil, false)

	return &productCategory, nil
}

func (uc *ProductCategoryUseCase) DeleteProductCategory(ctx context.Context, id int64) error {
	result := uc.db.WithContext(ctx).Delete(&product.ProductCategory{}, id)
	if err := result.Error; err != nil {
		panic(err)
	}
	if result.RowsAffected == 0 {
		return errorx.WithCause(errorx.ErrDeleteObjectNotFound, "未找到产品品类")
	}
	return nil
}
