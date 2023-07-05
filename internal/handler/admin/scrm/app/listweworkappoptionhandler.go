package app

import (
	"net/http"

	"PowerX/internal/logic/admin/scrm/app"
	"PowerX/internal/svc"
	"github.com/zeromicro/go-zero/rest/httpx"
)

func ListWeWorkAppOptionHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := app.NewListWeWorkAppOptionLogic(r.Context(), svcCtx)
		resp, err := l.ListWeWorkAppOption()
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
