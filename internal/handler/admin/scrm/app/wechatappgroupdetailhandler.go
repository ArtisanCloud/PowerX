package app

import (
	"net/http"

	"PowerX/internal/logic/admin/scrm/app"
	"PowerX/internal/svc"
	"PowerX/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
)

func WechatAppGroupDetailHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.AppGroupListRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		l := app.NewWechatAppGroupDetailLogic(r.Context(), svcCtx)
		resp, err := l.WechatAppGroupDetail(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
