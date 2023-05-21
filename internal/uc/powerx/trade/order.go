package trade

import (
	"PowerX/internal/model/powermodel"
	"PowerX/internal/model/trade"
	"PowerX/internal/types"
	"PowerX/internal/types/errorx"
	"context"
	"github.com/pkg/errors"
	"gorm.io/gorm"
	"strings"
)

type OrderUseCase struct {
	db *gorm.DB
}

func NewOrderUseCase(db *gorm.DB) *OrderUseCase {
	return &OrderUseCase{
		db: db,
	}
}

type FindManyOrdersOption struct {
	LikeName string
	OrderBy  string
	types.PageEmbedOption
}

func (uc *OrderUseCase) buildFindQueryNoPage(db *gorm.DB, opt *FindManyOrdersOption) *gorm.DB {

	if opt.LikeName != "" {
		db = db.Where("name LIKE ?", "%"+opt.LikeName+"%")
	}

	orderBy := "id desc"
	if opt.OrderBy != "" {
		orderBy = opt.OrderBy + "," + orderBy
	}
	db.Order(orderBy)

	return db
}

func (uc *OrderUseCase) FindAllOrders(ctx context.Context, opt *FindManyOrdersOption) (dictionaryItems []*trade.Order, err error) {
	query := uc.db.WithContext(ctx).Model(&trade.Order{})

	query = uc.buildFindQueryNoPage(query, opt)
	if err := query.
		Debug().
		Preload("Artisans").
		Find(&dictionaryItems).Error; err != nil {
		panic(errors.Wrap(err, "find all dictionaryItems failed"))
	}
	return dictionaryItems, err
}

func (uc *OrderUseCase) FindManyOrders(ctx context.Context, opt *FindManyOrdersOption) (pageList types.Page[*trade.Order], err error) {
	opt.DefaultPageIfNotSet()
	var orders []*trade.Order
	db := uc.db.WithContext(ctx).Model(&trade.Order{})

	db = uc.buildFindQueryNoPage(db, opt)

	var count int64
	if err := db.Count(&count).Error; err != nil {
		panic(err)
	}

	opt.DefaultPageIfNotSet()
	if opt.PageIndex != 0 && opt.PageSize != 0 {
		db.Offset((opt.PageIndex - 1) * opt.PageSize).Limit(opt.PageSize)
	}

	if err := db.Find(&orders).Error; err != nil {
		panic(err)
	}

	return types.Page[*trade.Order]{
		List:      orders,
		PageIndex: opt.PageIndex,
		PageSize:  opt.PageSize,
		Total:     count,
	}, nil
}

func (uc *OrderUseCase) CreateOrder(ctx context.Context, order *trade.Order) error {

	if err := uc.db.WithContext(ctx).
		Debug().
		Create(&order).Error; err != nil {
		if strings.Contains(err.Error(), "duplicate key value violates unique constraint") {
			return errorx.WithCause(errorx.ErrDuplicatedInsert, "该对象不能重复创建")
		}
		panic(err)
	}
	return nil
}

func (uc *OrderUseCase) UpsertOrder(ctx context.Context, order *trade.Order) (*trade.Order, error) {

	orders := []*trade.Order{order}

	err := uc.db.Transaction(func(tx *gorm.DB) error {
		// 删除产品的相关联对象
		_, err := uc.ClearAssociations(tx, order)
		if err != nil {
			return err
		}

		// 更新产品对象主体
		_, err = uc.UpsertOrders(ctx, orders)
		if err != nil {
			return errors.Wrap(err, "upsert order failed")
		}

		return err
	})

	return order, err
}

func (uc *OrderUseCase) UpsertOrders(ctx context.Context, orders []*trade.Order) ([]*trade.Order, error) {

	err := powermodel.UpsertModelsOnUniqueID(uc.db.WithContext(ctx), &trade.Order{}, powermodel.UniqueId, orders, nil, false)

	if err != nil {
		panic(errors.Wrap(err, "batch upsert orders failed"))
	}

	return orders, err
}

func (uc *OrderUseCase) PatchOrder(ctx context.Context, id int64, order *trade.Order) {
	if err := uc.db.WithContext(ctx).Model(&trade.Order{}).
		Where(id).Updates(&order).Error; err != nil {
		panic(err)
	}
}

func (uc *OrderUseCase) GetOrder(ctx context.Context, id int64) (*trade.Order, error) {
	var order = &trade.Order{}
	if err := uc.db.WithContext(ctx).First(order, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errorx.WithCause(errorx.ErrBadRequest, "未找到产品")
		}
		panic(err)
	}

	return order, nil
}

func (uc *OrderUseCase) DeleteOrder(ctx context.Context, id int64) error {

	// 获取产品相关项
	order, err := uc.GetOrder(ctx, id)
	if err != nil {
		return errorx.ErrNotFoundObject
	}

	err = uc.db.Transaction(func(tx *gorm.DB) error {
		// 删除产品相关项
		_, err = uc.ClearAssociations(tx, order)
		if err != nil {
			return err
		}

		result := tx.Delete(&trade.Order{}, id)
		if err := result.Error; err != nil {
			panic(err)
		}
		if result.RowsAffected == 0 {
			return errorx.WithCause(errorx.ErrBadRequest, "未找到产品")
		}
		return err
	})

	return err
}

func (uc *OrderUseCase) ClearAssociations(db *gorm.DB, order *trade.Order) (*trade.Order, error) {
	var err error
	// 清除订单的关联
	//err = order.ClearArtisans(db)
	//if err != nil {
	//	return nil, err
	//}

	return order, err
}
