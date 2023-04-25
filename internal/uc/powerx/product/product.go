package product

import (
	"PowerX/internal/model/powermodel"
	model "PowerX/internal/model/product"
	"PowerX/internal/types"
	"PowerX/internal/types/errorx"
	"context"
	"github.com/pkg/errors"
	"gorm.io/gorm"
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
	Types    []string
	Plans    []string
	LikeName string
	types.PageEmbedOption
}

func (uc *ProductUseCase) buildFindQueryNoPage(db *gorm.DB, opt *FindManyProductsOption) *gorm.DB {
	if len(opt.Types) > 0 {
		db = db.Where("type IN ?", opt.Types)
	}
	if len(opt.Plans) > 0 {
		db = db.Where("plan IN ?", opt.Plans)
	}
	if opt.LikeName != "" {
		db = db.Where("name LIKE ?", "%"+opt.LikeName+"%")
	}

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

	if err := db.Find(&products).Error; err != nil {
		panic(err)
	}

	if count > 0 {
		for i, product := range products {
			products[i], err = uc.LoadAssociations(product)
			if err != nil {
				return pageList, err
			}
		}
	}

	return types.Page[*model.Product]{
		List:      products,
		PageIndex: opt.PageIndex,
		PageSize:  opt.PageSize,
		Total:     count,
	}, nil
}

func (uc *ProductUseCase) CreateProduct(ctx context.Context, product *model.Product) {

	if err := uc.db.WithContext(ctx).
		Debug().
		Create(&product).Error; err != nil {
		panic(err)
	}
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

	err := powermodel.UpsertModelsOnUniqueID(uc.db.WithContext(ctx), &model.Product{}, model.ProductUniqueId, products, nil, false)

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
	if err := uc.db.WithContext(ctx).First(product, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errorx.WithCause(errorx.ErrBadRequest, "未找到产品")
		}
		panic(err)
	}
	product, err := uc.LoadAssociations(product)
	if err != nil {
		return product, err
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
	return product, err
}
