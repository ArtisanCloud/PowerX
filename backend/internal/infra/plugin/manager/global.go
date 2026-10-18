package manager

import "github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"

// 保留既有基础设施入口，所有调用共享 pkg/plugin_mgr 的同一个注册实例。
func SetGlobalProvider(p func() (plugin_mgr.Manager, error)) { plugin_mgr.SetGlobalProvider(p) }
func InitGlobal(m plugin_mgr.Manager)                        { plugin_mgr.InitGlobal(m) }
func GetPluginManager() plugin_mgr.Manager                   { return plugin_mgr.GetPluginManager() }
func ResetGlobalForTest(m plugin_mgr.Manager)                { plugin_mgr.ResetGlobalForTest(m) }
func CurrentPluginManager() plugin_mgr.Manager               { return plugin_mgr.CurrentPluginManager() }
