package common

import (
	"net/http"

	"PowerX/internal/logic/admin/common"
	"PowerX/internal/svc"
	"github.com/zeromicro/go-zero/rest/httpx"
)

func GetEmployeeQueryOptionsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := common.NewGetEmployeeQueryOptionsLogic(r.Context(), svcCtx)
		resp, err := l.GetEmployeeQueryOptions()
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
