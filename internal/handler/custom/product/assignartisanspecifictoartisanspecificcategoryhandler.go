package product

import (
	"net/http"

	"PowerX/internal/logic/custom/product"
	"PowerX/internal/svc"
	"PowerX/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
)

func AssignArtisanSpecificToArtisanSpecificCategoryHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.AssignArtisanSpecificManagerRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		l := product.NewAssignArtisanSpecificToArtisanSpecificCategoryLogic(r.Context(), svcCtx)
		resp, err := l.AssignArtisanSpecificToArtisanSpecificCategory(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
