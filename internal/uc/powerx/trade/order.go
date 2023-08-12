package trade

import (
	customerdomain2 "PowerX/internal/model/customerdomain"
	"PowerX/internal/model/powermodel"
	"PowerX/internal/model/product"
	"PowerX/internal/model/trade"
	"PowerX/internal/types"
	"PowerX/internal/types/errorx"
	"PowerX/internal/uc/powerx"
	"PowerX/pkg/slicex"
	"context"
	"github.com/pkg/errors"
	"gorm.io/gorm"
	"strings"
	"time"
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
	CustomerId int64
	Status     []int
	Type       []int
	LikeName   string
	OrderBy    string
	StartAt    time.Time
	EndAt      time.Time
	types.PageEmbedOption
}

func (uc *OrderUseCase) buildFindQueryNoPage(db *gorm.DB, opt *FindManyOrdersOption) *gorm.DB {

	if opt.LikeName != "" {
		db = db.Where("name LIKE ?", "%"+opt.LikeName+"%")
	}

	if opt.CustomerId > 0 {
		db = db.Where("customer_id = ?", opt.CustomerId)
	}

	if len(opt.Status) > 0 {
		db = db.Where("status IN ?", opt.Status)
	}
	if len(opt.Type) > 0 {
		db = db.Where("type IN ?", opt.Type)
	}

	orderBy := "id desc"
	if opt.OrderBy != "" {
		orderBy = opt.OrderBy + "," + orderBy
	}
	db.Order(orderBy)

	return db
}

func (uc *OrderUseCase) PreloadItems(db *gorm.DB) *gorm.DB {
	db = db.
		Preload("Items.ProductBookEntry.SKU").
		Preload("Items.ProductBookEntry.Product.PivotCoverImages").
		Preload("Items.CoverImage").
		Preload("Payments.Items")
	return db
}

func (uc *OrderUseCase) FindAllOrders(ctx context.Context, opt *FindManyOrdersOption) (orders []*trade.Order, err error) {
	query := uc.db.WithContext(ctx).Model(&trade.Order{})

	query = uc.buildFindQueryNoPage(query, opt)
	query = uc.PreloadItems(query)
	if err := query.
		Debug().
		Find(&orders).Error; err != nil {
		panic(errors.Wrap(err, "find all dictionaryItems failed"))
	}
	return orders, err
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

	db = uc.PreloadItems(db)
	if err := db.
		//Debug().
		Find(&orders).Error; err != nil {
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
		//Debug().
		Create(&order).Error; err != nil {
		if strings.Contains(err.Error(), "duplicate key value violates unique constraint") {
			return errorx.WithCause(errorx.ErrDuplicatedInsert, "该对象不能重复创建")
		}
		panic(err)
	}
	return nil
}

func (uc *OrderUseCase) CreateOrderByPriceBookEntries(ctx context.Context,
	customer *customerdomain2.Customer,
	entries []*product.PriceBookEntry,
	quantities []int,
	shippingAddress *trade.ShippingAddress,
	comment string,
) (*trade.Order, error) {
	order := &trade.Order{}
	db := uc.db.WithContext(ctx)

	// 创建订单，类型为 普通订单
	orderTypeId := uc.GetOrderTypeId(ctx, trade.OrderTypeNormal)
	// 创建订单，状态为 待处理
	orderStatusId := uc.GetOrderStatusId(ctx, trade.OrderStatusToBePaid)

	err := db.Transaction(func(tx *gorm.DB) error {
		var err error

		// 创建订单
		orderItems, totalUnitPrice, totalListPrice := uc.MakeOrderItemsFromEntries(
			entries,
			customer,
			quantities,
			orderTypeId,
			orderStatusId,
		)
		order.Items = orderItems

		order.CustomerId = customer.Id
		order.Type = orderTypeId
		order.Status = orderStatusId
		order.OrderNumber = trade.GenerateOrderNumber()
		order.UnitPrice = totalUnitPrice
		order.ListPrice = totalListPrice
		order.Discount = totalUnitPrice / totalListPrice
		order.Comment = comment

		err = tx.Model(trade.Order{}).
			//Debug().
			Create(order).Error
		if err != nil {
			return err
		}

		// 创建发货地址
		deliveryAddress := shippingAddress.MakeDeliveryAddress()
		deliveryAddress.OrderId = order.Id

		err = tx.Model(trade.DeliveryAddress{}).
			//Debug().
			Create(deliveryAddress).Error

		return err
	})

	return order, err
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
			// 在生成订单时候，不希望将sku的相关信息再次有数据操作
			Omit("Items.SKU.pricebookentry").
			Omit("Items.SKU").
			Create(cart).Error

		if err != nil {
			return err
		}

		orderTypeId := uc.GetOrderTypeId(ctx, trade.OrderTypeCart)
		orderStatusId := uc.GetOrderStatusId(ctx, trade.OrderStatusToBePaid)

		// 创建订单
		orderItems, totalUnitPrice, totalListPrice := uc.MakeOrderItemsFromCartItems(
			cartItems,
			orderTypeId,
			orderStatusId,
		)
		order.Items = orderItems

		order.CustomerId = customer.Id
		order.CartId = cart.Id
		order.Type = orderTypeId
		order.Status = orderStatusId
		order.OrderNumber = trade.GenerateOrderNumber()
		order.UnitPrice = totalUnitPrice
		order.ListPrice = totalListPrice
		order.Discount = totalUnitPrice / totalListPrice
		order.Comment = comment

		err = tx.Model(trade.Order{}).
			//Debug().
			Create(order).Error
		if err != nil {
			return err
		}

		// 创建发货地址
		deliveryAddress := shippingAddress.MakeDeliveryAddress()
		deliveryAddress.OrderId = order.Id

		err = tx.Model(trade.DeliveryAddress{}).
			//Debug().
			Create(deliveryAddress).Error

		return err
	})

	return order, cart, err
}

func (uc *OrderUseCase) MakeOrderItemsFromEntries(
	entries []*product.PriceBookEntry,
	customer *customerdomain2.Customer,
	quantities []int,
	orderType int,
	orderStatus int,
) (orderItems []*trade.OrderItem, totalUnitPrice float64, totalListPrice float64) {

	totalUnitPrice = 0.0
	totalListPrice = 0.0
	orderItems = []*trade.OrderItem{}
	for i, entry := range entries {
		orderItem, subUnitTotal, subListTotal := uc.MakeOrderItemFromEntry(i, entry, customer, quantities[i], orderType, orderStatus)
		orderItems = append(orderItems, orderItem)
		totalUnitPrice += subUnitTotal
		totalListPrice += subListTotal
	}
	return orderItems, totalUnitPrice, totalListPrice
}

func (uc *OrderUseCase) MakeOrderItemFromEntry(
	index int,
	entry *product.PriceBookEntry,
	customer *customerdomain2.Customer,
	quantity int,
	orderType int,
	orderStatus int,
) (orderItem *trade.OrderItem, subUnitTotal float64, subListTotal float64) {

	subUnitTotal = 0.0
	subListTotal = 0.0
	orderItem = &trade.OrderItem{
		PriceBookEntryId: entry.Id,
		CustomerId:       customer.Id,
		Type:             orderType,
		Status:           orderStatus,
		Quantity:         quantity,
		UnitPrice:        entry.UnitPrice,
		ListPrice:        entry.ListPrice,
		Discount:         entry.UnitPrice / entry.ListPrice,
		ProductName:      entry.Product.Name,
		SkuNo:            entry.SKU.SkuNo,
		CoverImageId:     entry.Product.PivotCoverImages[0].MediaResourceId,
	}
	subUnitTotal = orderItem.UnitPrice * float64(orderItem.Quantity)
	subListTotal = orderItem.ListPrice * float64(orderItem.Quantity)

	return orderItem, subUnitTotal, subListTotal
}

func (uc *OrderUseCase) MakeOrderItemsFromCartItems(
	cartItems []*trade.CartItem,
	orderType int,
	orderStatus int,
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
	orderType int,
	orderStatus int,
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
		ProductName:      cartItem.ProductName,
		SkuNo:            cartItem.Specifications,
		CoverImageId:     cartItem.Product.PivotCoverImages[0].MediaResourceId,
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

	err := powermodel.UpsertModelsOnUniqueID(uc.db.WithContext(ctx), &trade.Order{}, trade.OrderUniqueId, orders, nil, false)

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

func (uc *OrderUseCase) ChangeOrderStatusFromTo(ctx context.Context, order *trade.Order,
	fromStatus string, toStatus string,
) (*trade.Order, error) {

	ucDD := powerx.NewDataDictionaryUseCase(uc.db)

	orderStatusId := ucDD.GetCachedDDId(ctx, trade.TypeOrderStatus, fromStatus)
	toStatusId := ucDD.GetCachedDDId(ctx, trade.TypeOrderStatus, toStatus)

	err := uc.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {

		// 修改订单的支付状态记录
		order.Status = toStatusId
		err := tx.Save(order).Error
		if err != nil {
			return err
		}

		// 保存订单状态记录
		changeLog := &trade.OrderStatusTransition{}
		changeLog.OrderId = order.Id
		changeLog.FromStatus = orderStatusId
		changeLog.ToStatus = toStatusId
		err = tx.Create(changeLog).Error

		return err
	})

	return order, err
}

func (uc *OrderUseCase) CanOrderCancel(ctx context.Context, order *trade.Order) bool {
	ucDD := powerx.NewDataDictionaryUseCase(uc.db)
	ddOrderStatus := ucDD.GetCachedDDById(ctx, order.Status)
	availableStatus := []string{
		trade.OrderStatusPending,
		trade.OrderStatusToBePaid,
		trade.OrderStatusConfirmed,
		trade.OrderStatusToBeShipped,
		trade.OrderStatusShipping,
	}
	//fmt.Dump(ddOrderStatus.Key, availableStatus)
	return slicex.Contains(availableStatus, ddOrderStatus.Key)

}

func (uc *OrderUseCase) IsOrderTypeSameAs(ctx context.Context, order *trade.Order, orderType string) bool {
	ucDD := powerx.NewDataDictionaryUseCase(uc.db)

	return order.Type == ucDD.GetCachedDDId(ctx, trade.TypeOrderType, orderType)

}

func (uc *OrderUseCase) IsOrderStatusSameAs(ctx context.Context, order *trade.Order, orderStatus string) bool {
	ucDD := powerx.NewDataDictionaryUseCase(uc.db)

	return order.Status == ucDD.GetCachedDDId(ctx, trade.TypeOrderStatus, orderStatus)

}

func (uc *OrderUseCase) GetOrderTypeId(ctx context.Context, orderType string) (orderTypeId int) {
	ucDD := powerx.NewDataDictionaryUseCase(uc.db)
	orderTypeId = ucDD.GetCachedDDId(ctx, trade.TypeOrderType, orderType)
	return orderTypeId
}
func (uc *OrderUseCase) GetOrderStatusId(ctx context.Context, orderStatus string) (orderStatusId int) {
	ucDD := powerx.NewDataDictionaryUseCase(uc.db)
	orderStatusId = ucDD.GetCachedDDId(ctx, trade.TypeOrderStatus, orderStatus)
	return orderStatusId
}
