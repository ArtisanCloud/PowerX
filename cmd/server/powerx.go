package main

import (
	"PowerX/internal/middleware/recovery"
	"flag"
	"fmt"
	"github.com/zeromicro/go-zero/rest/httpx"
	"path"

	"PowerX/internal/config"
	"PowerX/internal/handler"
	"PowerX/internal/svc"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/rest"
)

var configFile = flag.String("f", "etc/powerx.yaml", "the config file")

func main() {
	flag.Parse()

	var c config.Config
	conf.MustLoad(*configFile, &c)

	c.EtcDir = path.Dir(*configFile)

	server := rest.MustNewServer(c.Server)
	defer server.Stop()

	ctx := svc.NewServiceContext(c)
	handler.RegisterHandlers(server, ctx)
	handler.RegisterWebhookHandlers(server, ctx)

	// error 5xx
	server.Use(recovery.RecoverMiddleware())

	// 设置自定义错误处理逻辑 3xx 4xx default: 400
	httpx.SetErrorHandler(handler.ErrorHandle)
	httpx.SetErrorHandlerCtx(handler.ErrorHandleCtx)

	fmt.Printf("Starting server at %s:%d...\n", c.Server.Host, c.Server.Port)
	server.Start()
}
