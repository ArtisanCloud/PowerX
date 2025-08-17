package factory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"

	"net/http"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// RemoteFactory 提供将请求转发到外部 HTTP 服务的能力
type RemoteFactory interface {
	Create(spec *schemas.ToolSpec) server.ToolHandlerFunc
}

func NewRemoteFactory() RemoteFactory {
	return &remoteFactory{}
}

type remoteFactory struct{}

func (r *remoteFactory) Create(spec *schemas.ToolSpec) server.ToolHandlerFunc {
	// 从 Metadata 中读取 endpoint 配置
	endpoint, ok := spec.Metadata["endpoint"].(string)
	if !ok || endpoint == "" {
		return nil
	}
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// 序列化请求并转发
		body, err := json.Marshal(req)
		if err != nil {
			return nil, err
		}
		httpResp, err := http.Post(endpoint, "application/json", bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		defer httpResp.Body.Close()

		var result mcp.CallToolResult
		if err := json.NewDecoder(httpResp.Body).Decode(&result); err != nil {
			return nil, fmt.Errorf("解析远程响应失败: %w", err)
		}
		return &result, nil
	}
}
