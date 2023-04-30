package product

import (
	"PowerX/internal/model/custom/product"
	"PowerX/internal/model/powermodel"
	"PowerX/internal/types"
	"PowerX/internal/types/errorx"
	"context"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

type ServiceSpecificUseCase struct {
	db *gorm.DB
}

func NewServiceSpecificUseCase(db *gorm.DB) *ServiceSpecificUseCase {
	return &ServiceSpecificUseCase{
		db: db,
	}
}

type FindServiceSpecificOption struct {
	OrderBy string
	Ids     []int64
	Names   []string
	types.PageEmbedOption
}

func (uc *ServiceSpecificUseCase) buildFindQueryNoPage(query *gorm.DB, opt *FindServiceSpecificOption) *gorm.DB {
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

func (uc *ServiceSpecificUseCase) FindAllServiceSpecifics(ctx context.Context, opt *FindServiceSpecificOption) []*product.ServiceSpecific {
	var productCategories []*product.ServiceSpecific
	query := uc.db.WithContext(ctx).Model(&product.ServiceSpecific{})

	query = uc.buildFindQueryNoPage(query, opt)
	if err := query.Find(&productCategories).Error; err != nil {
		panic(errors.Wrap(err, "find all productCategories failed"))
	}
	return productCategories
}

func (uc *ServiceSpecificUseCase) FindManyServiceSpecifics(ctx context.Context, opt *FindServiceSpecificOption) (pageList types.Page[*product.ServiceSpecific], err error) {
	var serviceSpecifics []*product.ServiceSpecific
	db := uc.db.WithContext(ctx).Model(&product.ServiceSpecific{})

	db = uc.buildFindQueryNoPage(db, opt)

	// 只拿第一层服务
	db = db.Where("parent_id", 0)

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
		Preload("Product").
		Preload("Children").
		Find(&serviceSpecifics).Error; err != nil {
		panic(err)
	}

	return types.Page[*product.ServiceSpecific]{
		List:      serviceSpecifics,
		PageIndex: opt.PageIndex,
		PageSize:  opt.PageSize,
		Total:     count,
	}, nil
}

func (uc *ServiceSpecificUseCase) FindOneServiceSpecific(ctx context.Context, opt *FindServiceSpecificOption) (*product.ServiceSpecific, error) {
	var mpCustomer *product.ServiceSpecific
	query := uc.db.WithContext(ctx).Model(&product.ServiceSpecific{})

	query = uc.buildFindQueryNoPage(query, opt)
	if err := query.First(&mpCustomer).Error; err != nil {
		return nil, errorx.ErrRecordNotFound
	}
	return mpCustomer, nil
}

func (uc *ServiceSpecificUseCase) CreateServiceSpecific(ctx context.Context, lead *product.ServiceSpecific) {
	if err := uc.db.WithContext(ctx).Create(&lead).Error; err != nil {
		panic(err)
	}
}

func (uc *ServiceSpecificUseCase) UpsertServiceSpecific(ctx context.Context, lead *product.ServiceSpecific) (*product.ServiceSpecific, error) {

	leads := []*product.ServiceSpecific{lead}

	_, err := uc.UpsertServiceSpecifics(ctx, leads)
	if err != nil {
		panic(errors.Wrap(err, "upsert lead failed"))
	}

	return lead, err
}

func (uc *ServiceSpecificUseCase) UpsertServiceSpecifics(ctx context.Context, leads []*product.ServiceSpecific) ([]*product.ServiceSpecific, error) {

	err := powermodel.UpsertModelsOnUniqueID(uc.db.WithContext(ctx), &product.ServiceSpecific{}, product.ServiceSpecificUniqueId, leads, nil, false)

	if err != nil {
		panic(errors.Wrap(err, "batch upsert leads failed"))
	}

	return leads, err
}

func (uc *ServiceSpecificUseCase) PatchServiceSpecific(ctx context.Context, id int64, lead *product.ServiceSpecific) {
	if err := uc.db.WithContext(ctx).Model(&product.ServiceSpecific{}).Where(id).Updates(&lead).Error; err != nil {
		panic(err)
	}
}

func (uc *ServiceSpecificUseCase) GetServiceSpecific(ctx context.Context, id int64) (*product.ServiceSpecific, error) {
	var lead product.ServiceSpecific
	if err := uc.db.WithContext(ctx).First(&lead, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errorx.WithCause(errorx.ErrBadRequest, "未找到产品")
		}
		panic(err)
	}
	return &lead, nil
}

func (uc *ServiceSpecificUseCase) DeleteServiceSpecific(ctx context.Context, id int64) error {
	result := uc.db.WithContext(ctx).Delete(&product.ServiceSpecific{}, id)
	if err := result.Error; err != nil {
		panic(err)
	}
	if result.RowsAffected == 0 {
		return errorx.WithCause(errorx.ErrBadRequest, "未找到产品")
	}
	return nil
}
