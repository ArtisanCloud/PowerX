package employee

import (
	"net/http"

	"PowerX/internal/logic/admin/employee"
	"PowerX/internal/svc"
	"github.com/zeromicro/go-zero/rest/httpx"
)

func GetEmployeeOptionsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := employee.NewGetEmployeeOptionsLogic(r.Context(), svcCtx)
		resp, err := l.GetEmployeeOptions()
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
