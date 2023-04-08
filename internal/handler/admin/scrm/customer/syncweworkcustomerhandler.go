package customer

import (
	"net/http"

	"PowerX/internal/logic/admin/scrm/customer"
	"PowerX/internal/svc"
	"github.com/zeromicro/go-zero/rest/httpx"
)

func SyncWeWorkCustomerHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := customer.NewSyncWeWorkCustomerLogic(r.Context(), svcCtx)
		resp, err := l.SyncWeWorkCustomer()
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
