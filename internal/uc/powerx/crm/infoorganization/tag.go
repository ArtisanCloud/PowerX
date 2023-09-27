package infoorganization

import (
	infoorganizatoin "PowerX/internal/model/infoorganization"
	"PowerX/internal/model/powermodel"
	"PowerX/internal/types/errorx"
	"context"
	"github.com/pkg/errors"
	"gorm.io/gorm"
	"strings"
)

type TagUseCase struct {
	db *gorm.DB
}

func NewTagUseCase(db *gorm.DB) *TagUseCase {
	return &TagUseCase{
		db: db,
	}
}

type FindTagOption struct {
	OrderBy string
	TagPId  int
	Limit   int
	Ids     []int64
	Names   []string
}

func (uc *TagUseCase) buildFindQueryNoPage(query *gorm.DB, opt *FindTagOption) *gorm.DB {
	if len(opt.Ids) > 0 {
		query.Where("id in ?", opt.Ids)
	}
	if len(opt.Names) > 0 {
		query.Where("name in ?", opt.Names)
	}
	if opt.Limit > 0 {
		query.Limit(opt.Limit)
	}

	orderBy := "sort desc, id "
	if opt.OrderBy != "" {
		orderBy = opt.OrderBy + "," + orderBy
	}
	query.Order(orderBy)

	return query
}

func (uc *TagUseCase) PreloadItems(db *gorm.DB) *gorm.DB {
	db = db.
		Preload("CoverImage")

	return db
}

func (uc *TagUseCase) ListTagTree(ctx context.Context, opt *FindTagOption, pId int64) []*infoorganizatoin.Tag {
	if pId < 0 {
		panic(errors.New("find tags pId invalid"))
	}

	var tags []*infoorganizatoin.Tag

	query := uc.db.WithContext(ctx).Model(&infoorganizatoin.Tag{})
	query = uc.buildFindQueryNoPage(query, opt)

	query = uc.PreloadItems(query)
	err := query.
		Where("p_id", pId).
		//Debug().
		Find(&tags).
		Error
	if err != nil {
		panic(errors.Wrap(err, "find all tags failed"))
	}
	var children []*infoorganizatoin.Tag
	for i, tag := range tags {

		children = uc.ListTagTree(ctx, opt, tag.Id)

		if len(children) > 0 {
			tags[i].Children = children
		}
	}
	return tags
}

func (uc *TagUseCase) FindTagsByParentId(ctx context.Context, opt *FindTagOption) []*infoorganizatoin.Tag {
	if opt.TagPId < 0 {
		panic(errors.New("find tags pId invalid"))
	}

	var tags []*infoorganizatoin.Tag
	query := uc.db.WithContext(ctx).Model(&infoorganizatoin.Tag{})

	query = uc.buildFindQueryNoPage(query, opt)

	query = uc.PreloadItems(query)
	if err := query.
		Where("p_id", opt.TagPId).
		Find(&tags).Error; err != nil {
		panic(errors.Wrap(err, "find all tags failed"))
	}
	return tags
}

func (uc *TagUseCase) FindAllTags(ctx context.Context, opt *FindTagOption) []*infoorganizatoin.Tag {

	var tags []*infoorganizatoin.Tag
	query := uc.db.WithContext(ctx).Model(&infoorganizatoin.Tag{})

	query = uc.buildFindQueryNoPage(query, opt)

	if err := query.
		Find(&tags).Error; err != nil {
		panic(errors.Wrap(err, "find all tags failed"))
	}
	return tags
}

func (uc *TagUseCase) FindOneTag(ctx context.Context, opt *FindTagOption) (*infoorganizatoin.Tag, error) {
	var mpCustomer *infoorganizatoin.Tag
	query := uc.db.WithContext(ctx).Model(&infoorganizatoin.Tag{})

	query = uc.buildFindQueryNoPage(query, opt)
	if err := query.First(&mpCustomer).Error; err != nil {
		return nil, errorx.ErrRecordNotFound
	}
	return mpCustomer, nil
}

func (uc *TagUseCase) CreateTag(ctx context.Context, tag *infoorganizatoin.Tag) error {
	if err := uc.db.WithContext(ctx).Create(&tag).Error; err != nil {
		if strings.Contains(err.Error(), "duplicate key value violates unique constraint") {
			return errorx.WithCause(errorx.ErrDuplicatedInsert, "该对象不能重复创建")
		}
		panic(err)
	}
	return nil
}

func (uc *TagUseCase) UpsertTag(ctx context.Context, tag *infoorganizatoin.Tag) (*infoorganizatoin.Tag, error) {

	// 查询父节点
	if tag.PId > 0 {
		var pTag *infoorganizatoin.Tag
		err := uc.db.WithContext(ctx).
			Where(tag.PId).First(&pTag).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, errorx.WithCause(errorx.ErrBadRequest, "父标签不存在")
			}
			panic(errors.Wrap(err, "query parent product tag failed"))
		}
	} else if tag.PId < 0 {
		panic(errors.New("query parent product tag in invalid"))
	}

	tags := []*infoorganizatoin.Tag{tag}

	_, err := uc.UpsertTags(ctx, tags)
	if err != nil {
		panic(errors.Wrap(err, "upsert tag failed"))
	}

	return tag, err
}

func (uc *TagUseCase) UpsertTags(ctx context.Context, tags []*infoorganizatoin.Tag) ([]*infoorganizatoin.Tag, error) {

	err := powermodel.UpsertModelsOnUniqueID(uc.db.WithContext(ctx), &infoorganizatoin.Tag{}, infoorganizatoin.TagUniqueId, tags, nil, false)

	if err != nil {
		panic(errors.Wrap(err, "batch upsert product tags failed"))
	}

	return tags, err
}

func (uc *TagUseCase) PatchTag(ctx context.Context, id int64, tag *infoorganizatoin.Tag) {
	if err := uc.db.WithContext(ctx).Model(&infoorganizatoin.Tag{}).Where(id).Updates(tag).Error; err != nil {
		panic(err)
	}
}

func (uc *TagUseCase) GetTag(ctx context.Context, id int64) (*infoorganizatoin.Tag, error) {
	var tag infoorganizatoin.Tag
	db := uc.db.WithContext(ctx)
	db = uc.PreloadItems(db)
	if err := db.
		//Debug().
		First(&tag, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errorx.WithCause(errorx.ErrBadRequest, "未找到产品标签")
		}
		panic(err)
	}

	_ = tag.LoadChildren(db, nil, false)

	return &tag, nil
}

func (uc *TagUseCase) DeleteTag(ctx context.Context, id int64) error {
	result := uc.db.WithContext(ctx).Delete(&infoorganizatoin.Tag{}, id)
	if err := result.Error; err != nil {
		panic(err)
	}
	if result.RowsAffected == 0 {
		return errorx.WithCause(errorx.ErrDeleteObjectNotFound, "未找到产品标签")
	}
	return nil
}
