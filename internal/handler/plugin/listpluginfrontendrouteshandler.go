package plugin

import (
	"net/http"

	"PowerX/internal/logic/plugin"
	"PowerX/internal/svc"
	"github.com/zeromicro/go-zero/rest/httpx"
)

func ListPluginFrontendRoutesHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := plugin.NewListPluginFrontendRoutesLogic(r.Context(), svcCtx)
		resp, err := l.ListPluginFrontendRoutes()
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
