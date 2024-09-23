package brainx

import (
	"net/http"

	"PowerX/internal/logic/openapi/provider/brainx"
	"PowerX/internal/svc"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// hello world api for provider demo
func HelloWorldHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := brainx.NewHelloWorldLogic(r.Context(), svcCtx)
		resp, err := l.HelloWorld()
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
