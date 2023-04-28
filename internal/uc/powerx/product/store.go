package product

import (
	"PowerX/internal/model/powermodel"
	model "PowerX/internal/model/product"
	"PowerX/internal/types"
	"PowerX/internal/types/errorx"
	"context"
	"github.com/pkg/errors"
	"gorm.io/gorm"
	"strings"
)

type StoreUseCase struct {
	db *gorm.DB
}

func NewStoreUseCase(db *gorm.DB) *StoreUseCase {
	return &StoreUseCase{
		db: db,
	}
}

type FindManyStoresOption struct {
	Types    []string
	Plans    []string
	LikeName string
	types.PageEmbedOption
}

func (uc *StoreUseCase) buildFindQueryNoPage(db *gorm.DB, opt *FindManyStoresOption) *gorm.DB {
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

func (uc *StoreUseCase) FindAllShops(ctx context.Context, opt *FindManyStoresOption) (dictionaryItems []*model.Store, err error) {
	query := uc.db.WithContext(ctx).Model(&model.Store{})

	query = uc.buildFindQueryNoPage(query, opt)
	if err := query.
		Debug().
		Preload("Artisans").
		Find(&dictionaryItems).Error; err != nil {
		panic(errors.Wrap(err, "find all dictionaryItems failed"))
	}
	return dictionaryItems, err
}

func (uc *StoreUseCase) FindManyStores(ctx context.Context, opt *FindManyStoresOption) (pageList types.Page[*model.Store], err error) {
	opt.DefaultPageIfNotSet()
	var products []*model.Store
	db := uc.db.WithContext(ctx).Model(&model.Store{})

	db = uc.buildFindQueryNoPage(db, opt)

	var count int64
	if err := db.Count(&count).Error; err != nil {
		panic(err)
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

	return types.Page[*model.Store]{
		List:      products,
		PageIndex: opt.PageIndex,
		PageSize:  opt.PageSize,
		Total:     count,
	}, nil
}

func (uc *StoreUseCase) CreateStore(ctx context.Context, product *model.Store) error {

	if err := uc.db.WithContext(ctx).
		Debug().
		Create(&product).Error; err != nil {
		if strings.Contains(err.Error(), "duplicate key value violates unique constraint") {
			return errorx.WithCause(errorx.ErrDuplicatedInsert, "该对象不能重复创建")
		}
		panic(err)
	}
	return nil
}

func (uc *StoreUseCase) UpsertStore(ctx context.Context, product *model.Store) (*model.Store, error) {

	products := []*model.Store{product}

	err := uc.db.Transaction(func(tx *gorm.DB) error {
		// 删除产品的相关联对象
		_, err := uc.ClearAssociations(tx, product)
		if err != nil {
			return err
		}

		// 更新产品对象主体
		_, err = uc.UpsertStores(ctx, products)
		if err != nil {
			return errors.Wrap(err, "upsert product failed")
		}

		return err
	})

	return product, err
}

func (uc *StoreUseCase) UpsertStores(ctx context.Context, products []*model.Store) ([]*model.Store, error) {

	err := powermodel.UpsertModelsOnUniqueID(uc.db.WithContext(ctx), &model.Store{}, model.StoreUniqueId, products, nil, false)

	if err != nil {
		panic(errors.Wrap(err, "batch upsert products failed"))
	}

	return products, err
}

func (uc *StoreUseCase) PatchStore(ctx context.Context, id int64, product *model.Store) {
	if err := uc.db.WithContext(ctx).Model(&model.Store{}).
		Where(id).Updates(&product).Error; err != nil {
		panic(err)
	}
}

func (uc *StoreUseCase) GetStore(ctx context.Context, id int64) (*model.Store, error) {
	var product = &model.Store{}
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

func (uc *StoreUseCase) DeleteStore(ctx context.Context, id int64) error {

	// 获取产品相关项
	product, err := uc.GetStore(ctx, id)
	if err != nil {
		return errorx.ErrNotFoundObject
	}

	err = uc.db.Transaction(func(tx *gorm.DB) error {
		// 删除产品相关项
		_, err = uc.ClearAssociations(tx, product)
		if err != nil {
			return err
		}

		result := tx.Delete(&model.Store{}, id)
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

func (uc *StoreUseCase) LoadAssociations(store *model.Store) (*model.Store, error) {
	var err error

	err = store.LoadArtisans(uc.db, nil, false)
	if err != nil {
		return nil, err
	}

	return store, err

}

func (uc *StoreUseCase) ClearAssociations(db *gorm.DB, store *model.Store) (*model.Store, error) {
	var err error
	// 清除元匠的关联
	err = store.ClearArtisans(db)
	if err != nil {
		return nil, err
	}

	return store, err
}
