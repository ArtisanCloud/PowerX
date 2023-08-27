package order

import (
	"PowerX/internal/model/trade"
	"net/http"

	"PowerX/internal/logic/admin/trade/order"
	"PowerX/internal/svc"
	"github.com/zeromicro/go-zero/rest/httpx"
)

func ImportOrdersHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := order.NewImportOrdersLogic(r.Context(), svcCtx)

		l.OrderStatusToBeShippedId = svcCtx.PowerX.DataDictionary.GetCachedDDId(r.Context(), trade.TypeOrderStatus, trade.OrderStatusToBeShipped)
		l.OrderStatusShippingId = svcCtx.PowerX.DataDictionary.GetCachedDDId(r.Context(), trade.TypeOrderStatus, trade.OrderStatusShipping)

		resp, err := l.ImportOrders(r)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
