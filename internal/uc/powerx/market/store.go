package market

import (
	model "PowerX/internal/model/market"
	"PowerX/internal/model/media"
	"PowerX/internal/model/powermodel"
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
	LikeName string
	Ids      []int64
	OrderBy  string
	types.PageEmbedOption
}

func (uc *StoreUseCase) buildFindQueryNoPage(db *gorm.DB, opt *FindManyStoresOption) *gorm.DB {

	if opt.LikeName != "" {
		db = db.Where("name LIKE ?", "%"+opt.LikeName+"%")
	}

	if len(opt.Ids) > 0 {
		db = db.Where("id in ?", opt.Ids)
	}

	orderBy := "id desc"
	if opt.OrderBy != "" {
		orderBy = opt.OrderBy + "," + orderBy
	}
	db.Order(orderBy)

	return db
}

func (uc *StoreUseCase) PreloadItems(db *gorm.DB) *gorm.DB {
	db = db.
		Preload("PivotDetailImages", "media_usage = ?", media.MediaUsageDetail).Preload("PivotDetailImages.MediaResource").
		Preload("CoverImage")

	return db
}

func (uc *StoreUseCase) FindAllShops(ctx context.Context, opt *FindManyStoresOption) (dictionaryItems []*model.Store, err error) {
	query := uc.db.WithContext(ctx).Model(&model.Store{})

	query = uc.buildFindQueryNoPage(query, opt)

	query = uc.PreloadItems(query)
	if err := query.
		//Debug().
		Find(&dictionaryItems).Error; err != nil {
		panic(errors.Wrap(err, "find all dictionaryItems failed"))
	}
	return dictionaryItems, err
}

func (uc *StoreUseCase) FindManyStores(ctx context.Context, opt *FindManyStoresOption) (pageList types.Page[*model.Store], err error) {

	var stores []*model.Store
	db := uc.db.WithContext(ctx).Model(&model.Store{})

	db = uc.buildFindQueryNoPage(db, opt)

	var count int64
	if err := db.
		//Debug().
		Count(&count).
		Error; err != nil {
		panic(err)
	}

	opt.DefaultPageIfNotSet()
	if opt.PageIndex != 0 && opt.PageSize != 0 {
		db.Offset((opt.PageIndex - 1) * opt.PageSize).Limit(opt.PageSize)
	}

	db = uc.PreloadItems(db)
	if err := db.
		Find(&stores).Error; err != nil {
		panic(err)
	}

	if count > 0 {
		for i, store := range stores {
			stores[i], err = uc.LoadAssociations(store)
			if err != nil {
				return pageList, err
			}
		}
	}

	return types.Page[*model.Store]{
		List:      stores,
		PageIndex: opt.PageIndex,
		PageSize:  opt.PageSize,
		Total:     count,
	}, nil
}

func (uc *StoreUseCase) CreateStore(ctx context.Context, store *model.Store) error {

	if err := uc.db.WithContext(ctx).
		//Debug().
		Create(&store).Error; err != nil {
		if strings.Contains(err.Error(), "duplicate key value violates unique constraint") {
			return errorx.WithCause(errorx.ErrDuplicatedInsert, "该对象不能重复创建")
		}
		panic(err)
	}
	return nil
}

func (uc *StoreUseCase) UpsertStore(ctx context.Context, store *model.Store) (*model.Store, error) {

	stores := []*model.Store{store}

	err := uc.db.Transaction(func(tx *gorm.DB) error {
		// 删除产品的相关联对象
		_, err := uc.ClearAssociations(tx, store)
		if err != nil {
			return err
		}

		// 更新产品对象主体
		_, err = uc.UpsertStores(ctx, stores)
		if err != nil {
			return errors.Wrap(err, "upsert store failed")
		}

		return err
	})

	return store, err
}

func (uc *StoreUseCase) UpsertStores(ctx context.Context, stores []*model.Store) ([]*model.Store, error) {

	err := powermodel.UpsertModelsOnUniqueID(uc.db.WithContext(ctx), &model.Store{}, model.StoreUniqueId, stores, nil, false)

	if err != nil {
		panic(errors.Wrap(err, "batch upsert stores failed"))
	}

	return stores, err
}

func (uc *StoreUseCase) PatchStore(ctx context.Context, id int64, store *model.Store) {
	if err := uc.db.WithContext(ctx).Model(&model.Store{}).
		Where(id).Updates(&store).Error; err != nil {
		panic(err)
	}
}

func (uc *StoreUseCase) GetStore(ctx context.Context, id int64) (*model.Store, error) {
	var store = &model.Store{}
	if err := uc.db.WithContext(ctx).First(store, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errorx.WithCause(errorx.ErrBadRequest, "未找到产品")
		}
		panic(err)
	}
	store, err := uc.LoadAssociations(store)
	if err != nil {
		return store, err
	}

	return store, nil
}

func (uc *StoreUseCase) DeleteStore(ctx context.Context, id int64) error {

	// 获取产品相关项
	store, err := uc.GetStore(ctx, id)
	if err != nil {
		return errorx.ErrNotFoundObject
	}

	err = uc.db.Transaction(func(tx *gorm.DB) error {
		// 删除产品相关项
		_, err = uc.ClearAssociations(tx, store)
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

	// 清除产品详细图片记录
	err = store.ClearPivotDetailImages(db)
	if err != nil {
		return nil, err
	}

	return store, err
}
