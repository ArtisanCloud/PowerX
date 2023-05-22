package trade

import (
	customerdomain2 "PowerX/internal/model/customerdomain"
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

func (uc *OrderUseCase) CreateOrderByCartItems(ctx context.Context,
	customer *customerdomain2.Customer,
	cartItems []*trade.CartItem,
	shippingAddress *trade.ShippingAddress,
	comment string,
) (*trade.Order, *trade.Cart, error) {

	order := &trade.Order{}
	// 创建购物车合集
	cart := &trade.Cart{
		CustomerId: customer.Id,
	}
	db := uc.db.WithContext(ctx)

	err := db.Transaction(func(tx *gorm.DB) error {
		var err error

		// 这里可以将cartItems和cart进行关联
		cart.Items = cartItems
		err = tx.Model(trade.Cart{}).
			//Debug().
			Omit("Items.SKU.PriceBookEntry").
			Omit("Items.SKU").
			Create(cart).Error

		if err != nil {
			return err
		}

		// 创建订单
		orderItems, totalUnitPrice, totalListPrice := uc.MakeOrderItemsFromCartItems(
			cartItems,
			trade.OrderTypeNormal,
			trade.OrderStatusPending,
		)
		order.OrderItems = orderItems

		order.CustomerId = customer.Id
		order.CartId = cart.Id
		order.Type = trade.OrderTypeNormal
		order.Status = trade.OrderStatusPending
		order.OrderNumber = trade.GenerateOrderNumber()
		order.UnitPrice = totalUnitPrice
		order.ListPrice = totalListPrice
		order.Discount = totalUnitPrice / totalListPrice
		order.Comment = comment

		err = tx.Model(trade.Order{}).
			Debug().
			Create(order).Error
		if err != nil {
			return err
		}

		// 创建发货地址
		deliveryAddress := &trade.DeliveryAddress{}
		deliveryAddress.OrderId = order.Id
		deliveryAddress.ShippingAddress = shippingAddress

		err = tx.Model(trade.DeliveryAddress{}).
			Debug().
			Create(deliveryAddress).Error

		return err
	})

	return order, cart, err
}

func (uc *OrderUseCase) MakeOrderItemsFromCartItems(
	cartItems []*trade.CartItem,
	orderType trade.OrderType,
	orderStatus trade.OrderStatus,
) (orderItems []*trade.OrderItem, totalUnitPrice float64, totalListPrice float64) {

	totalUnitPrice = 0.0
	totalListPrice = 0.0
	orderItems = []*trade.OrderItem{}
	for _, cartItem := range cartItems {
		orderItem, subUnitTotal, subListTotal := uc.MakeOrderItemFromCartItem(cartItem, orderType, orderStatus)
		orderItems = append(orderItems, orderItem)
		totalUnitPrice += subUnitTotal
		totalListPrice += subListTotal
	}
	return orderItems, totalUnitPrice, totalListPrice
}

func (uc *OrderUseCase) MakeOrderItemFromCartItem(
	cartItem *trade.CartItem,
	orderType trade.OrderType,
	orderStatus trade.OrderStatus,
) (orderItem *trade.OrderItem, subUnitTotal float64, subListTotal float64) {

	subUnitTotal = 0.0
	subListTotal = 0.0
	orderItem = &trade.OrderItem{
		PriceBookEntryId: cartItem.SkuId,
		CustomerId:       cartItem.CustomerId,
		Type:             orderType,
		Status:           orderStatus,
		Quantity:         cartItem.Quantity,
		UnitPrice:        cartItem.UnitPrice,
		ListPrice:        cartItem.ListPrice,
		Discount:         cartItem.Discount,
	}
	subUnitTotal = orderItem.UnitPrice * float64(orderItem.Quantity)
	subListTotal = orderItem.ListPrice * float64(orderItem.Quantity)

	return orderItem, subUnitTotal, subListTotal
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
