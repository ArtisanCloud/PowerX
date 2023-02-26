package customer

import (
	"net/http"

	"PowerX/internal/logic/customer"
	"PowerX/internal/svc"
	"github.com/zeromicro/go-zero/rest/httpx"
)

func BatchSyncCustomersHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := customer.NewBatchSyncCustomersLogic(r.Context(), svcCtx)
		err := l.BatchSyncCustomers()
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.Ok(w)
		}
	}
}
