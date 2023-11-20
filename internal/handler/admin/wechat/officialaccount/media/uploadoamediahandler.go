package media

import (
	"net/http"

	"PowerX/internal/logic/admin/wechat/officialaccount/media"
	"PowerX/internal/svc"
	"github.com/zeromicro/go-zero/rest/httpx"
)

func UploadOAMediaHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := media.NewUploadOAMediaLogic(r.Context(), svcCtx)
		resp, err := l.UploadOAMedia(r)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
