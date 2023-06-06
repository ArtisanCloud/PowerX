package handler

import (
	"PowerX/internal/handler/mp/static"
	"PowerX/internal/svc"
	"github.com/zeromicro/go-zero/rest"
	"net/http"
	"path/filepath"
	"strings"
)

func RegisterStaticHandlers(server *rest.Server, serverCtx *svc.ServiceContext) {

	uri := filepath.Join("/", serverCtx.PowerX.MediaResource.LocalStoragePath, ":bucket", ":filename")
	handlerUri := filepath.Join("./", serverCtx.PowerX.MediaResource.LocalStoragePath)
	//fmt.Dump(uri, handlerUri)
	uri = strings.ReplaceAll(uri, `\`, `/`)
	handlerUri = `./` + strings.ReplaceAll(handlerUri, `\`, `/`)
	server.AddRoutes(
		rest.WithMiddlewares(
			[]rest.Middleware{},
			[]rest.Route{
				{
					Method:  http.MethodGet,
					Path:    uri,
					Handler: static.FileHandler(handlerUri),
				},
			}...,
		),
		//rest.WithPrefix("/static"),
	)
}
