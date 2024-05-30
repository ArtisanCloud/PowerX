package organization

import (
	"net/http"

	"PowerX/internal/logic/admin/scrm/organization"
	"PowerX/internal/svc"
	"github.com/zeromicro/go-zero/rest/httpx"
)

func SyncWeWorkUserHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := organization.NewSyncWeWorkUserLogic(r.Context(), svcCtx)
		resp, err := l.SyncWeWorkUser()
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
