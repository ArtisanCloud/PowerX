package payment

import (
	"PowerX/internal/model/trade"
	tradeUC "PowerX/internal/uc/powerx/trade"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListPaymentsPageLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListPaymentsPageLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListPaymentsPageLogic {
	return &ListPaymentsPageLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListPaymentsPageLogic) ListPaymentsPage(req *types.ListPaymentsPageRequest) (resp *types.ListPaymentsPageReply, err error) {
	page, err := l.svcCtx.PowerX.Payment.FindManyPayments(l.ctx, &tradeUC.FindManyPaymentsOption{
		PageEmbedOption: types.PageEmbedOption{
			PageIndex: req.PageIndex,
			PageSize:  req.PageSize,
		},
	})
	if err != nil {
		return nil, err
	}

	// list
	list := TransformPaymentsToReply(page.List)
	return &types.ListPaymentsPageReply{
		List:      list,
		PageIndex: page.PageIndex,
		PageSize:  page.PageSize,
		Total:     page.Total,
	}, nil
}

func TransformPaymentsToReply(payments []*trade.Payment) []*types.Payment {
	paymentsReply := []*types.Payment{}
	for _, payment := range payments {

		paymentReply := TransformPaymentToPaymentReply(payment)
		paymentsReply = append(paymentsReply, paymentReply)

	}
	return paymentsReply
}
