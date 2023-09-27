package pricebookentry

import (
	"net/http"

	"PowerX/internal/logic/admin/crm/product/pricebookentry"
	"PowerX/internal/svc"
	"PowerX/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
)

func ConfigPriceBookEntryHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.ConfigPriceBookEntryRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		l := pricebookentry.NewConfigPriceBookEntryLogic(r.Context(), svcCtx)
		resp, err := l.ConfigPriceBookEntry(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
