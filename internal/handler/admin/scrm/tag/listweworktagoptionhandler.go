package tag

import (
	"net/http"

	"PowerX/internal/logic/admin/scrm/tag"
	"PowerX/internal/svc"
	"github.com/zeromicro/go-zero/rest/httpx"
)

func ListWeWorkTagOptionHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := tag.NewListWeWorkTagOptionLogic(r.Context(), svcCtx)
		resp, err := l.ListWeWorkTagOption()
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
