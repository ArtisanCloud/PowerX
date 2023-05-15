package media

import (
	"net/http"

	"PowerX/internal/logic/admin/media"
	"PowerX/internal/svc"
	"github.com/zeromicro/go-zero/rest/httpx"
)

func CreateMediaResourceHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := media.NewCreateMediaResourceLogic(r.Context(), svcCtx)
		resp, err := l.CreateMediaResource(r)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
