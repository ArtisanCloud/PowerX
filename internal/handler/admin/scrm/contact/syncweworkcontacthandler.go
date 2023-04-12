package contact

import (
	"net/http"

	"PowerX/internal/logic/admin/scrm/contact"
	"PowerX/internal/svc"
	"github.com/zeromicro/go-zero/rest/httpx"
)

func SyncWeWorkContactHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := contact.NewSyncWeWorkContactLogic(r.Context(), svcCtx)
		resp, err := l.SyncWeWorkContact()
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
