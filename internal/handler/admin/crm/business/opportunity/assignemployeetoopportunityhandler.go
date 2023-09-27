package opportunity

import (
	"net/http"

	"PowerX/internal/logic/admin/crm/business/opportunity"
	"PowerX/internal/svc"
	"PowerX/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
)

func AssignEmployeeToOpportunityHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.AssignEmployeeToOpportunityRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		l := opportunity.NewAssignEmployeeToOpportunityLogic(r.Context(), svcCtx)
		resp, err := l.AssignEmployeeToOpportunity(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
