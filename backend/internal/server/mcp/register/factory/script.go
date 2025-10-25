package factory

import (
	"context"
	"fmt"
	"github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	lua "github.com/yuin/gopher-lua"
)

// ScriptFactory 提供基于脚本（如 Lua）的动态执行
type ScriptFactory interface {
	Create(spec *schemas.ToolSpec) server.ToolHandlerFunc
}

func NewScriptFactory() ScriptFactory {
	return &scriptFactory{}
}

type scriptFactory struct{}

func (s *scriptFactory) Create(spec *schemas.ToolSpec) server.ToolHandlerFunc {
	// 从 Metadata 中读取脚本内容
	raw, ok := spec.Metadata["script"].(string)
	if !ok || raw == "" {
		return nil
	}
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		L := lua.NewState()
		defer L.Close()

		// 可以根据需要，把 req.Params 注入到 Lua 环境
		if err := L.DoString(raw); err != nil {
			return nil, fmt.Errorf("脚本执行失败: %w", err)
		}
		// 假定脚本执行后，把结果放到全局变量 result
		lv := L.GetGlobal("result")
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{Type: "text", Text: fmt.Sprintf("%v", lv)},
			},
		}, nil
	}
}
