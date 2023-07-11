package resource

import (
    "net/http"

    "PowerX/internal/logic/admin/scrm/resource"
    "PowerX/internal/svc"
    "github.com/zeromicro/go-zero/rest/httpx"
)

func CreateWeWorkImageHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        l := resource.NewCreateWeWorkImageLogic(r.Context(), svcCtx)
        resp, err := l.CreateWeWorkImage(r)
        if err != nil {
            httpx.ErrorCtx(r.Context(), w, err)
        } else {
            httpx.OkJsonCtx(r.Context(), w, resp)
        }
    }
}
