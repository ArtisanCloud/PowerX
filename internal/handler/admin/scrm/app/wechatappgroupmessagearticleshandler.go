package app

import (
    "net/http"

    "PowerX/internal/logic/admin/scrm/app"
    "PowerX/internal/svc"
    "PowerX/internal/types"
    "github.com/zeromicro/go-zero/rest/httpx"
)

func WechatAppGroupMessageArticlesHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        var req types.AppGroupMessageArticleRequest
        if err := httpx.Parse(r, &req); err != nil {
            httpx.ErrorCtx(r.Context(), w, err)
            return
        }

        l := app.NewWechatAppGroupMessageArticlesLogic(r.Context(), svcCtx)
        resp, err := l.WechatAppGroupMessageArticles(&req)
        if err != nil {
            httpx.ErrorCtx(r.Context(), w, err)
        } else {
            httpx.OkJsonCtx(r.Context(), w, resp)
        }
    }
}
