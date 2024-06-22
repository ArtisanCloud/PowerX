package pluginx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/rest/httpx"
	"gopkg.in/yaml.v3"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Manager 管理插件的加载、启动和关闭
type Manager struct {
	r httpx.Router

	mainReadyMonitor *ReadyMonitor
	pluginList       []*Plugin
	pluginMap        map[string]*Plugin
	etc              *PluginManagerEtc
	frontendInfo     *PluginFrontendInfo
	buildInfo        *BuildInfo

	ctx      context.Context
	mainHost string

	frontendServer *PluginFrontendServer
}

// NewManager 创建一个新的插件管理器实例
func NewManager(ctx context.Context, route httpx.Router, mainHost string) *Manager {
	manager := &Manager{
		r:                route,
		mainReadyMonitor: NewReadyMonitor(mainHost),
		pluginMap:        make(map[string]*Plugin),
		ctx:              ctx,
		mainHost:         mainHost,
	}
	manager.frontendServer = NewPluginFrontendServer(manager)
	return manager
}

// ProxyHandleFunc 返回处理代理请求的 HTTP 处理函数
func (m *Manager) ProxyHandleFunc() http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		const prefix = "/api/plugin"
		const unknownError = `{"error": "plugin backend request failed"}`

		// 如果请求路径不以指定前缀开头，则返回未知错误
		if !strings.HasPrefix(request.URL.Path, prefix) {
			http.Error(writer, unknownError, http.StatusInternalServerError)
			return
		}

		// 根据路径分割获取插件名称和路径
		splitPaths := strings.Split(request.URL.Path, "/")
		if len(splitPaths) < 4 {
			http.Error(writer, unknownError, http.StatusInternalServerError)
			return
		}

		name := splitPaths[3]
		pluginPath := strings.Join(splitPaths[4:], "/")

		// 根据插件名称查找插件
		plugin, ok := m.pluginMap[name]
		if !ok || !plugin.IsReady() {
			logx.Errorf("Plugin %s not found", name)
			http.Error(writer, unknownError, http.StatusInternalServerError)
			return
		}

		// 构建目标 URL
		targetURL := fmt.Sprintf("http://%s/%s", plugin.PluginHost, pluginPath)
		fmt.Println(targetURL)
		if request.URL.RawQuery != "" {
			targetURL = targetURL + "?" + request.URL.RawQuery
		}

		// 读取请求体
		var buf bytes.Buffer
		if _, err := io.Copy(&buf, request.Body); err != nil {
			logx.Errorf("Error reading request body: %v", err)
			http.Error(writer, "Error reading request body", http.StatusInternalServerError)
			return
		}

		// 创建代理请求
		proxyReq, err := http.NewRequest(request.Method, targetURL, &buf)

		if err != nil {
			logx.Errorf("Error creating proxy request: %v", err)
			http.Error(writer, "Error creating proxy request", http.StatusInternalServerError)
			return
		}

		// 复制请求头
		copyHeaders(request.Header, proxyReq.Header)
		// 发送代理请求
		resp, err := http.DefaultClient.Do(proxyReq)
		if err != nil {
			logx.Errorf("Error sending proxy request: %v", err)
			http.Error(writer, "Error sending proxy request", http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		// 复制响应头
		copyHeaders(resp.Header, writer.Header())
		writer.WriteHeader(resp.StatusCode)

		// 发送代理响应
		if _, err = io.Copy(writer, resp.Body); err != nil {
			logx.Errorf("Error sending proxy request: %v", err)
			http.Error(writer, "Error sending proxy request", http.StatusBadGateway)
		}
	}
}

// PluginRoute 插件嵌入主服务的路由
type PluginRoute struct {
	Name string
	Path string
	Meta struct {
		Icon   string
		Locale string
	}
}

func copyHeaders(src, dst http.Header) {
	for key, values := range src {
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

// SetupPluginManager 初始化插件管理器
func (m *Manager) SetupPluginManager() {
	// 如果插件目录不存在, 则创建插件目录
	if _, err := os.Stat("./plugins"); os.IsNotExist(err) {
		err := os.Mkdir("./plugins", os.ModePerm)
		if err != nil {
			logger.Error(fmt.Sprintf("creating plugins directory: %v", err))
			return
		}
	}

	// 读取缓存的插件信息
	buildData, err := readCachePluginInfo("./plugins/.config")
	if err != nil {
		buildData = &BuildData{
			Build:    BuildInfo{},
			Frontend: PluginFrontendInfo{},
			Etc:      PluginManagerEtc{},
		}
	}

	// 扫描插件目录下的插件信息, 获取插件名和版本号
	buildInfo, err := scanDirPluginInfo("./plugins")
	if err != nil {
		logger.Error(fmt.Sprintf("scanning plugin info: %v", err))
		return
	}

	// 定义重新构建插件的函数
	rebuild := func() {
		loader := NewLoader("./plugins", &BuildLoaderConfig{
			MainAPIEndpoint: "/api/plugin",
		})
		err = loader.CheckEnvDependency()
		if err != nil {
			logger.Error(fmt.Sprintf("checking env dependency: %v", err))
			return
		}
		err = loader.BuildPluginFrontend(BuildPluginFrontendOptions{
			ReDownload: true,
		})
		if err != nil {
			logger.Error(fmt.Sprintf("building plugin frontend: %v", err))
			return
		}
	}

	// 如果 buildData 为空, 且 buildInfo 存在插件信息, 则应该重新构建插件
	if buildData == nil && len(buildInfo.Plugins) > 0 {
		rebuild()
	}

	// 如果 BuildData 和 BuildInfo 比较, 有插件信息不一致, 则应该重新构建插件
	if buildData != nil && len(buildInfo.Plugins) > 0 {
		if len(buildData.Build.Plugins) != len(buildInfo.Plugins) {
			rebuild()
		} else {
			for _, buildPlugin := range buildInfo.Plugins {
				var found bool
				for _, plugin := range buildData.Build.Plugins {
					if buildPlugin.Name == plugin.Name && buildPlugin.Version == plugin.Version {
						found = true
						break
					}
				}
				if !found {
					rebuild()
					break
				}
			}
		}
	}

	// 重新加载 cache 中的插件信息
	if buildData == nil {
		buildData, err = readCachePluginInfo("./plugins/.config")
		if err != nil {
			logger.Error(fmt.Sprintf("Error reading cache plugin info: %v", err))
			return
		}
	}

	m.buildInfo = &buildData.Build
	m.etc = &buildData.Etc
	m.frontendInfo = &buildData.Frontend
	logger.Info(fmt.Sprintf("plugin manager setup success: %d plugins founded", len(m.buildInfo.Plugins)))
}

// ScanDirPluginInfo 扫描插件目录下的插件信息, 获取插件名和版本号
func scanDirPluginInfo(dir string) (*BuildInfo, error) {
	logger.Info(fmt.Sprintf("scanning plugin info: %s", dir))
	var buildInfo BuildInfo
	buildInfo.Plugins = make([]BuildPluginItem, 0)
	// 扫描插件目录下所有不以.开头的一级目录
	var pluginPaths []string
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
			pluginPath := filepath.Join(dir, entry.Name())
			pluginPaths = append(pluginPaths, pluginPath)
		}
	}
	// 遍历所有目录，获取info.yaml
	for _, pluginPath := range pluginPaths {
		// 获取info.yaml
		infoPath := filepath.Join(pluginPath, "info.yaml")
		fileBytes, err := os.ReadFile(infoPath)
		if err != nil {
			continue
		}
		// 解析 	info.yaml
		var info BuildPluginItem
		err = yaml.Unmarshal(fileBytes, &info)
		if err != nil {
			continue
		}
		buildInfo.Plugins = append(buildInfo.Plugins, BuildPluginItem{
			Name:    info.Name,
			Version: info.Version,
		})
	}
	return &buildInfo, nil
}

// 读取缓存的插件信息
func readCachePluginInfo(dir string) (*BuildData, error) {
	logger.Info(fmt.Sprintf("reading cache plugin info: %s", dir))
	var buildData BuildData
	buildData.Build.Plugins = make([]BuildPluginItem, 0)
	// 获取 dir 下的 build.yaml, frontend.yaml, etc.yaml
	buildPath := filepath.Join(dir, "build.yaml")
	frontendPath := filepath.Join(dir, "frontend.yaml")
	etcPath := filepath.Join(dir, "etc.yaml")
	// 如果 build.yaml 不存在, 则返回 nil
	if _, err := os.Stat(buildPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("build.yaml not found")
	}
	// 如果 frontend.yaml 不存在, 则返回 nil
	if _, err := os.Stat(frontendPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("frontend.yaml not found")
	}
	// 如果 etc.yaml 不存在, 则返回 nil
	if _, err := os.Stat(etcPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("etc.yaml not found")
	}
	// 读取 build.yaml
	buildFile, err := os.ReadFile(buildPath)
	if err != nil {
		return nil, err
	}
	// 解析 build.yaml
	err = yaml.Unmarshal(buildFile, &buildData.Build)
	if err != nil {
		return nil, err
	}
	// 读取 frontend.yaml
	frontendInfoFile, err := os.ReadFile(frontendPath)
	if err != nil {
		return nil, err
	}
	// 解析 frontend.yaml
	err = yaml.Unmarshal(frontendInfoFile, &buildData.Frontend)
	if err != nil {
		return nil, err
	}
	// 读取 etc.yaml
	etcFile, err := os.ReadFile(etcPath)
	if err != nil {
		return nil, err
	}
	// 解析 etc.yaml
	err = yaml.Unmarshal(etcFile, &buildData.Etc)
	if err != nil {
		return nil, err
	}
	return &buildData, nil
}

func (m *Manager) Start() {
	// 初始化插件管理器
	m.SetupPluginManager()

	// 加载所有插件
	for _, info := range m.buildInfo.Plugins {
		LoadPlugin(m, info.Name)
	}

	// 遍历启动插件
	for _, p := range m.pluginList {
		p := p
		go func() {
			logger.Info(fmt.Sprintf("starting plugin: %s", p.Name))
			if err := p.Start(); err != nil {
				return
			}
		}()
	}

	// 如果有超过一个插件处于启用状态 启动插件前端服务
	if slices.ContainsFunc(m.pluginList, func(e *Plugin) bool {
		return e.Enable
	}) {
		go func() {
			logger.Info("serving plugin frontend")
			err := m.frontendServer.Serve("./plugins/.plugin/dist", "5717")
			if err != nil {
				logger.Error("serving plugin frontend failed: ", err)
			}
		}()
	}

	// 阻塞
	select {
	case <-m.ctx.Done():
		logger.Info("plugin manager stopped")
		return
	}
}

func (m *Manager) InitRoute() {
	methods := []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTION"}
	for _, method := range methods {
		m.r.Handle(method, "/api/plugin/*path", m.ProxyHandleFunc())
	}
}

// Register 注册插件并且返回配置
func (m *Manager) Register(name string, addr string, routes []BackendRoute) (PluginEtcMap, error) {
	// name must be hyphenated
	name = StringToHyphenCase(name)

	// todo validate plugin licence and key, and active plugin auth config

	if p, ok := m.pluginMap[name]; ok {
		p.BackendRoutes = routes
		p.PluginHost = addr
		return p.Etc, nil
	} else {
		return nil, errors.New("plugin not found, registration failed")
	}
}

func (m *Manager) List() []*Plugin {
	var plugins []*Plugin
	for _, plugin := range m.pluginMap {
		plugins = append(plugins, plugin)
	}
	return plugins
}

// ListFrontendRoutes 获取所有处于激活状态插件的前端路由
func (m *Manager) ListFrontendRoutes() []PluginFrontendRoute {
	var routes []PluginFrontendRoute
	for _, p := range m.List() {
		for _, route := range m.frontendInfo.Routes {
			if p.IsReady() && p.Name == route.Name {
				routes = append(routes, route)
			}
		}
	}
	return routes
}

func (m *Manager) StartPlugins() {
	go func() {
		for _, plugin := range m.pluginMap {
			if plugin.Enable {
				plugin.Start()
			}
		}
	}()
}

func (m *Manager) Close() {
	for _, plugin := range m.pluginMap {
		plugin.Stop()
	}
}
