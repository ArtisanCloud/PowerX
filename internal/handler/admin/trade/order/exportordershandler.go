package order

import (
	"fmt"
	"net/http"

	"PowerX/internal/logic/admin/trade/order"
	"PowerX/internal/svc"
	"PowerX/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
)

func ExportOrdersHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.ExportOrdersRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		l := order.NewExportOrdersLogic(r.Context(), svcCtx)
		resp, err := l.ExportOrders(&req)

		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		// 设置HTTP响应头
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", resp.FileName))
		w.Header().Set("Content-Type", resp.FileType)
		w.Header().Set("Content-Length", fmt.Sprint(resp.FileSize))

		_, err = w.Write(resp.Content)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		}
	}
}
