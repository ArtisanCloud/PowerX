package userinfo

import (
	"net/http"

	"PowerX/internal/logic/admin/userinfo"
	"PowerX/internal/svc"
	"github.com/zeromicro/go-zero/rest/httpx"
)

func GetMenuRolesHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := userinfo.NewGetMenuRolesLogic(r.Context(), svcCtx)
		resp, err := l.GetMenuRoles()
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
