package product

import (
	"PowerX/internal/model/custom/product"
	"PowerX/internal/model/powermodel"
	"PowerX/internal/types/errorx"
	"context"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

type ArtisanSpecificUseCase struct {
	db *gorm.DB
}

func NewArtisanSpecificUseCase(db *gorm.DB) *ArtisanSpecificUseCase {
	return &ArtisanSpecificUseCase{
		db: db,
	}
}

type FindArtisanSpecificOption struct {
	OrderBy string
	Ids     []int64
	Names   []string
}

func (uc *ArtisanSpecificUseCase) buildFindQueryNoPage(query *gorm.DB, opt *FindArtisanSpecificOption) *gorm.DB {
	if len(opt.Ids) > 0 {
		query.Where("id in ?", opt.Ids)
	}
	if len(opt.Names) > 0 {
		query.Where("name in ?", opt.Names)
	}

	orderBy := "id asc"
	if opt.OrderBy != "" {
		orderBy = opt.OrderBy + "," + orderBy
	}
	query.Order(orderBy)

	return query
}

func (uc *ArtisanSpecificUseCase) FindAllArtisanSpecifics(ctx context.Context, opt *FindArtisanSpecificOption) []*product.ArtisanSpecific {
	var productCategories []*product.ArtisanSpecific
	query := uc.db.WithContext(ctx).Model(&product.ArtisanSpecific{})

	query = uc.buildFindQueryNoPage(query, opt)
	if err := query.Find(&productCategories).Error; err != nil {
		panic(errors.Wrap(err, "find all productCategories failed"))
	}
	return productCategories
}

func (uc *ArtisanSpecificUseCase) FindOneArtisanSpecific(ctx context.Context, opt *FindArtisanSpecificOption) (*product.ArtisanSpecific, error) {
	var mpCustomer *product.ArtisanSpecific
	query := uc.db.WithContext(ctx).Model(&product.ArtisanSpecific{})

	query = uc.buildFindQueryNoPage(query, opt)
	if err := query.First(&mpCustomer).Error; err != nil {
		return nil, errorx.ErrRecordNotFound
	}
	return mpCustomer, nil
}

func (uc *ArtisanSpecificUseCase) CreateArtisanSpecific(ctx context.Context, artisan *product.ArtisanSpecific) {
	if err := uc.db.WithContext(ctx).Create(&artisan).Error; err != nil {
		panic(err)
	}
}

func (uc *ArtisanSpecificUseCase) UpsertArtisanSpecific(ctx context.Context, artisan *product.ArtisanSpecific) (*product.ArtisanSpecific, error) {

	productCategories := []*product.ArtisanSpecific{artisan}

	_, err := uc.UpsertArtisanSpecifics(ctx, productCategories)
	if err != nil {
		panic(errors.Wrap(err, "upsert artisan failed"))
	}

	return artisan, err
}

func (uc *ArtisanSpecificUseCase) UpsertArtisanSpecifics(ctx context.Context, productCategories []*product.ArtisanSpecific) ([]*product.ArtisanSpecific, error) {

	err := powermodel.UpsertModelsOnUniqueID(uc.db.WithContext(ctx), &product.ArtisanSpecific{}, product.ArtisanSpecificUniqueId, productCategories, nil, false)

	if err != nil {
		panic(errors.Wrap(err, "batch upsert product categories failed"))
	}

	return productCategories, err
}

func (uc *ArtisanSpecificUseCase) PatchArtisanSpecific(ctx context.Context, id int64, artisan *product.ArtisanSpecific) {
	if err := uc.db.WithContext(ctx).Model(&product.ArtisanSpecific{}).Where(id).Updates(artisan).Error; err != nil {
		panic(err)
	}
}

func (uc *ArtisanSpecificUseCase) GetArtisanSpecific(ctx context.Context, id int64) (*product.ArtisanSpecific, error) {
	var artisan product.ArtisanSpecific
	if err := uc.db.WithContext(ctx).First(&artisan, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errorx.WithCause(errorx.ErrBadRequest, "未找到元匠")
		}
		panic(err)
	}
	return &artisan, nil
}

func (uc *ArtisanSpecificUseCase) DeleteArtisanSpecific(ctx context.Context, id int64) error {
	result := uc.db.WithContext(ctx).Delete(&product.ArtisanSpecific{}, id)
	if err := result.Error; err != nil {
		panic(err)
	}
	if result.RowsAffected == 0 {
		return errorx.WithCause(errorx.ErrDeleteObjectNotFound, "未找到元匠")
	}
	return nil
}
