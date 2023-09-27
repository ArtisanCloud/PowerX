package order

import (
	"net/http"

	"PowerX/internal/logic/admin/crm/trade/order"
	"PowerX/internal/svc"
	"github.com/zeromicro/go-zero/rest/httpx"
)

func ImportOrdersHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := order.NewImportOrdersLogic(r.Context(), svcCtx)
		resp, err := l.ImportOrders(r)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
