package payment

import (
	customerdomain2 "PowerX/internal/model/customerdomain"
	"PowerX/internal/model/trade"
	"PowerX/internal/types/errorx"
	"PowerX/internal/uc/powerx/customerdomain"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreatePaymentFromOrderLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreatePaymentFromOrderLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreatePaymentFromOrderLogic {
	return &CreatePaymentFromOrderLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreatePaymentFromOrderLogic) CreatePaymentFromOrder(req *types.CreatePaymentFromOrderRequest) (resp *types.CreatePaymentFromOrderRequestReply, err error) {
	vAuthCustomer := l.ctx.Value(customerdomain.AuthCustomerKey)
	authCustomer := vAuthCustomer.(*customerdomain2.Customer)

	order, err := l.svcCtx.PowerX.Order.GetOrder(l.ctx, req.OrderId)
	if err != nil {
		return nil, errorx.WithCause(errorx.ErrNotFoundObject, "未找到该订单")
	}
	if order.CustomerId != authCustomer.Id {
		return nil, errorx.WithCause(errorx.ErrBadRequest, "无权操作该订单")
	}
	if !l.svcCtx.PowerX.Order.IsOrderStatusSameAs(l.ctx, order, trade.OrderStatusToBePaid) {
		return nil, errorx.WithCause(errorx.ErrBadRequest, "该订单不属于待支付状态")
	}

	// 定义支付单
	var createdPayment *trade.Payment
	// 支付平台的返回信息
	var data interface{}
	// 微信支付

	ddPaymentType := l.svcCtx.PowerX.DataDictionary.GetCachedDDById(l.ctx, req.PaymentType)

	switch ddPaymentType.Key {
	case trade.PaymentTypeWeChat:
		// 创建一条支付单
		createdPayment, data, err = l.svcCtx.PowerX.Payment.CreatePaymentFromOrderByWechat(l.ctx,
			authCustomer, order,
			authCustomer.OpenIdInMiniProgram, req.PaymentType,
		)
		if err != nil {
			return nil, errorx.WithCause(errorx.ErrCreateObject, "创建微信小程序订单失败:"+err.Error())
		}

	default:
		return nil, errorx.WithCause(errorx.ErrBadRequest, "支付类型不支持")
	}

	return &types.CreatePaymentFromOrderRequestReply{
		PaymentId: createdPayment.Id,
		Data:      data,
	}, nil
}
