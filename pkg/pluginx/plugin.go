package pluginx

import (
	"strings"
)

type Plugin struct {
	BuildPluginItem
	// todo auth
	Etc           PluginEtcMap
	BackendRoutes []BackendRoute
	m             *Manager
	backendServer *PluginBackendServer
	Enable        bool
	PluginHost    string
}

type BackendRoute struct {
	Method string
	Path   string
}

func LoadPlugin(m *Manager, name string) *Plugin {
	plugin := &Plugin{
		m: m,
	}
	// 在 m 中查找 name 对应的插件信息和配置
	for _, p := range m.buildInfo.Plugins {
		if p.Name == name {
			plugin = &Plugin{
				BuildPluginItem: p,
				m:               m,
			}
			break
		}
	}
	if plugin == nil {
		return nil
	}
	// 在 m 中查找 name 对应的插件配置
	for _, p := range m.etc.Plugins {
		if p.Name == name {
			plugin.Etc = p.Etc
			plugin.Enable = p.Enable
			break
		}
	}
	plugin.backendServer = NewPluginBackendServer(plugin)
	m.pluginList = append(m.pluginList, plugin)
	m.pluginMap[strings.ToLower(name)] = plugin
	return plugin
}

func (p *Plugin) WithRoutes(routes []BackendRoute) *Plugin {
	p.BackendRoutes = routes
	return p
}

func (p *Plugin) Start() error {
	if p.IsReady() {
		return nil
	}
	err := p.backendServer.Serve()
	if err != nil {
		return err
	}
	return nil
}

func (p *Plugin) Stop() error {
	if !p.Enable {
		return nil
	}
	err := p.backendServer.Stop()
	if err != nil {
		return err
	}
	return nil
}

func (p *Plugin) Restart() error {
	err := p.Stop()
	if err != nil {
		return err
	}
	err = p.Start()
	if err != nil {
		return err
	}
	return nil
}

// IsReady 判断插件是否就绪
func (p *Plugin) IsReady() bool {
	return p.backendServer.Process != nil
}
