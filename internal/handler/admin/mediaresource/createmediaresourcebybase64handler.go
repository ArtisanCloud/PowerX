package mediaresource

import (
	"net/http"

	"PowerX/internal/logic/admin/mediaresource"
	"PowerX/internal/svc"
	"PowerX/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
)

func CreateMediaResourceByBase64Handler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.CreateMediaResourceByBase64Request
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		l := mediaresource.NewCreateMediaResourceByBase64Logic(r.Context(), svcCtx)
		resp, err := l.CreateMediaResourceByBase64(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
