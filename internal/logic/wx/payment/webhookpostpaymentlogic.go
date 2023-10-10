package payment

import (
	"PowerX/internal/logic/mp/crm/trade/payment"
	"PowerX/internal/svc"
	"context"
	"github.com/zeromicro/go-zero/core/logx"
	"net/http"
)

type WXPostPaymentLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewWXPostPaymentLogic(ctx context.Context, svcCtx *svc.ServiceContext) *WXPostPaymentLogic {
	return &WXPostPaymentLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *WXPostPaymentLogic) WebhookWXPostPayment(w http.ResponseWriter, r *http.Request) {

	handleWXPaidLogic := payment.NewHandleWXPaidLogic(l.ctx, l.svcCtx)
	res, err := l.svcCtx.PowerX.Payment.WXPayment.HandlePaidNotify(r, handleWXPaidLogic.HandleWXPaid(w, r))

	// 这里可能是因为不是微信官方调用的，无法正常解析出transaction和message，所以直接抛错。
	if err != nil {
		panic(err)
	}

	// 这里根据之前返回的是true或者fail，框架这边自动会帮你回复微信
	err = res.Write(w)

	return
}
