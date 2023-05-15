package handler

import (
	"PowerX/internal/handler/mp/static"
	"PowerX/internal/svc"
	"github.com/zeromicro/go-zero/rest"
	"net/http"
)

func RegisterStaticHandlers(server *rest.Server, serverCtx *svc.ServiceContext) {

	server.AddRoutes(
		rest.WithMiddlewares(
			[]rest.Middleware{},
			[]rest.Route{
				{
					Method:  http.MethodGet,
					Path:    "/images/:filename",
					Handler: static.FileHandler("./resource/images/mp"),
				},
			}...,
		),
		rest.WithPrefix("/static"),
	)
}
