package recovery

import (
	"PowerX/internal/types/errorx"
	"fmt"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/rest"
	"github.com/zeromicro/go-zero/rest/httpx"
	"net/http"
	"runtime/debug"
)

func RecoverMiddleware() rest.Middleware {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if result := recover(); result != nil {
					unknown := errorx.ErrUnKnow
					logx.WithContext(r.Context()).Error(formatReq(r, fmt.Sprintf("%v\n%s", result, debug.Stack())))
					httpx.Error(w, unknown)
				}
			}()
			next(w, r)
		}
	}
}

func formatReq(r *http.Request, v ...interface{}) string {
	return fmt.Sprintf("(%s - %s) %s", r.RequestURI, httpx.GetRemoteAddr(r), fmt.Sprint(v))
}
