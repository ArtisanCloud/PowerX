package market

import (
	model "PowerX/internal/model/market"
	"PowerX/internal/model/media"
	"PowerX/internal/model/powermodel"
	"PowerX/internal/model/trade"
	"PowerX/internal/types"
	"PowerX/internal/types/errorx"
	"context"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

type MediaUseCase struct {
	db *gorm.DB
}

func NewMediaUseCase(db *gorm.DB) *MediaUseCase {
	return &MediaUseCase{
		db: db,
	}
}

type FindManyMediasOption struct {
	Types    []int8
	Ids      []int64
	LikeName string
	OrderBy  string
	types.PageEmbedOption
}

func (uc *MediaUseCase) buildFindQueryNoPage(db *gorm.DB, opt *FindManyMediasOption) *gorm.DB {
	if len(opt.Types) > 0 {
		db = db.Where("media_type IN ?", opt.Types)
	}

	if len(opt.Ids) > 0 {
		db = db.Where("id IN ?", opt.Ids)
	}

	if opt.LikeName != "" {
		db = db.Where("title LIKE ?", "%"+opt.LikeName+"%")
	}
	orderBy := "id desc"
	if opt.OrderBy != "" {
		orderBy = opt.OrderBy + "," + orderBy
	}
	db.Order(orderBy)
	return db
}

func (uc *MediaUseCase) PreloadItems(db *gorm.DB) *gorm.DB {
	db = db.
		Preload("PivotDetailImages", "media_usage = ?", media.MediaUsageDetail).Preload("PivotDetailImages.MediaResource").
		Preload("CoverImage")

	return db
}

func (uc *MediaUseCase) FindManyMedias(ctx context.Context, opt *FindManyMediasOption) (pageList types.Page[*model.Media], err error) {
	var medias []*model.Media
	db := uc.db.WithContext(ctx).Model(&model.Media{})

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
		Debug().
		Find(&medias).Error; err != nil {
		panic(err)
	}

	return types.Page[*model.Media]{
		List:      medias,
		PageIndex: opt.PageIndex,
		PageSize:  opt.PageSize,
		Total:     count,
	}, nil
}

func (uc *MediaUseCase) CreateMedia(ctx context.Context, m *model.Media) {
	err := uc.db.WithContext(ctx).
		Debug().
		Create(m).Error

	if err != nil {
		panic(err)
	}
}

func (uc *MediaUseCase) UpsertMedia(ctx context.Context, m *model.Media) (*model.Media, error) {

	medias := []*model.Media{m}

	err := uc.db.Transaction(func(tx *gorm.DB) error {
		// 删除媒体的相关联对象
		_, err := uc.ClearAssociations(tx, m)
		if err != nil {
			return err
		}

		// 更新媒体对象主体
		_, err = uc.UpsertMedias(ctx, medias)
		if err != nil {
			return errors.Wrap(err, "upsert media failed")
		}

		return err
	})

	return m, err
}

func (uc *MediaUseCase) UpsertMedias(ctx context.Context, medias []*model.Media) ([]*model.Media, error) {

	err := powermodel.UpsertModelsOnUniqueID(uc.db.WithContext(ctx), &model.Media{}, trade.RefundOrderUniqueId, medias, nil, false)

	if err != nil {
		panic(errors.Wrap(err, "batch upsert medias failed"))
	}

	return medias, err
}

func (uc *MediaUseCase) PatchMedia(ctx context.Context, id int64, m *model.Media) {
	if err := uc.db.WithContext(ctx).Model(&model.Media{}).Where(id).Updates(m).Error; err != nil {
		panic(err)
	}
}

func (uc *MediaUseCase) GetMedia(ctx context.Context, id int64) (*model.Media, error) {
	var m model.Media
	if err := uc.db.WithContext(ctx).First(&m, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errorx.WithCause(errorx.ErrBadRequest, "未找到媒体")
		}
		panic(err)
	}
	return &m, nil
}

func (uc *MediaUseCase) DeleteMedia(ctx context.Context, id int64) error {
	result := uc.db.WithContext(ctx).Delete(&model.Media{}, id)
	if err := result.Error; err != nil {
		panic(err)
	}
	if result.RowsAffected == 0 {
		return errorx.WithCause(errorx.ErrBadRequest, "未找到媒体")
	}
	return nil
}

func (uc *MediaUseCase) ClearAssociations(db *gorm.DB, media *model.Media) (*model.Media, error) {
	var err error

	// 清除媒体详细图片记录
	err = media.ClearPivotDetailImages(db)
	if err != nil {
		return nil, err
	}

	return media, err

}
