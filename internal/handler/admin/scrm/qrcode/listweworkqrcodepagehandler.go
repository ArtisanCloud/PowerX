package qrcode

import (
    "net/http"

    "PowerX/internal/logic/admin/scrm/qrcode"
    "PowerX/internal/svc"
    "PowerX/internal/types"
    "github.com/zeromicro/go-zero/rest/httpx"
)

func ListWeWorkQrcodePageHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        var req types.ListWeWorkGroupQrcodeActiveReqeust
        if err := httpx.Parse(r, &req); err != nil {
            httpx.ErrorCtx(r.Context(), w, err)
            return
        }

        l := qrcode.NewListWeWorkQrcodePageLogic(r.Context(), svcCtx)
        resp, err := l.ListWeWorkQrcodePage(&req)
        if err != nil {
            httpx.ErrorCtx(r.Context(), w, err)
        } else {
            httpx.OkJsonCtx(r.Context(), w, resp)
        }
    }
}
