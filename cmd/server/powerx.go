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

// 定义了一个全局变量 configFile，用于指定配置文件的路径，默认为 etc/powerx.yaml。
var configFile = flag.String("f", "etc/powerx.yaml", "the config file")

// 在 main() 函数中，首先解析命令行参数，加载配置文件，然后初始化 Gin 框架，并设置为发布模式。
func main() {
	// 解析命令行参数
	flag.Parse()

	// ---------- 加载配置文件 ----------
	var c config.Config
	conf.MustLoad(*configFile, &c)

	// 设置配置文件目录
	c.EtcDir = filepath.Dir(*configFile)

	// ---------- 设置 Gin 框架为发布模式 ----------
	gin.SetMode(gin.ReleaseMode)
	// 创建 Gin 路由实例
	r := zgin.NewRouter()

	// 创建 Go-Zero REST 服务器实例
	server := rest.MustNewServer(c.Server, rest.WithRouter(r))

	// 设置跨域配置
	runOpt := config.SetupCors(&c)
	if runOpt == nil {
		server = rest.MustNewServer(c.Server)
	} else {
		server = rest.MustNewServer(c.Server, runOpt)
	}
	// 确保服务器关闭
	defer server.Stop()

	// ---------- 设置日志 ----------
	zerox.MustSetupLog(c.Log)

	// ---------- 创建插件管理器实例 ----------
	plugin := pluginx.NewManager(context.Background(), r, fmt.Sprintf("%s:%d", "127.0.0.1", c.Server.Port))
	// 创建服务上下文
	ctx := svc.NewServiceContext(c, svc.WithPlugin(plugin))

	// 启动插件
	go func() {
		plugin.Start()
	}()
	// 确保插件关闭
	defer ctx.Plugin.Close()

	// ---------- 注册业务处理函数 ----------
	handler.RegisterHandlers(server, ctx)
	// ---------- 注册 Webhook 处理函数 ----------
	handler.RegisterWebhookHandlers(server, ctx)
	// ---------- 注册静态资源处理函数 ----------
	handler.RegisterStaticHandlers(server, ctx)

	// ---------- 添加恢复中间件 ----------
	server.Use(recovery.RecoverMiddleware())

	// ---------- 设置自定义错误处理逻辑 ----------
	// 3xx 4xx default: 400
	httpx.SetErrorHandler(handler.ErrorHandle)
	httpx.SetErrorHandlerCtx(handler.ErrorHandleCtx)

	// ---------- 启动服务器 ----------
	fmt.Printf("Starting server at %s:%d...\n", c.Server.Host, c.Server.Port)
	server.Start()
}
