package pluginx

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

type PluginFrontendServer struct {
	*Manager
}

func NewPluginFrontendServer(m *Manager) *PluginFrontendServer {
	return &PluginFrontendServer{
		Manager: m,
	}
}

func (s *PluginFrontendServer) Serve(dir string, port string) error {
	// 如果目录下不存在 index.html, 则报错
	if _, err := os.Stat(filepath.Join(dir, "index.html")); os.IsNotExist(err) {
		return fmt.Errorf("index.html not found")
	}

	r := gin.Default()

	r.Use(ginCORS())

	// 自定义静态文件处理器
	r.Static("/css", filepath.Join(dir, "css"))
	r.Static("/assets", filepath.Join(dir, "assets"))

	// api 路由转发, /api -> proxy
	r.Any("/api/*path", gin.WrapF(s.ProxyHandleFunc()))

	// 单页面应用处理
	r.NoRoute(func(c *gin.Context) {
		c.File(filepath.Join(dir, "index.html"))
	})

	err := r.Run(fmt.Sprintf(":%s", port))
	logger.Info(fmt.Sprintf("plugin %s frontend server start at %s", s.mainHost, port))
	if err != nil {
		return err
	}
	return nil
}

type PluginBackendServer struct {
	p       *Plugin
	Host    string
	Process *os.Process
}

func NewPluginBackendServer(plugin *Plugin) *PluginBackendServer {
	return &PluginBackendServer{
		p: plugin,
	}
}

func (s *PluginBackendServer) Serve() error {
	if s.Process != nil {
		return fmt.Errorf("plugin already started")
	}
	// cmd 启动插件, 同时启动一个 goroutine 监听插件的就绪
	cmd := exec.Command(s.p.Backend, "-n", s.p.Name, "-h", s.p.m.mainHost, "-m", "prod")
	go func() {
		for i := 0; i < 10; i++ {
			if cmd.Process != nil {
				s.Process = cmd.Process
				break
			}
			// 等待 3s
			time.Sleep(time.Second * 3)
		}
		// 如果 30s 还没有启动成功, 则报错
		if cmd.Process == nil {
			logger.Error(fmt.Sprintf("plugin %s start failed", s.p.Name))
		} else {
			logger.Info(fmt.Sprintf("plugin %s start success", s.p.Name))
		}
	}()
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err := cmd.Start()
	if err != nil {
		logger.Error(fmt.Sprintf("plugin %s start failed: %s", s.p.Name, err.Error()))
		return err
	}
	return nil
}

func (s *PluginBackendServer) Stop() error {
	if s.Process == nil {
		return fmt.Errorf("plugin not started")
	}
	err := s.Process.Kill()
	if err != nil {
		return err
	}
	s.Process = nil
	return nil
}
