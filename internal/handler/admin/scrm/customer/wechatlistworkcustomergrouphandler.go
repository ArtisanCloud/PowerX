package customer

import (
	"net/http"

	"PowerX/internal/logic/admin/scrm/customer"
	"PowerX/internal/svc"
	"PowerX/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
)

func WechatListWorkCustomerGroupHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.WechatCustomerGroupRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		l := customer.NewWechatListWorkCustomerGroupLogic(r.Context(), svcCtx)
		resp, err := l.WechatListWorkCustomerGroup(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
