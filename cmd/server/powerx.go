package main

import (
	"PowerX/internal/middleware/recovery"
	"PowerX/pkg/zerox"
	"flag"
	"fmt"
	"github.com/zeromicro/go-zero/rest/httpx"
	"path/filepath"

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

	c.EtcDir = filepath.Dir(*configFile)

	var server *rest.Server
	runOpt := config.SetupCors(&c)
	if runOpt == nil {
		server = rest.MustNewServer(c.Server)
	} else {
		server = rest.MustNewServer(c.Server, runOpt)
	}
	defer server.Stop()
	zerox.MustSetupLog(c.Log)

	ctx := svc.NewServiceContext(c)
	handler.RegisterHandlers(server, ctx)
	handler.RegisterWebhookHandlers(server, ctx)

	handler.RegisterStaticHandlers(server, ctx)

	// error 5xx
	server.Use(recovery.RecoverMiddleware())

	// 设置自定义错误处理逻辑 3xx 4xx default: 400
	httpx.SetErrorHandler(handler.ErrorHandle)
	httpx.SetErrorHandlerCtx(handler.ErrorHandleCtx)

	fmt.Printf("Starting server at %s:%d...\n", c.Server.Host, c.Server.Port)
	server.Start()
}
