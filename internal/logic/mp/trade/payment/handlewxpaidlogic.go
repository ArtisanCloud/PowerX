package payment

import (
	"PowerX/internal/model/trade"
	"PowerX/internal/svc"
	"context"
	"fmt"
	"github.com/ArtisanCloud/PowerWeChat/v3/src/kernel/models"
	"github.com/ArtisanCloud/PowerWeChat/v3/src/payment/notify/request"
	"github.com/zeromicro/go-zero/core/logx"
	"net/http"
)

type HandleWXPaidLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewHandleWXPaidLogic(ctx context.Context, svcCtx *svc.ServiceContext) *HandleWXPaidLogic {
	return &HandleWXPaidLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (srv *HandleWXPaidLogic) HandleWXPaid(w http.ResponseWriter, r *http.Request) func(message *request.RequestNotify, transaction *models.Transaction, fail func(message string)) interface{} {

	//body, _ := io.ReadAll(r.Body)
	//r.Body = io.NopCloser(bytes.NewBuffer(body))
	//fmt2.Dump(string(body))
	//defer r.Body.Close()

	return func(message *request.RequestNotify, transaction *models.Transaction, fail func(message string)) interface{} {
		//fmt2.Dump("payload here:", message.EventType, transaction)

		if transaction == nil || transaction.OutTradeNo == "" {
			return "no content notify"
		}

		// 获取该支付单
		payment, err := srv.svcCtx.PowerX.Payment.GetPaymentByNumber(srv.ctx, transaction.OutTradeNo)
		if err != nil {
			errorMsg := fmt.Sprintf("微信支付回调-获取支付单号:%s,错误信息：%s", transaction.OutTradeNo, err.Error())
			srv.Logger.Error(errorMsg)
			return errorMsg
		}

		// 将该未支付完成的订单，修改状态
		//if payment.IsStatusToBePaid() && message.EventType == "TRANSACTION.SUCCESS" {
		if message.EventType == "TRANSACTION.SUCCESS" {
			payment, err = srv.svcCtx.PowerX.Payment.ChangePaymentStatusPaid(srv.ctx, payment)
			if err != nil {
				errorMsg := fmt.Sprintf("微信支付回调-修改订单状态:%s,错误信息：%s", payment.PaymentNumber, err.Error())
				srv.Logger.Error(errorMsg)
				return errorMsg
			}

			// order如果状态修改出错，可以在另外的机制处理，不能干预payment的记录状态
			_, err := srv.svcCtx.PowerX.Order.ChangeOrderStatusFromTo(srv.ctx, payment.Order, trade.OrderStatusToBePaid, trade.OrderStatusToBeShipped)
			if err != nil {
				errorMsg := fmt.Sprintf("微信支付回调-记录订单状态跳变:%s,错误信息：%s", payment.PaymentNumber, err.Error())
				srv.Logger.Error(errorMsg)
			}

			// 如果需要做其他的事件，可以通过消息队列方式，异步去处理订单所产生的业务变化
			// 这里只做支付单的记录和状态变更
			// ...

			return true
		}

		return true
	}

}
