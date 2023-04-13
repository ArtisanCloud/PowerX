package reservationcenter

import (
	"PowerX/internal/model/custom/reservationcenter"
	"PowerX/internal/model/powermodel"
	"PowerX/internal/types/errorx"
	"context"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

type ArtisanUseCase struct {
	db *gorm.DB
}

func NewArtisanUseCase(db *gorm.DB) *ArtisanUseCase {
	return &ArtisanUseCase{
		db: db,
	}
}

func (uc *ArtisanUseCase) CreateArtisan(ctx context.Context, lead *reservationcenter.Artisan) {
	if err := uc.db.WithContext(ctx).Create(&lead).Error; err != nil {
		panic(err)
	}
}

func (uc *ArtisanUseCase) UpsertArtisan(ctx context.Context, lead *reservationcenter.Artisan) (*reservationcenter.Artisan, error) {

	leads := []*reservationcenter.Artisan{lead}

	_, err := uc.UpsertArtisans(ctx, leads)
	if err != nil {
		panic(errors.Wrap(err, "upsert lead failed"))
	}

	return lead, err
}

func (uc *ArtisanUseCase) UpsertArtisans(ctx context.Context, leads []*reservationcenter.Artisan) ([]*reservationcenter.Artisan, error) {

	err := powermodel.UpsertModelsOnUniqueID(uc.db.WithContext(ctx), &reservationcenter.Artisan{}, reservationcenter.ArtisanUniqueId, leads, nil)

	if err != nil {
		panic(errors.Wrap(err, "batch upsert leads failed"))
	}

	return leads, err
}

func (uc *ArtisanUseCase) PatchArtisan(ctx context.Context, id int64, lead *reservationcenter.Artisan) {
	if err := uc.db.WithContext(ctx).Model(&reservationcenter.Artisan{}).Where(id).Updates(&lead).Error; err != nil {
		panic(err)
	}
}

func (uc *ArtisanUseCase) GetArtisan(ctx context.Context, id int64) (*reservationcenter.Artisan, error) {
	var lead reservationcenter.Artisan
	if err := uc.db.WithContext(ctx).First(&lead, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errorx.WithCause(errorx.ErrBadRequest, "未找到产品")
		}
		panic(err)
	}
	return &lead, nil
}

func (uc *ArtisanUseCase) DeleteArtisan(ctx context.Context, id int64) error {
	result := uc.db.WithContext(ctx).Delete(&reservationcenter.Artisan{}, id)
	if err := result.Error; err != nil {
		panic(err)
	}
	if result.RowsAffected == 0 {
		return errorx.WithCause(errorx.ErrBadRequest, "未找到产品")
	}
	return nil
}
