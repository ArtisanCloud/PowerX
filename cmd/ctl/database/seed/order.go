package seed

import (
	"PowerX/internal/model/crm/trade"
	"PowerX/internal/uc/powerx"
	"context"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

func CreateOrders(db *gorm.DB) (err error) {

	var count int64
	if err = db.Model(&trade.Order{}).Count(&count).Error; err != nil {
		panic(errors.Wrap(err, "init order failed"))
	}

	data := DefaultOrder(db)
	if count == 0 {
		if err = db.Model(&trade.Order{}).Create(data).Error; err != nil {
			panic(errors.Wrap(err, "init price category failed"))
		}

	}
	return err
}

func DefaultOrder(db *gorm.DB) (data []*trade.Order) {

	//ucOrder := trade2.NewOrderUseCase(db)
	ucDD := powerx.NewDataDictionaryUseCase(db)

	orderTypeGoods := ucDD.GetCachedDD(context.Background(), trade.TypeOrderType, trade.OrderTypeNormal)
	paymentTypeWechat := ucDD.GetCachedDD(context.Background(), trade.TypePaymentType, trade.PaymentTypeWeChat)
	orderStatusToBePaid := ucDD.GetCachedDD(context.Background(), trade.TypeOrderStatus, trade.OrderStatusToBePaid)
	orderStatusToBeShipped := ucDD.GetCachedDD(context.Background(), trade.TypeOrderStatus, trade.OrderStatusToBeShipped)

	//now := carbon.Now()
	data = []*trade.Order{}

	for i := 0; i < 20; i++ {
		orderStatus := int(orderStatusToBePaid.Id)
		if i%2 == 0 {
			orderStatus = int(orderStatusToBeShipped.Id)
		}

		seedOrder := &trade.Order{
			CustomerId:     0,
			CartId:         0,
			PaymentType:    int(paymentTypeWechat.Id),
			Type:           int(orderTypeGoods.Id),
			Status:         orderStatus,
			OrderNumber:    trade.GenerateOrderNumber(),
			UnitPrice:      100.00,
			ListPrice:      120,
			Discount:       0.8,
			Comment:        "种子货品",
			ShippingMethod: "普通快递",
		}

		seedOrder.Items = DefaultOrderItems(seedOrder)

		data = append(data, seedOrder)
	}

	return data
}

func DefaultOrderItems(order *trade.Order) []*trade.OrderItem {

	items := []*trade.OrderItem{
		{

			PriceBookEntryId: 1,
			CustomerId:       0,
			CoverImageId:     1,
			Type:             order.Type,
			Status:           order.Status,
			ProductName:      "测试订单项目",
			SkuNo:            "SKU-test",
			Quantity:         1,
			UnitPrice:        50,
			ListPrice:        60,
			Discount:         order.Discount,
		},
		{

			PriceBookEntryId: 2,
			CustomerId:       0,
			CoverImageId:     2,
			Type:             order.Type,
			Status:           order.Status,
			ProductName:      "测试订单项目2",
			SkuNo:            "SKU-test2",
			Quantity:         2,
			UnitPrice:        25,
			ListPrice:        30,
			Discount:         order.Discount,
		},
	}
	return items

}
