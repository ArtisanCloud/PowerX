package bot

import (
	"net/http"

	"PowerX/internal/logic/admin/scrm/bot"
	"PowerX/internal/svc"
	"PowerX/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
)

func WechatBotArticlesHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.GroupRobotMsgNewsArticlesRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		l := bot.NewWechatBotArticlesLogic(r.Context(), svcCtx)
		resp, err := l.WechatBotArticles(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
