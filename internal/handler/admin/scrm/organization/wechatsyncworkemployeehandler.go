package organization

import (
	"net/http"

	"PowerX/internal/logic/admin/scrm/organization"
	"PowerX/internal/svc"
	"github.com/zeromicro/go-zero/rest/httpx"
)

func WechatSyncWorkEmployeeHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := organization.NewWechatSyncWorkEmployeeLogic(r.Context(), svcCtx)
		resp, err := l.WechatSyncWorkEmployee()
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
