package handler

import (
	"PowerX/internal/handler/webhook/wework"
	"PowerX/internal/svc"
	"github.com/zeromicro/go-zero/rest"
	"net/http"
)

func RegisterWebhookHandlers(server *rest.Server, serverCtx *svc.ServiceContext) {

	server.AddRoutes(
		rest.WithMiddlewares(
			[]rest.Middleware{},
			[]rest.Route{
				{
					Method:  http.MethodGet,
					Path:    "/message",
					Handler: wework.GetMessageHandler(serverCtx),
				},
				{
					Method:  http.MethodPost,
					Path:    "/message",
					Handler: wework.PostMessageHandler(serverCtx),
				},
			}...,
		),
		rest.WithPrefix("/webhook/wework"),
	)
}
