package tag

import (
	"net/http"

	"PowerX/internal/logic/admin/scrm/tag"
	"PowerX/internal/svc"
	"github.com/zeromicro/go-zero/rest/httpx"
)

func SyncWeWorkGroupTagHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := tag.NewSyncWeWorkGroupTagLogic(r.Context(), svcCtx)
		resp, err := l.SyncWeWorkGroupTag()
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
