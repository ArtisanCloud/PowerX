package openapi

import (
	"net/http"

	"PowerX/internal/logic/openapi"
	"PowerX/internal/svc"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// Get the version of the API
func GetVersionHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := openapi.NewGetVersionLogic(r.Context(), svcCtx)
		resp, err := l.GetVersion()
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
