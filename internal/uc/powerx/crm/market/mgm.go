package market

import (
	model "PowerX/internal/model/crm/market"
	"PowerX/internal/model/powermodel"
	"PowerX/internal/types"
	"PowerX/internal/types/errorx"
	"context"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

type MGMRuleUseCase struct {
	db *gorm.DB
}

func NewMGMRuleUseCase(db *gorm.DB) *MGMRuleUseCase {
	return &MGMRuleUseCase{
		db: db,
	}
}

type FindManyMGMRulesOption struct {
	Types    []int8
	Ids      []int64
	LikeName string
	OrderBy  string
	types.PageEmbedOption
}

func (uc *MGMRuleUseCase) buildFindQueryNoPage(db *gorm.DB, opt *FindManyMGMRulesOption) *gorm.DB {
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

func (uc *MGMRuleUseCase) PreloadItems(db *gorm.DB) *gorm.DB {
	//db = db.
	//	Preload("CoverImage")

	return db
}

func (uc *MGMRuleUseCase) FindManyMGMRules(ctx context.Context, opt *FindManyMGMRulesOption) (pageList types.Page[*model.MGMRule], err error) {
	var medias []*model.MGMRule
	db := uc.db.WithContext(ctx).Model(&model.MGMRule{})

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
		Find(&medias).Error; err != nil {
		panic(err)
	}

	return types.Page[*model.MGMRule]{
		List:      medias,
		PageIndex: opt.PageIndex,
		PageSize:  opt.PageSize,
		Total:     count,
	}, nil
}

func (uc *MGMRuleUseCase) CreateMGMRule(ctx context.Context, m *model.MGMRule) {
	err := uc.db.WithContext(ctx).
		//Debug().
		Create(m).Error

	if err != nil {
		panic(err)
	}
}

func (uc *MGMRuleUseCase) UpsertMGMRule(ctx context.Context, m *model.MGMRule) (*model.MGMRule, error) {

	medias := []*model.MGMRule{m}

	err := uc.db.Transaction(func(tx *gorm.DB) error {
		// 删除媒体的相关联对象
		_, err := uc.ClearAssociations(tx, m)
		if err != nil {
			return err
		}

		// 更新媒体对象主体
		_, err = uc.UpsertMGMRules(ctx, medias)
		if err != nil {
			return errors.Wrap(err, "upsert media failed")
		}

		return err
	})

	return m, err
}

func (uc *MGMRuleUseCase) UpsertMGMRules(ctx context.Context, medias []*model.MGMRule) ([]*model.MGMRule, error) {

	err := powermodel.UpsertModelsOnUniqueID(uc.db.WithContext(ctx), &model.MGMRule{}, model.MGMRuleUniqueId, medias, nil, false)

	if err != nil {
		panic(errors.Wrap(err, "batch upsert medias failed"))
	}

	return medias, err
}

func (uc *MGMRuleUseCase) PatchMGMRule(ctx context.Context, id int64, m *model.MGMRule) {
	if err := uc.db.WithContext(ctx).Model(&model.MGMRule{}).Where(id).Updates(m).Error; err != nil {
		panic(err)
	}
}

func (uc *MGMRuleUseCase) GetMGMRule(ctx context.Context, id int64) (*model.MGMRule, error) {
	var m model.MGMRule
	if err := uc.db.WithContext(ctx).First(&m, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errorx.WithCause(errorx.ErrBadRequest, "未找到媒体")
		}
		panic(err)
	}
	return &m, nil
}

func (uc *MGMRuleUseCase) DeleteMGMRule(ctx context.Context, id int64) error {
	result := uc.db.WithContext(ctx).Delete(&model.MGMRule{}, id)
	if err := result.Error; err != nil {
		panic(err)
	}
	if result.RowsAffected == 0 {
		return errorx.WithCause(errorx.ErrBadRequest, "未找到媒体")
	}
	return nil
}

func (uc *MGMRuleUseCase) ClearAssociations(db *gorm.DB, media *model.MGMRule) (*model.MGMRule, error) {
	var err error

	return media, err

}
