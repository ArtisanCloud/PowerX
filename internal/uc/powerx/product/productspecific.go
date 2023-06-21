package product

import (
	"PowerX/internal/model/product"
	"PowerX/internal/types"
	"PowerX/internal/types/errorx"
	"context"
	"github.com/pkg/errors"
	"gorm.io/gorm"
	"strings"
)

type ProductSpecificUseCase struct {
	db *gorm.DB
}

func NewProductSpecificUseCase(db *gorm.DB) *ProductSpecificUseCase {
	return &ProductSpecificUseCase{
		db: db,
	}
}

type FindProductSpecificOption struct {
	OrderBy   string
	Ids       []int64
	ProductId int64
	types.PageEmbedOption
}

func (uc *ProductSpecificUseCase) buildFindQueryNoPage(query *gorm.DB, opt *FindProductSpecificOption) *gorm.DB {
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

func (uc *ProductSpecificUseCase) PreloadItems(db *gorm.DB) *gorm.DB {
	db = db.
		Preload("Options")
	return db
}

func (uc *ProductSpecificUseCase) FindManyProductSpecifics(ctx context.Context, opt *FindProductSpecificOption) types.Page[*product.ProductSpecific] {
	var ProductSpecifics []*product.ProductSpecific
	var count int64
	query := uc.db.WithContext(ctx).Model(&product.ProductSpecific{})

	query = uc.buildFindQueryNoPage(query, opt)
	if err := query.Count(&count).Error; err != nil {
		panic(errors.Wrap(err, "find many ProductSpecifics failed"))
	}

	opt.DefaultPageIfNotSet()
	if opt.PageIndex != 0 && opt.PageSize != 0 {
		query.Offset((opt.PageIndex - 1) * opt.PageSize).Limit(opt.PageSize)
	}

	query = uc.PreloadItems(query)
	if err := query.
		Debug().
		Find(&ProductSpecifics).Error; err != nil {
		panic(errors.Wrap(err, "find many ProductSpecifics failed"))
	}
	return types.Page[*product.ProductSpecific]{
		List:      ProductSpecifics,
		PageIndex: opt.PageIndex,
		PageSize:  opt.PageSize,
		Total:     count,
	}

}

func (uc *ProductSpecificUseCase) FindOneProductSpecific(ctx context.Context, opt *FindProductSpecificOption) (*product.ProductSpecific, error) {
	var mpCustomer *product.ProductSpecific
	query := uc.db.WithContext(ctx).Model(&product.ProductSpecific{})

	query = uc.buildFindQueryNoPage(query, opt)
	if err := query.First(&mpCustomer).Error; err != nil {
		return nil, errorx.ErrRecordNotFound
	}
	return mpCustomer, nil
}

func (uc *ProductSpecificUseCase) CreateProductSpecific(ctx context.Context, ProductSpecific *product.ProductSpecific) error {
	if err := uc.db.WithContext(ctx).Create(&ProductSpecific).Error; err != nil {
		if strings.Contains(err.Error(), "duplicate key value violates unique constraint") {
			return errorx.WithCause(errorx.ErrDuplicatedInsert, "该对象不能重复创建")
		}
		panic(err)
	}
	return nil
}

func (uc *ProductSpecificUseCase) PatchProductSpecific(ctx context.Context, id int64, ProductSpecific *product.ProductSpecific) {
	if err := uc.db.WithContext(ctx).Model(&product.ProductSpecific{}).Where(id).Updates(&ProductSpecific).Error; err != nil {
		panic(err)
	}
}

func (uc *ProductSpecificUseCase) GetProductSpecific(ctx context.Context, id int64) (*product.ProductSpecific, error) {
	var ProductSpecific product.ProductSpecific
	if err := uc.db.WithContext(ctx).First(&ProductSpecific, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errorx.WithCause(errorx.ErrBadRequest, "未找到价格手册")
		}
		panic(err)
	}
	return &ProductSpecific, nil
}

func (uc *ProductSpecificUseCase) DeleteProductSpecific(ctx context.Context, id int64) error {
	result := uc.db.WithContext(ctx).Delete(&product.ProductSpecific{}, id)
	if err := result.Error; err != nil {
		panic(err)
	}
	if result.RowsAffected == 0 {
		return errorx.WithCause(errorx.ErrDeleteObjectNotFound, "未找到价格手册")
	}
	return nil
}
