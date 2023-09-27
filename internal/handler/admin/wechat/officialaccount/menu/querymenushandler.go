package menu

import (
	"net/http"

	"PowerX/internal/logic/admin/wechat/officialaccount/menu"
	"PowerX/internal/svc"
	"github.com/zeromicro/go-zero/rest/httpx"
)

func QueryMenusHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := menu.NewQueryMenusLogic(r.Context(), svcCtx)
		resp, err := l.QueryMenus()
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
