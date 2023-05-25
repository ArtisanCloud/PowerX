package payment

import (
	"PowerX/internal/model/trade"
	"PowerX/internal/svc"
	fmt "PowerX/pkg/printx"
	"context"
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
	//fmt.DD(r)
	//body, _ := io.ReadAll(r.Body)
	//fmt.Dump(string(body))
	//
	//defer r.Body.Close()

	return func(message *request.RequestNotify, transaction *models.Transaction, fail func(message string)) interface{} {
		//fmt2.Dump("payload here:", message.EventType, transaction)

		if transaction == nil || transaction.OutTradeNo == "" {
			return "no content notify"
		}

		// 获取该支付单
		payment, err := srv.svcCtx.PowerX.Payment.GetPaymentByNumber(srv.ctx, transaction.OutTradeNo)
		if err != nil {
			return err.Error()
		}

		// 将该未支付完成的订单，修改状态
		if payment.IsStatusToBePaid() && message.EventType == "TRANSACTION.SUCCESS" {
			payment, err = srv.svcCtx.PowerX.Payment.ChangePaymentStatusPaid(srv.ctx, payment)
			if err != nil {
				return err
			}

			// order如果状态修改出错，可以在另外的机制处理，不能敢于payment的记录状态
			_, err := srv.svcCtx.PowerX.Order.ChangeOrderStatusFromTo(srv.ctx, payment.Order, trade.OrderStatusToBePaid, trade.OrderStatusToBeShipped)
			if err != nil {
				fmt.Dump(err.Error())
			}

			// 如果需要做其他的事件，可以通过消息队列方式，异步去处理订单所产生的业务变化
			// 这里只做支付单的记录和状态变更
			// ...

			return true
		}

		return true
	}

}
