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

type LabelUseCase struct {
	db *gorm.DB
}

func NewLabelUseCase(db *gorm.DB) *LabelUseCase {
	return &LabelUseCase{
		db: db,
	}
}

type FindLabelOption struct {
	OrderBy  string
	LabelPId int
	Limit    int
	Ids      []int64
	Names    []string
}

func (uc *LabelUseCase) buildFindQueryNoPage(query *gorm.DB, opt *FindLabelOption) *gorm.DB {
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

func (uc *LabelUseCase) PreloadItems(db *gorm.DB) *gorm.DB {
	db = db.
		Preload("CoverImage")

	return db
}

func (uc *LabelUseCase) ListLabelTree(ctx context.Context, opt *FindLabelOption, pId int64) []*infoorganizatoin.Label {
	if pId < 0 {
		panic(errors.New("find labels pId invalid"))
	}

	var labels []*infoorganizatoin.Label

	query := uc.db.WithContext(ctx).Model(&infoorganizatoin.Label{})
	query = uc.buildFindQueryNoPage(query, opt)

	query = uc.PreloadItems(query)
	err := query.
		Where("p_id", pId).
		//Debug().
		Find(&labels).
		Error
	if err != nil {
		panic(errors.Wrap(err, "find all labels failed"))
	}
	var children []*infoorganizatoin.Label
	for i, label := range labels {

		children = uc.ListLabelTree(ctx, opt, label.Id)

		if len(children) > 0 {
			labels[i].Children = children
		}
	}
	return labels
}

func (uc *LabelUseCase) FindLabelsByParentId(ctx context.Context, opt *FindLabelOption) []*infoorganizatoin.Label {
	if opt.LabelPId < 0 {
		panic(errors.New("find labels pId invalid"))
	}

	var labels []*infoorganizatoin.Label
	query := uc.db.WithContext(ctx).Model(&infoorganizatoin.Label{})

	query = uc.buildFindQueryNoPage(query, opt)

	query = uc.PreloadItems(query)
	if err := query.
		Where("p_id", opt.LabelPId).
		Find(&labels).Error; err != nil {
		panic(errors.Wrap(err, "find all labels failed"))
	}
	return labels
}

func (uc *LabelUseCase) FindAllLabels(ctx context.Context, opt *FindLabelOption) []*infoorganizatoin.Label {

	var labels []*infoorganizatoin.Label
	query := uc.db.WithContext(ctx).Model(&infoorganizatoin.Label{})

	query = uc.buildFindQueryNoPage(query, opt)

	if err := query.
		Find(&labels).Error; err != nil {
		panic(errors.Wrap(err, "find all labels failed"))
	}
	return labels
}

func (uc *LabelUseCase) FindOneLabel(ctx context.Context, opt *FindLabelOption) (*infoorganizatoin.Label, error) {
	var mpCustomer *infoorganizatoin.Label
	query := uc.db.WithContext(ctx).Model(&infoorganizatoin.Label{})

	query = uc.buildFindQueryNoPage(query, opt)
	if err := query.First(&mpCustomer).Error; err != nil {
		return nil, errorx.ErrRecordNotFound
	}
	return mpCustomer, nil
}

func (uc *LabelUseCase) CreateLabel(ctx context.Context, label *infoorganizatoin.Label) error {
	if err := uc.db.WithContext(ctx).Create(&label).Error; err != nil {
		if strings.Contains(err.Error(), "duplicate key value violates unique constraint") {
			return errorx.WithCause(errorx.ErrDuplicatedInsert, "该对象不能重复创建")
		}
		panic(err)
	}
	return nil
}

func (uc *LabelUseCase) UpsertLabel(ctx context.Context, label *infoorganizatoin.Label) (*infoorganizatoin.Label, error) {

	// 查询父节点
	if label.PId > 0 {
		var pLabel *infoorganizatoin.Label
		err := uc.db.WithContext(ctx).
			Where(label.PId).First(&pLabel).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, errorx.WithCause(errorx.ErrBadRequest, "父标签不存在")
			}
			panic(errors.Wrap(err, "query parent product label failed"))
		}
	} else if label.PId < 0 {
		panic(errors.New("query parent product label in invalid"))
	}

	labels := []*infoorganizatoin.Label{label}

	_, err := uc.UpsertLabels(ctx, labels)
	if err != nil {
		panic(errors.Wrap(err, "upsert label failed"))
	}

	return label, err
}

func (uc *LabelUseCase) UpsertLabels(ctx context.Context, labels []*infoorganizatoin.Label) ([]*infoorganizatoin.Label, error) {

	err := powermodel.UpsertModelsOnUniqueID(uc.db.WithContext(ctx), &infoorganizatoin.Label{}, infoorganizatoin.LabelUniqueId, labels, nil, false)

	if err != nil {
		panic(errors.Wrap(err, "batch upsert product labels failed"))
	}

	return labels, err
}

func (uc *LabelUseCase) PatchLabel(ctx context.Context, id int64, label *infoorganizatoin.Label) {
	if err := uc.db.WithContext(ctx).Model(&infoorganizatoin.Label{}).Where(id).Updates(label).Error; err != nil {
		panic(err)
	}
}

func (uc *LabelUseCase) GetLabel(ctx context.Context, id int64) (*infoorganizatoin.Label, error) {
	var label infoorganizatoin.Label
	db := uc.db.WithContext(ctx)
	db = uc.PreloadItems(db)
	if err := db.
		//Debug().
		First(&label, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errorx.WithCause(errorx.ErrBadRequest, "未找到产品标签")
		}
		panic(err)
	}

	_ = label.LoadChildren(db, nil, false)

	return &label, nil
}

func (uc *LabelUseCase) DeleteLabel(ctx context.Context, id int64) error {
	result := uc.db.WithContext(ctx).Delete(&infoorganizatoin.Label{}, id)
	if err := result.Error; err != nil {
		panic(err)
	}
	if result.RowsAffected == 0 {
		return errorx.WithCause(errorx.ErrDeleteObjectNotFound, "未找到产品标签")
	}
	return nil
}
