package tools

import (
	"context"
	"fmt"
	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/ArtisanCloud/PowerX/services/mcp/types"
	"os"
	"path/filepath"

	"github.com/mark3labs/mcp-go/mcp"
	"gopkg.in/yaml.v3"
)

// LoadBlueprintTool 蓝图加载工具 (使用正确的 mark3labs API)
func LoadBlueprintTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// 使用正确的 API 获取参数
	flowID := request.GetString(types.ParamFlowID, "")
	if flowID == "" {
		return nil, fmt.Errorf("缺少 %s 参数", types.ParamFlowID)
	}

	// 获取配置
	cfg := config.GetGlobalConfig().MCP

	// 构建蓝图文件路径，支持子目录
	// 首先尝试直接路径
	path := filepath.Join(cfg.FlowSpecsConfig.Blueprints, flowID+".yaml")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		// 如果直接路径不存在，尝试usecases子目录
		path = filepath.Join(cfg.FlowSpecsConfig.Blueprints, "usecases", flowID+".yaml")
	}

	// 读取文件
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("加载蓝图文件失败: %w", err)
	}

	// 解析YAML
	var flow schemas.Flow
	if err := yaml.Unmarshal(data, &flow); err != nil {
		return nil, fmt.Errorf("解析蓝图文件失败: %w", err)
	}

	// 返回结果 (使用正确的 Content 类型)
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: types.FieldType,
				Text: fmt.Sprintf("成功加载蓝图: %s\n描述: %s\n版本: %s", flow.Name, flow.Description, flow.Version),
			},
		},
	}, nil
}
