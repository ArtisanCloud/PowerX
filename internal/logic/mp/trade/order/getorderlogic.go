package order

import (
	"PowerX/internal/model/trade"
	"context"
	"math"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetOrderLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetOrderLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetOrderLogic {
	return &GetOrderLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetOrderLogic) GetOrder(req *types.GetOrderRequest) (resp *types.GetOrderReply, err error) {
	// todo: add your logic here and delete this line

	return
}

func TransformOrderToOrderReplyToMP(order *trade.Order) *types.Order {
	if order == nil {
		return nil
	}
	discount := order.Discount
	if math.IsNaN(discount) {
		discount = 0
	}
	return &types.Order{
		Id:          order.Id,
		CustomerId:  order.CustomerId,
		PaymentType: order.PaymentType,
		Type:        int8(order.Type),
		Status:      int8(order.Status),
		OrderNumber: order.OrderNumber,
		Discount:    discount,
		ListPrice:   order.ListPrice,
		UnitPrice:   order.UnitPrice,
		Comment:     order.Comment,
		OrderItems:  TransformOrderItemsToOrderItemsReplyToMP(order.Items),
		Payments:    TransformPaymentsToPaymentsReplyToMP(order.Payments),
	}
}

func TransformOrderItemsToOrderItemsReplyToMP(orderItems []*trade.OrderItem) (orderItemsReply []*types.OrderItem) {
	orderItemsReply = []*types.OrderItem{}
	for _, orderItem := range orderItems {
		orderItemReply := TransformOrderItemToOrderItemReplyToMP(orderItem)
		orderItemsReply = append(orderItemsReply, orderItemReply)

	}
	return orderItemsReply
}

func TransformOrderItemToOrderItemReplyToMP(orderItem *trade.OrderItem) (orderItemReply *types.OrderItem) {
	if orderItem == nil {
		return nil
	}
	return &types.OrderItem{
		Id:               orderItem.Id,
		OrderId:          orderItem.OrderId,
		PriceBookEntryId: orderItem.PriceBookEntryId,
		CustomerId:       orderItem.CustomerId,
		Type:             int(orderItem.Type),
		Status:           int(orderItem.Status),
		Quantity:         orderItem.Quantity,
		UnitPrice:        orderItem.UnitPrice,
		ListPrice:        orderItem.ListPrice,
		CoverUrl:         orderItem.CoverImage.Url,
		ProdcutName:      orderItem.ProductName,
		SkuNo:            orderItem.SkuNo,
	}
}

func TransformPaymentsToPaymentsReplyToMP(payments []*trade.Payment) (paymentsReply []*types.Payment) {
	paymentsReply = []*types.Payment{}
	for _, payment := range payments {
		paymentReply := TransformPaymentToPaymentReplyToMP(payment)
		paymentsReply = append(paymentsReply, paymentReply)

	}
	return paymentsReply
}

func TransformPaymentToPaymentReplyToMP(payment *trade.Payment) (paymentReply *types.Payment) {
	if payment == nil {
		return nil
	}
	paymentReply = &types.Payment{
		Id:              payment.Id,
		OrderId:         payment.OrderId,
		PaymentDate:     payment.PaymentDate.String(),
		PaymentType:     int(payment.PaymentType),
		PaidAmount:      payment.PaidAmount,
		PaymentNumber:   payment.PaymentNumber,
		ReferenceNumber: payment.ReferenceNumber,
		Status:          int(payment.Status),
	}
	if len(payment.Items) > 0 {
		paymentReply.PaymentItems = TransformPaymentItemsToPaymentItemsReplyToMP(payment.Items)
	}
	return paymentReply

}

func TransformPaymentItemsToPaymentItemsReplyToMP(paymentItems []*trade.PaymentItem) (paymentItemsReply []*types.PaymentItem) {
	paymentItemsReply = []*types.PaymentItem{}
	for _, paymentItem := range paymentItems {
		paymentItemReply := TransformPaymentItemToPaymentItemReplyToMP(paymentItem)
		paymentItemsReply = append(paymentItemsReply, paymentItemReply)

	}
	return paymentItemsReply
}

func TransformPaymentItemToPaymentItemReplyToMP(paymentItem *trade.PaymentItem) (paymentItemReply *types.PaymentItem) {
	if paymentItem == nil {
		return nil
	}
	return &types.PaymentItem{
		Id:                  paymentItem.Id,
		PaymentID:           paymentItem.PaymentID,
		Quantity:            paymentItem.Quantity,
		UnitPrice:           paymentItem.UnitPrice,
		PaymentCustomerName: paymentItem.PaymentCustomerName,
		BankInformation:     paymentItem.BankInformation,
		BankResponseCode:    paymentItem.BankResponseCode,
		CarrierType:         paymentItem.CarrierType,
		CreditCardNumber:    paymentItem.CreditCardNumber,
		DeductMembershipId:  paymentItem.DeductMembershipId,
		DeductionPoint:      paymentItem.DeductionPoint,
		InvoiceCreateTime:   paymentItem.InvoiceCreateTime.String(),
		InvoiceNumber:       paymentItem.InvoiceNumber,
		InvoiceTotalAmount:  paymentItem.InvoiceTotalAmount,
		TaxIdNumber:         paymentItem.TaxIdNumber,
	}
}
