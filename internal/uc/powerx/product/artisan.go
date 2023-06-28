package product

import (
	"PowerX/internal/model/media"
	"PowerX/internal/model/powermodel"
	"PowerX/internal/model/product"
	"PowerX/internal/types"
	"PowerX/internal/types/errorx"
	"context"
	"github.com/pkg/errors"
	"gorm.io/gorm"
	"strings"
)

type ArtisanUseCase struct {
	db *gorm.DB
}

func NewArtisanUseCase(db *gorm.DB) *ArtisanUseCase {
	return &ArtisanUseCase{
		db: db,
	}
}

type FindManyArtisanOption struct {
	LikeName string
	OrderBy  string
	Ids      []int64
	StoreIds []int64
	Names    []string
	types.PageEmbedOption
}

func (uc *ArtisanUseCase) buildFindQueryNoPage(query *gorm.DB, opt *FindManyArtisanOption) *gorm.DB {
	if len(opt.Ids) > 0 {
		query.Where("id in ?", opt.Ids)
	}
	if len(opt.Names) > 0 {
		query.Where("name in ?", opt.Names)
	}

	if len(opt.StoreIds) > 0 {
		query.Joins("LEFT JOIN pivot_store_to_artisan ON artisans.id = pivot_store_to_artisan.artisan_id").
			Where("pivot_store_to_artisan.store_id in (?)", opt.StoreIds)
	}

	orderBy := "id desc"
	if opt.OrderBy != "" {
		orderBy = opt.OrderBy + "," + orderBy
	}
	query.Order(orderBy)

	return query
}

func (uc *ArtisanUseCase) PreloadItems(db *gorm.DB) *gorm.DB {
	db = db.
		Preload("PivotDetailImages", "media_usage = ?", media.MediaUsageDetail).Preload("PivotDetailImages.MediaResource").
		Preload("CoverImage").
		Preload("PivotStoreToArtisans")

	return db
}

func (uc *ArtisanUseCase) FindAllArtisans(ctx context.Context, opt *FindManyArtisanOption) []*product.Artisan {
	var productCategories []*product.Artisan
	query := uc.db.WithContext(ctx).Model(&product.Artisan{})

	query = uc.buildFindQueryNoPage(query, opt)
	query = uc.PreloadItems(query)
	if err := query.
		Find(&productCategories).Error; err != nil {
		panic(errors.Wrap(err, "find all productCategories failed"))
	}
	return productCategories
}

func (uc *ArtisanUseCase) FindManyArtisans(ctx context.Context, opt *FindManyArtisanOption) (pageList types.Page[*product.Artisan], err error) {
	var artisans []*product.Artisan
	db := uc.db.WithContext(ctx).Model(&product.Artisan{})

	db = uc.buildFindQueryNoPage(db, opt)

	var count int64
	if err := db.Count(&count).Error; err != nil {
		panic(err)
	}

	opt.DefaultPageIfNotSet()
	if opt.PageIndex != 0 && opt.PageSize != 0 {
		db.Offset((opt.PageIndex - 1) * opt.PageSize).Limit(opt.PageSize)
	}

	db = uc.PreloadItems(db)
	if err := db.
		//Debug().
		//Preload("ArtisanSpecific").
		Find(&artisans).Error; err != nil {
		panic(err)
	}

	return types.Page[*product.Artisan]{
		List:      artisans,
		PageIndex: opt.PageIndex,
		PageSize:  opt.PageSize,
		Total:     count,
	}, nil
}

func (uc *ArtisanUseCase) FindOneArtisan(ctx context.Context, opt *FindManyArtisanOption) (*product.Artisan, error) {
	var mpCustomer *product.Artisan
	query := uc.db.WithContext(ctx).Model(&product.Artisan{})

	query = uc.buildFindQueryNoPage(query, opt)
	query = uc.PreloadItems(query)
	if err := query.First(&mpCustomer).Error; err != nil {
		return nil, errorx.ErrRecordNotFound
	}
	return mpCustomer, nil
}

func (uc *ArtisanUseCase) CreateArtisan(ctx context.Context, artisan *product.Artisan) error {
	if err := uc.db.WithContext(ctx).Create(&artisan).Error; err != nil {
		if strings.Contains(err.Error(), "duplicate key value violates unique constraint") {
			return errorx.WithCause(errorx.ErrDuplicatedInsert, "该对象不能重复创建")
		}
		panic(err)
	}
	return nil
}

func (uc *ArtisanUseCase) UpsertArtisan(ctx context.Context, artisan *product.Artisan) (*product.Artisan, error) {

	artisans := []*product.Artisan{artisan}

	err := uc.db.Transaction(func(tx *gorm.DB) error {
		// 删除产品的相关联对象
		_, err := uc.ClearAssociations(tx, artisan)
		if err != nil {
			return err
		}

		// 更新产品对象主体
		_, err = uc.UpsertArtisans(ctx, artisans)
		if err != nil {
			return errors.Wrap(err, "upsert store failed")
		}

		return err
	})

	return artisan, err
}

func (uc *ArtisanUseCase) UpsertArtisans(ctx context.Context, productCategories []*product.Artisan) ([]*product.Artisan, error) {

	err := powermodel.UpsertModelsOnUniqueID(uc.db.WithContext(ctx), &product.Artisan{}, product.ArtisanUniqueId, productCategories, nil, false)

	if err != nil {
		panic(errors.Wrap(err, "batch upsert product categories failed"))
	}

	return productCategories, err
}

func (uc *ArtisanUseCase) PatchArtisan(ctx context.Context, id int64, artisan *product.Artisan) {
	if err := uc.db.WithContext(ctx).Model(&product.Artisan{}).Where(id).Updates(artisan).Error; err != nil {
		panic(err)
	}
}

func (uc *ArtisanUseCase) GetArtisan(ctx context.Context, id int64) (*product.Artisan, error) {
	var artisan product.Artisan
	if err := uc.db.WithContext(ctx).First(&artisan, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errorx.WithCause(errorx.ErrBadRequest, "未找到元匠")
		}
		panic(err)
	}
	return &artisan, nil
}

func (uc *ArtisanUseCase) DeleteArtisan(ctx context.Context, id int64) error {
	result := uc.db.WithContext(ctx).Delete(&product.Artisan{}, id)
	if err := result.Error; err != nil {
		panic(err)
	}
	if result.RowsAffected == 0 {
		return errorx.WithCause(errorx.ErrDeleteObjectNotFound, "未找到元匠")
	}
	return nil
}

func (uc *ArtisanUseCase) ClearAssociations(db *gorm.DB, artisan *product.Artisan) (*product.Artisan, error) {
	var err error

	// 清除产品详细图片记录
	err = artisan.ClearPivotDetailImages(db)
	if err != nil {
		return nil, err
	}

	return artisan, err
}
