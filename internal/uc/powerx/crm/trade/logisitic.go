package trade

import (
	"PowerX/internal/model/crm/trade"
	"PowerX/internal/model/powermodel"
	"PowerX/internal/types"
	"PowerX/internal/types/errorx"
	"context"
	"github.com/pkg/errors"
	"gorm.io/gorm"
	"strings"
)

type LogisticsUseCase struct {
	db *gorm.DB
}

func NewLogisticsUseCase(db *gorm.DB) *LogisticsUseCase {
	return &LogisticsUseCase{
		db: db,
	}
}

type FindManyLogisticsOption struct {
	CustomerId int64

	OrderBy string
	types.PageEmbedOption
}

func (uc *LogisticsUseCase) buildFindQueryNoPage(db *gorm.DB, opt *FindManyLogisticsOption) *gorm.DB {

	if opt.CustomerId > 0 {
		db = db.Where("customer_id = ?", opt.CustomerId)
	}

	orderBy := "id desc"
	if opt.OrderBy != "" {
		orderBy = opt.OrderBy + "," + orderBy
	}
	db.Order(orderBy)

	return db
}

func (uc *LogisticsUseCase) PreloadItems(db *gorm.DB) *gorm.DB {
	//db = db.
	//Preload("Items.ProductBookEntry.SKU").
	return db
}

func (uc *LogisticsUseCase) FindManyLogistics(ctx context.Context, opt *FindManyLogisticsOption) (pageList types.Page[*trade.Logistics], err error) {
	opt.DefaultPageIfNotSet()
	var logistics []*trade.Logistics
	db := uc.db.WithContext(ctx).Model(&trade.Logistics{})

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
		Find(&logistics).Error; err != nil {
		panic(err)
	}

	return types.Page[*trade.Logistics]{
		List:      logistics,
		PageIndex: opt.PageIndex,
		PageSize:  opt.PageSize,
		Total:     count,
	}, nil
}

func (uc *LogisticsUseCase) CreateLogistics(ctx context.Context, order *trade.Logistics) error {

	if err := uc.db.WithContext(ctx).
		//Debug().
		Create(&order).Error; err != nil {
		if strings.Contains(err.Error(), "duplicate key value violates unique constraint") {
			return errorx.WithCause(errorx.ErrDuplicatedInsert, "该对象不能重复创建")
		}
		panic(err)
	}
	return nil
}

func (uc *LogisticsUseCase) UpsertLogistic(ctx context.Context, order *trade.Logistics) (*trade.Logistics, error) {

	logistics := []*trade.Logistics{order}

	err := uc.db.Transaction(func(tx *gorm.DB) error {
		// 删除产品的相关联对象
		_, err := uc.ClearAssociations(tx, order)
		if err != nil {
			return err
		}

		// 更新产品对象主体
		_, err = uc.UpsertLogistics(ctx, logistics)
		if err != nil {
			return errors.Wrap(err, "upsert order failed")
		}

		return err
	})

	return order, err
}

func (uc *LogisticsUseCase) UpsertLogistics(ctx context.Context, logistics []*trade.Logistics) ([]*trade.Logistics, error) {

	err := powermodel.UpsertModelsOnUniqueID(uc.db.WithContext(ctx), &trade.Logistics{}, trade.LogisticsUniqueId, logistics, nil, false)

	if err != nil {
		panic(errors.Wrap(err, "batch upsert logistics failed"))
	}

	return logistics, err
}

func (uc *LogisticsUseCase) PatchLogistics(ctx context.Context, id int64, order *trade.Logistics) {
	if err := uc.db.WithContext(ctx).Model(&trade.Logistics{}).
		Where(id).Updates(&order).Error; err != nil {
		panic(err)
	}
}

func (uc *LogisticsUseCase) GetLogistics(ctx context.Context, id int64) (*trade.Logistics, error) {
	var order = &trade.Logistics{}
	db := uc.db.WithContext(ctx)
	db = uc.PreloadItems(db)
	if err := db.
		//Debug().
		First(order, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errorx.WithCause(errorx.ErrBadRequest, "未找到产品")
		}
		panic(err)
	}

	return order, nil
}

func (uc *LogisticsUseCase) ClearAssociations(db *gorm.DB, order *trade.Logistics) (*trade.Logistics, error) {
	var err error
	// 清除订单的关联
	//err = order.ClearArtisans(db)
	//if err != nil {
	//	return nil, err
	//}

	return order, err
}
