package common

import (
	"net/http"

	"PowerX/internal/logic/admin/scrm/common"
	"PowerX/internal/svc"
	"github.com/zeromicro/go-zero/rest/httpx"
)

func AvailabilityCheckHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := common.NewAvailabilityCheckLogic(r.Context(), svcCtx)
		err := l.AvailabilityCheck()
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.Ok(w)
		}
	}
}
