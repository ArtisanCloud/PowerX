package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/ArtisanCloud/PowerX/services/mcp/types"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"gopkg.in/yaml.v3"
)

// mcp/tools/list_blueprints.go

// BlueprintInfo 蓝图信息结构体
type BlueprintInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// ListBlueprintsTool 列出所有可用蓝图
func ListBlueprintsTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// 获取配置
	cfg := config.GetGlobalConfig().MCP

	// 构建蓝图目录路径
	usecasesPath := filepath.Join(cfg.FlowSpecsConfig.Blueprints, "usecases")

	// 检查目录是否存在
	if _, err := os.Stat(usecasesPath); os.IsNotExist(err) {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: types.FieldType,
					Text: fmt.Sprintf("Error: blueprints usecases directory not found: %s", usecasesPath),
				},
			},
		}, nil
	}

	var blueprints []BlueprintInfo

	// 遍历目录中的所有 YAML 文件
	err := filepath.WalkDir(usecasesPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// 跳过目录
		if d.IsDir() {
			return nil
		}

		// 只处理 .yaml 文件
		if !strings.HasSuffix(path, ".yaml") {
			return nil
		}

		// 读取文件内容
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read blueprint file %s: %w", path, err)
		}

		// 解析 YAML 获取基本信息
		var flow schemas.Flow
		if err := yaml.Unmarshal(data, &flow); err != nil {
			// 如果解析失败，跳过该文件
			return nil
		}

		// 生成蓝图ID（文件名去掉扩展名）
		filename := filepath.Base(path)
		blueprintID := strings.TrimSuffix(filename, ".yaml")

		// 添加到列表
		blueprints = append(blueprints, BlueprintInfo{
			ID:          blueprintID,
			Name:        flow.Name,
			Description: flow.Description,
		})

		return nil
	})
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: types.FieldType,
					Text: fmt.Sprintf("Error: failed to scan blueprints directory: %v", err),
				},
			},
		}, nil
	}

	// 将结果转换为 JSON
	blueprintsJSON, err := json.Marshal(blueprints)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: types.FieldType,
					Text: fmt.Sprintf("Error: failed to marshal blueprints: %v", err),
				},
			},
		}, nil
	}

	// 返回结果
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: types.FieldType,
				Text: fmt.Sprintf("Found %d blueprints", len(blueprints)),
			},
			mcp.TextContent{
				Type: types.FieldType,
				Text: string(blueprintsJSON),
			},
		},
	}, nil
}
