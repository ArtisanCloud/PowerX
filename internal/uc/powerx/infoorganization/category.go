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

type CategoryUseCase struct {
	db *gorm.DB
}

func NewCategoryUseCase(db *gorm.DB) *CategoryUseCase {
	return &CategoryUseCase{
		db: db,
	}
}

type FindCategoryOption struct {
	OrderBy     string
	CategoryPId int
	Limit       int
	Ids         []int64
	Names       []string
}

func (uc *CategoryUseCase) buildFindQueryNoPage(query *gorm.DB, opt *FindCategoryOption) *gorm.DB {
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

func (uc *CategoryUseCase) PreloadItems(db *gorm.DB) *gorm.DB {
	db = db.
		Preload("CoverImage")

	return db
}

func (uc *CategoryUseCase) ListCategoryTree(ctx context.Context, opt *FindCategoryOption, pId int64) []*infoorganizatoin.Category {
	if pId < 0 {
		panic(errors.New("find categories pId invalid"))
	}

	var categories []*infoorganizatoin.Category

	query := uc.db.WithContext(ctx).Model(&infoorganizatoin.Category{})
	query = uc.buildFindQueryNoPage(query, opt)

	query = uc.PreloadItems(query)
	err := query.
		Where("p_id", pId).
		Debug().
		Find(&categories).
		Error
	if err != nil {
		panic(errors.Wrap(err, "find all categories failed"))
	}
	var children []*infoorganizatoin.Category
	for i, category := range categories {

		children = uc.ListCategoryTree(ctx, opt, category.Id)

		if len(children) > 0 {
			categories[i].Children = children
		}
	}
	return categories
}

func (uc *CategoryUseCase) FindCategoriesByParentId(ctx context.Context, opt *FindCategoryOption) []*infoorganizatoin.Category {
	if opt.CategoryPId < 0 {
		panic(errors.New("find categories pId invalid"))
	}

	var categories []*infoorganizatoin.Category
	query := uc.db.WithContext(ctx).Model(&infoorganizatoin.Category{})

	query = uc.buildFindQueryNoPage(query, opt)

	query = uc.PreloadItems(query)
	if err := query.
		Where("p_id", opt.CategoryPId).
		Find(&categories).Error; err != nil {
		panic(errors.Wrap(err, "find all categories failed"))
	}
	return categories
}

func (uc *CategoryUseCase) FindAllCategories(ctx context.Context, opt *FindCategoryOption) []*infoorganizatoin.Category {

	var categories []*infoorganizatoin.Category
	query := uc.db.WithContext(ctx).Model(&infoorganizatoin.Category{})

	query = uc.buildFindQueryNoPage(query, opt)

	if err := query.
		Find(&categories).Error; err != nil {
		panic(errors.Wrap(err, "find all categories failed"))
	}
	return categories
}

func (uc *CategoryUseCase) FindOneCategory(ctx context.Context, opt *FindCategoryOption) (*infoorganizatoin.Category, error) {
	var mpCustomer *infoorganizatoin.Category
	query := uc.db.WithContext(ctx).Model(&infoorganizatoin.Category{})

	query = uc.buildFindQueryNoPage(query, opt)
	if err := query.First(&mpCustomer).Error; err != nil {
		return nil, errorx.ErrRecordNotFound
	}
	return mpCustomer, nil
}

func (uc *CategoryUseCase) CreateCategory(ctx context.Context, category *infoorganizatoin.Category) error {
	if err := uc.db.WithContext(ctx).Create(&category).Error; err != nil {
		if strings.Contains(err.Error(), "duplicate key value violates unique constraint") {
			return errorx.WithCause(errorx.ErrDuplicatedInsert, "该对象不能重复创建")
		}
		panic(err)
	}
	return nil
}

func (uc *CategoryUseCase) UpsertCategory(ctx context.Context, category *infoorganizatoin.Category) (*infoorganizatoin.Category, error) {

	// 查询父节点
	if category.PId > 0 {
		var pCategory *infoorganizatoin.Category
		err := uc.db.WithContext(ctx).
			Where(category.PId).First(&pCategory).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, errorx.WithCause(errorx.ErrBadRequest, "父类别不存在")
			}
			panic(errors.Wrap(err, "query parent product category failed"))
		}
	} else if category.PId < 0 {
		panic(errors.New("query parent product category in invalid"))
	}

	categories := []*infoorganizatoin.Category{category}

	_, err := uc.UpsertCategories(ctx, categories)
	if err != nil {
		panic(errors.Wrap(err, "upsert category failed"))
	}

	return category, err
}

func (uc *CategoryUseCase) UpsertCategories(ctx context.Context, categories []*infoorganizatoin.Category) ([]*infoorganizatoin.Category, error) {

	err := powermodel.UpsertModelsOnUniqueID(uc.db.WithContext(ctx), &infoorganizatoin.Category{}, infoorganizatoin.CategoryUniqueId, categories, nil, false)

	if err != nil {
		panic(errors.Wrap(err, "batch upsert product categories failed"))
	}

	return categories, err
}

func (uc *CategoryUseCase) PatchCategory(ctx context.Context, id int64, category *infoorganizatoin.Category) {
	if err := uc.db.WithContext(ctx).Model(&infoorganizatoin.Category{}).Where(id).Updates(category).Error; err != nil {
		panic(err)
	}
}

func (uc *CategoryUseCase) GetCategory(ctx context.Context, id int64) (*infoorganizatoin.Category, error) {
	var category infoorganizatoin.Category
	db := uc.db.WithContext(ctx)
	db = uc.PreloadItems(db)
	if err := db.
		//Debug().
		First(&category, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errorx.WithCause(errorx.ErrBadRequest, "未找到产品类别")
		}
		panic(err)
	}

	_ = category.LoadChildren(db, nil, false)

	return &category, nil
}

func (uc *CategoryUseCase) DeleteCategory(ctx context.Context, id int64) error {
	result := uc.db.WithContext(ctx).Delete(&infoorganizatoin.Category{}, id)
	if err := result.Error; err != nil {
		panic(err)
	}
	if result.RowsAffected == 0 {
		return errorx.WithCause(errorx.ErrDeleteObjectNotFound, "未找到产品类别")
	}
	return nil
}
