package organization

import (
    "net/http"

    "PowerX/internal/logic/admin/scrm/organization"
    "PowerX/internal/svc"
    "PowerX/internal/types"
    "github.com/zeromicro/go-zero/rest/httpx"
)

func WechatListWorkEmployeeHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        var req types.ListWechatWorkEmployeeReqeust
        if err := httpx.Parse(r, &req); err != nil {
            httpx.ErrorCtx(r.Context(), w, err)
            return
        }

        l := organization.NewWechatListWorkEmployeeLogic(r.Context(), svcCtx)
        resp, err := l.WechatListWorkEmployee(&req)
        if err != nil {
            httpx.ErrorCtx(r.Context(), w, err)
        } else {
            httpx.OkJsonCtx(r.Context(), w, resp)
        }
    }
}
