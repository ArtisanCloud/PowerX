package payment

import (
	"PowerX/internal/model/trade"
	"PowerX/internal/types/errorx"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetPaymentLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetPaymentLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetPaymentLogic {
	return &GetPaymentLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetPaymentLogic) GetPayment(req *types.GetPaymentRequest) (resp *types.GetPaymentReply, err error) {
	mdlPayment, err := l.svcCtx.PowerX.Payment.GetPayment(l.ctx, req.PaymentId)

	if err != nil {
		return nil, errorx.ErrNotFoundObject
	}

	return &types.GetPaymentReply{
		Payment: TransformPaymentToReply(mdlPayment),
	}, nil
}

func TransformPaymentToReply(mdlPayment *trade.Payment) (paymentReply *types.Payment) {

	return &types.Payment{
		Id:              mdlPayment.Id,
		OrderId:         mdlPayment.OrderId,
		PaymentDate:     mdlPayment.PaymentDate.String(),
		PaymentType:     int(mdlPayment.PaymentType),
		Status:          int(mdlPayment.Status),
		PaidAmount:      mdlPayment.PaidAmount,
		PaymentNumber:   mdlPayment.PaymentNumber,
		ReferenceNumber: mdlPayment.ReferenceNumber,

		PaymentItems: TransformPaymentItemsToReply(mdlPayment.Items),
	}

}

func TransformPaymentItemsToReply(paymentItems []*trade.PaymentItem) (paymentItemsReply []*types.PaymentItem) {

	paymentItemsReply = []*types.PaymentItem{}
	for _, paymentItem := range paymentItems {
		paymentItemReply := TransformPaymentItemToReply(paymentItem)
		paymentItemsReply = append(paymentItemsReply, paymentItemReply)
	}
	return paymentItemsReply
}

func TransformPaymentItemToReply(paymentItem *trade.PaymentItem) (paymentItemReply *types.PaymentItem) {
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
