package permission

import (
	"net/http"

	"PowerX/internal/logic/admin/permission"
	"PowerX/internal/svc"
	"github.com/zeromicro/go-zero/rest/httpx"
)

func SetRolePermissionsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := permission.NewSetRolePermissionsLogic(r.Context(), svcCtx)
		err := l.SetRolePermissions()
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.Ok(w)
		}
	}
}
