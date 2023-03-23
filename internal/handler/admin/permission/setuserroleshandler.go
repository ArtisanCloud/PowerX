package permission

import (
	"net/http"

	"PowerX/internal/logic/admin/permission"
	"PowerX/internal/svc"
	"github.com/zeromicro/go-zero/rest/httpx"
)

func SetUserRolesHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := permission.NewSetUserRolesLogic(r.Context(), svcCtx)
		err := l.SetUserRoles()
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.Ok(w)
		}
	}
}
