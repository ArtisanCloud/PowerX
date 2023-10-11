package main

import (
	"PowerX/internal/middleware/recovery"
	"PowerX/pkg/pluginx"
	"PowerX/pkg/zerox"
	"context"
	"flag"
	"fmt"
	"github.com/zeromicro/go-zero/rest"
	"github.com/zeromicro/go-zero/rest/httpx"
	"path/filepath"

	"PowerX/internal/config"
	"PowerX/internal/handler"
	"PowerX/internal/svc"

	"github.com/gin-gonic/gin"
	zgin "github.com/zeromicro/zero-contrib/router/gin"

	"github.com/zeromicro/go-zero/core/conf"
)

var configFile = flag.String("f", "etc/powerx.yaml", "the config file")

func main() {
	flag.Parse()

	var c config.Config
	conf.MustLoad(*configFile, &c)

	c.EtcDir = filepath.Dir(*configFile)

	gin.SetMode(gin.ReleaseMode)
	r := zgin.NewRouter()
	server := rest.MustNewServer(c.Server, rest.WithRouter(r))
	runOpt := config.SetupCors(&c)
	if runOpt == nil {
		server = rest.MustNewServer(c.Server)
	} else {
		server = rest.MustNewServer(c.Server, runOpt)
	}
	defer server.Stop()
	zerox.MustSetupLog(c.Log)

	plugin := pluginx.NewManager(context.Background(), r, fmt.Sprintf("%s:%d", "127.0.0.1", c.Server.Port))
	ctx := svc.NewServiceContext(c, svc.WithPlugin(plugin))

	go func() {
		plugin.Start()
	}()
	defer ctx.Plugin.Close()

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
