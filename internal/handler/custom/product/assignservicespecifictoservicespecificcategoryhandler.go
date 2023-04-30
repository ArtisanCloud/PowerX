package product

import (
	"net/http"

	"PowerX/internal/logic/custom/product"
	"PowerX/internal/svc"
	"PowerX/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
)

func AssignServiceSpecificToServiceSpecificCategoryHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.AssignServiceSpecificManagerRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		l := product.NewAssignServiceSpecificToServiceSpecificCategoryLogic(r.Context(), svcCtx)
		resp, err := l.AssignServiceSpecificToServiceSpecificCategory(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
