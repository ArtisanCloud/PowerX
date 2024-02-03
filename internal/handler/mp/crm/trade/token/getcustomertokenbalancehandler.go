package token

import (
	"net/http"

	"PowerX/internal/logic/mp/crm/trade/token"
	"PowerX/internal/svc"
	"github.com/zeromicro/go-zero/rest/httpx"
)

func GetCustomerTokenBalanceHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := token.NewGetCustomerTokenBalanceLogic(r.Context(), svcCtx)
		resp, err := l.GetCustomerTokenBalance()
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
