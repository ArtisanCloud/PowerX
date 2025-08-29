package factory

import (
	"github.com/ArtisanCloud/PowerX/internal/server/mcp/tools"
	"github.com/ArtisanCloud/PowerX/internal/server/mcp/types"
	"github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/mark3labs/mcp-go/server"
)

// mcp/register/factory/native.go

// NativeFactory 提供对内置工具的映射
type NativeFactory interface {
	Create(spec *schemas.ToolSpec) server.ToolHandlerFunc
}

func NewNativeFactory() NativeFactory {
	return &nativeFactory{}
}

type nativeFactory struct{}

func (n *nativeFactory) Create(spec *schemas.ToolSpec) server.ToolHandlerFunc {
	switch spec.ID {
	case types.ToolListBlueprints:
		return tools.ListBlueprintsTool
	case types.ToolLoadBlueprint:
		return tools.LoadBlueprintTool
	case types.ToolPlanFlow:
		return tools.PlanFlowTool
	case types.ToolRenderPlan:
		return tools.RenderPlanTool
	default:
		// 如果找不到内置实现，可返回 nil
		return nil
	}
}
