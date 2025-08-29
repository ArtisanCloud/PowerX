package factory

import (
	"github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/mark3labs/mcp-go/server"
)

// mcp/register/factory

// HandlerFactory 根据 ToolSpec.HandlerType 分发到不同的子工厂
type HandlerFactory interface {
	CreateHandler(spec *schemas.ToolSpec) server.ToolHandlerFunc
}

// NewHandlerFactory 返回默认的 HandlerFactory 实例
func NewHandlerFactory() HandlerFactory {
	return &defaultFactory{
		native: NewNativeFactory(),
		script: NewScriptFactory(),
		remote: NewRemoteFactory(),
	}
}

type defaultFactory struct {
	native NativeFactory
	script ScriptFactory
	remote RemoteFactory
}

func (f *defaultFactory) CreateHandler(spec *schemas.ToolSpec) server.ToolHandlerFunc {
	switch spec.HandlerType {
	case "", "native":
		return f.native.Create(spec)
	case "script":
		return f.script.Create(spec)
	case "remote":
		return f.remote.Create(spec)
	default:
		// 未知类型，可返回 nil 或一个报错 handler
		return nil
	}
}
