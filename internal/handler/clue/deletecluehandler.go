package clue

import (
	"net/http"

	"PowerX/internal/logic/clue"
	"PowerX/internal/svc"
	"github.com/zeromicro/go-zero/rest/httpx"
)

func DeleteClueHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := clue.NewDeleteClueLogic(r.Context(), svcCtx)
		resp, err := l.DeleteClue()
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
