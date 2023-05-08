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

type ArtisanUseCase struct {
	db *gorm.DB
}

func NewArtisanUseCase(db *gorm.DB) *ArtisanUseCase {
	return &ArtisanUseCase{
		db: db,
	}
}

func (uc *ArtisanUseCase) buildFindQueryNoPage(query *gorm.DB, opt *product.FindArtisanOption) *gorm.DB {
	if len(opt.Ids) > 0 {
		query.Where("id in ?", opt.Ids)
	}
	if len(opt.Names) > 0 {
		query.Where("name in ?", opt.Names)
	}

	orderBy := "id desc"
	if opt.OrderBy != "" {
		orderBy = opt.OrderBy + "," + orderBy
	}
	query.Order(orderBy)

	return query
}

func (uc *ArtisanUseCase) FindAllArtisans(ctx context.Context, opt *product.FindArtisanOption) []*product.Artisan {
	var productCategories []*product.Artisan
	query := uc.db.WithContext(ctx).Model(&product.Artisan{})

	query = uc.buildFindQueryNoPage(query, opt)
	if err := query.Find(&productCategories).Error; err != nil {
		panic(errors.Wrap(err, "find all productCategories failed"))
	}
	return productCategories
}

func (uc *ArtisanUseCase) FindManyArtisans(ctx context.Context, opt *product.FindArtisanOption) (pageList types.Page[*product.Artisan], err error) {
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

	if err := db.
		Debug().
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

func (uc *ArtisanUseCase) FindOneArtisan(ctx context.Context, opt *product.FindArtisanOption) (*product.Artisan, error) {
	var mpCustomer *product.Artisan
	query := uc.db.WithContext(ctx).Model(&product.Artisan{})

	query = uc.buildFindQueryNoPage(query, opt)
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

	productCategories := []*product.Artisan{artisan}

	_, err := uc.UpsertArtisans(ctx, productCategories)
	if err != nil {
		panic(errors.Wrap(err, "upsert artisan failed"))
	}

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
