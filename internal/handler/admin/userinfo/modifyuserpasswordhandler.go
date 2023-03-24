package userinfo

import (
	"net/http"

	"PowerX/internal/logic/admin/userinfo"
	"PowerX/internal/svc"
	"github.com/zeromicro/go-zero/rest/httpx"
)

func ModifyUserPasswordHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := userinfo.NewModifyUserPasswordLogic(r.Context(), svcCtx)
		err := l.ModifyUserPassword()
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.Ok(w)
		}
	}
}
