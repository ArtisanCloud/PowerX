package customer

import (
	"net/http"

	"PowerX/internal/logic/admin/customer"
	"PowerX/internal/svc"
	"github.com/zeromicro/go-zero/rest/httpx"
)

func ListCustomersHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := customer.NewListCustomersLogic(r.Context(), svcCtx)
		err := l.ListCustomers()
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.Ok(w)
		}
	}
}
